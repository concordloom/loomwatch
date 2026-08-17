package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/notify"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
)

// ZaiAgentInstance represents a running agent for a specific Z.ai account.
type ZaiAgentInstance struct {
	DBAccountID int64
	AccountName string
	Agent       *ZaiAgent
	Cancel      context.CancelFunc
}

// ZaiAgentManager manages multiple ZaiAgent instances for multi-account support.
//
// Fork change: upstream watches one Z.ai key, so a second subscription stays
// invisible — and a Coding Plan quota is counted per account, so the ones left
// out burn unseen until a run fails. Modelled on MiniMaxAgentManager: entirely
// DB-driven, with hot reload when accounts change through the UI or API.
type ZaiAgentManager struct {
	store               *store.Store
	tracker             *tracker.ZaiTracker
	interval            time.Duration
	logger              *slog.Logger
	notifier            *notify.NotificationEngine
	pollingCheck        func() bool                // Global Z.ai polling check
	accountPollingCheck func(accountID int64) bool // Per-account polling check
	baseURL             string                     // Default quota endpoint

	mu        sync.RWMutex
	instances map[int64]*ZaiAgentInstance // db account id -> instance
	wg        sync.WaitGroup              // tracks running agent goroutines
	reloadMu  sync.Mutex                  // prevents concurrent Reload() calls
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewZaiAgentManager creates a new manager for multi-account Z.ai polling.
func NewZaiAgentManager(store *store.Store, tracker *tracker.ZaiTracker, interval time.Duration, logger *slog.Logger) *ZaiAgentManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ZaiAgentManager{
		store:     store,
		tracker:   tracker,
		interval:  interval,
		logger:    logger,
		instances: make(map[int64]*ZaiAgentInstance),
	}
}

// SetNotifier sets the notification engine for all agents.
func (m *ZaiAgentManager) SetNotifier(n *notify.NotificationEngine) {
	m.notifier = n
}

// SetPollingCheck sets the global polling check function.
func (m *ZaiAgentManager) SetPollingCheck(fn func() bool) {
	m.pollingCheck = fn
}

// SetAccountPollingCheck sets a per-account polling check function.
func (m *ZaiAgentManager) SetAccountPollingCheck(fn func(accountID int64) bool) {
	m.accountPollingCheck = fn
}

// SetBaseURL sets the default quota endpoint used when an account does not
// carry its own.
func (m *ZaiAgentManager) SetBaseURL(baseURL string) {
	m.baseURL = baseURL
}

// zaiAccountMeta holds the JSON metadata stored in provider_accounts.metadata.
type zaiAccountMeta struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

func parseZaiAccountMeta(raw string) zaiAccountMeta {
	var meta zaiAccountMeta
	if raw != "" {
		json.Unmarshal([]byte(raw), &meta)
	}
	return meta
}

// Run starts the manager, loads all accounts, and starts agents.
func (m *ZaiAgentManager) Run(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	defer m.cancel()

	m.logger.Info("Z.ai agent manager started", "interval", m.interval)

	if err := m.loadAndStartAccounts(); err != nil {
		m.logger.Error("failed to load initial Z.ai accounts", "error", err)
	}

	<-m.ctx.Done()
	m.stopAllAgents()
	return nil
}

// Reload stops all agents and restarts from the current DB state.
// Called by the web handler when accounts are added/updated/deleted.
func (m *ZaiAgentManager) Reload() {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	if m.ctx == nil || m.ctx.Err() != nil {
		return
	}
	m.logger.Info("Z.ai agent manager reloading accounts")
	m.stopAllAgents()
	if err := m.loadAndStartAccounts(); err != nil {
		m.logger.Error("failed to reload Z.ai accounts", "error", err)
	}
}

// loadAndStartAccounts reads active Z.ai accounts from the DB and starts an
// agent for each one that carries a key.
func (m *ZaiAgentManager) loadAndStartAccounts() error {
	accounts, err := m.store.QueryActiveProviderAccounts("zai")
	if err != nil {
		return fmt.Errorf("failed to query Z.ai accounts: %w", err)
	}

	for _, acc := range accounts {
		meta := parseZaiAccountMeta(acc.Metadata)
		if meta.APIKey == "" {
			m.logger.Debug("skipping Z.ai account without API key", "account", acc.Name, "id", acc.ID)
			continue
		}
		if err := m.startAgentForAccount(acc.ID, acc.Name, meta); err != nil {
			m.logger.Warn("failed to start Z.ai agent for account", "account", acc.Name, "error", err)
		}
	}

	m.mu.RLock()
	count := len(m.instances)
	m.mu.RUnlock()
	m.logger.Info("Z.ai accounts loaded", "count", count)

	return nil
}

// startAgentForAccount creates and starts an agent for a specific account.
func (m *ZaiAgentManager) startAgentForAccount(accountID int64, name string, meta zaiAccountMeta) error {
	m.mu.RLock()
	if _, exists := m.instances[accountID]; exists {
		m.mu.RUnlock()
		return nil // Already running
	}
	m.mu.RUnlock()

	baseURL := meta.BaseURL
	if baseURL == "" {
		baseURL = m.baseURL
	}

	var opts []api.ZaiOption
	if baseURL != "" {
		opts = append(opts, api.WithZaiBaseURL(baseURL))
	}
	client := api.NewZaiClient(meta.APIKey, m.logger, opts...)
	sm := NewSessionManager(m.store, fmt.Sprintf("zai:%d", accountID), 5*time.Minute, m.logger)
	agent := NewZaiAgentWithAccount(client, m.store, m.tracker, m.interval, m.logger, sm, accountID)

	if m.notifier != nil {
		agent.SetNotifier(m.notifier)
	}

	// Combine global + per-account polling checks
	agent.SetPollingCheck(func() bool {
		if m.pollingCheck != nil && !m.pollingCheck() {
			return false
		}
		if m.accountPollingCheck != nil && !m.accountPollingCheck(accountID) {
			return false
		}
		return true
	})

	agentCtx, agentCancel := context.WithCancel(m.ctx)

	instance := &ZaiAgentInstance{
		DBAccountID: accountID,
		AccountName: name,
		Agent:       agent,
		Cancel:      agentCancel,
	}

	m.mu.Lock()
	m.instances[accountID] = instance
	m.mu.Unlock()

	m.logger.Info("starting Z.ai agent for account", "account", name, "id", accountID)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("Z.ai agent panicked", "account", name, "panic", r)
			}
		}()
		if err := agent.Run(agentCtx); err != nil && agentCtx.Err() == nil {
			m.logger.Error("Z.ai agent error", "account", name, "error", err)
		}
	}()

	return nil
}

// stopAllAgents stops all running agents and waits for goroutines to finish.
func (m *ZaiAgentManager) stopAllAgents() {
	m.mu.Lock()
	instances := make([]*ZaiAgentInstance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.instances = make(map[int64]*ZaiAgentInstance)
	m.mu.Unlock()

	for _, inst := range instances {
		if inst.Cancel != nil {
			inst.Cancel()
		}
	}
	m.wg.Wait()
}

// GetRunningAccounts returns information about currently running account agents.
func (m *ZaiAgentManager) GetRunningAccounts() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, map[string]interface{}{
			"name":          inst.AccountName,
			"db_account_id": inst.DBAccountID,
		})
	}
	return result
}
