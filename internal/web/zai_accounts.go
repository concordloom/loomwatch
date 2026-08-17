package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ZaiAccounts handles Z.ai account management.
//
// Fork change: upstream watches a single Z.ai key, so a second subscription was
// invisible — and Coding Plan quota is counted per account, meaning the ones
// left out burn unseen until a run fails on an exhausted window. Endpoints
// mirror /api/minimax/accounts.
//
// GET    /api/zai/accounts        - list all accounts
// POST   /api/zai/accounts        - create account  (body: {name, api_key, base_url})
// PUT    /api/zai/accounts?id=N   - update account  (body: {name?, api_key?, base_url?, restore?})
// DELETE /api/zai/accounts?id=N   - soft-delete account
func (h *Handler) ZaiAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.zaiAccountsList(w, r)
	case http.MethodPost:
		h.zaiAccountCreate(w, r)
	case http.MethodPut:
		h.zaiAccountUpdate(w, r)
	case http.MethodDelete:
		h.zaiAccountDelete(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) zaiAccountsList(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}
	accounts, err := h.store.QueryProviderAccounts("zai")
	if err != nil {
		h.logger.Error("failed to query Z.ai accounts", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query accounts")
		return
	}
	result := make([]map[string]interface{}, 0, len(accounts))
	for _, acc := range accounts {
		entry := map[string]interface{}{
			"id":        acc.ID,
			"name":      acc.Name,
			"createdAt": acc.CreatedAt.Format(time.RFC3339),
			"hasKey":    strings.Contains(acc.Metadata, "api_key"),
		}
		var meta map[string]interface{}
		if acc.Metadata != "" {
			if json.Unmarshal([]byte(acc.Metadata), &meta) == nil {
				if b, ok := meta["base_url"].(string); ok {
					entry["baseUrl"] = b
				}
			}
		}
		if acc.DeletedAt != nil {
			entry["deletedAt"] = acc.DeletedAt.Format(time.RFC3339)
		}
		result = append(result, entry)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": result})
}

func (h *Handler) zaiAccountCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respondError(w, http.StatusBadRequest, "account name is required")
		return
	}
	if !validProfileName.MatchString(req.Name) {
		respondError(w, http.StatusBadRequest, "invalid account name: use only letters, numbers, hyphens, and underscores")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusInternalServerError, "store not available")
		return
	}

	acc, err := h.store.CreateOrRestoreProviderAccount("zai", req.Name)
	if err != nil {
		h.logger.Error("failed to create Z.ai account", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	meta := map[string]string{}
	if req.APIKey != "" {
		meta["api_key"] = req.APIKey
	}
	if req.BaseURL != "" {
		meta["base_url"] = req.BaseURL
	}
	if len(meta) > 0 {
		metaJSON, _ := json.Marshal(meta)
		if err := h.store.UpdateProviderAccountMetadata(acc.ID, string(metaJSON)); err != nil {
			h.logger.Error("failed to update Z.ai account metadata", "error", err)
		}
	}

	if h.zaiAgentMgr != nil {
		h.zaiAgentMgr.Reload()
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "account created",
		"id":      acc.ID,
		"name":    req.Name,
	})
}

func (h *Handler) zaiAccountUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "valid account id is required")
		return
	}

	var req struct {
		Name    *string `json:"name"`
		APIKey  *string `json:"api_key"`
		BaseURL *string `json:"base_url"`
		Restore *bool   `json:"restore"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusInternalServerError, "store not available")
		return
	}

	acc, err := h.store.GetProviderAccountByID(id)
	if err != nil || acc == nil || acc.Provider != "zai" {
		respondError(w, http.StatusNotFound, "account not found")
		return
	}

	if req.Restore != nil && *req.Restore {
		if err := h.store.UndeleteProviderAccountByID(id); err != nil {
			h.logger.Error("failed to restore Z.ai account", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to restore account")
			return
		}
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		trimmedName := strings.TrimSpace(*req.Name)
		if !validProfileName.MatchString(trimmedName) {
			respondError(w, http.StatusBadRequest, "invalid account name")
			return
		}
		if err := h.store.RenameProviderAccount(id, trimmedName); err != nil {
			h.logger.Error("failed to rename Z.ai account", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to rename account")
			return
		}
	}

	// Metadata is merged, not replaced: an update that only carries a key must
	// not silently drop a custom base_url.
	if req.APIKey != nil || req.BaseURL != nil {
		existing := map[string]string{}
		if acc.Metadata != "" {
			json.Unmarshal([]byte(acc.Metadata), &existing)
		}
		if req.APIKey != nil && *req.APIKey != "" {
			existing["api_key"] = *req.APIKey
		}
		if req.BaseURL != nil {
			if *req.BaseURL == "" {
				delete(existing, "base_url")
			} else {
				existing["base_url"] = *req.BaseURL
			}
		}
		metaJSON, _ := json.Marshal(existing)
		if err := h.store.UpdateProviderAccountMetadata(id, string(metaJSON)); err != nil {
			h.logger.Error("failed to update Z.ai account metadata", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to update account")
			return
		}
	}

	if h.zaiAgentMgr != nil {
		h.zaiAgentMgr.Reload()
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "account updated", "id": id})
}

func (h *Handler) zaiAccountDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "valid account id is required")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusInternalServerError, "store not available")
		return
	}

	acc, err := h.store.GetProviderAccountByID(id)
	if err != nil || acc == nil || acc.Provider != "zai" {
		respondError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := h.store.MarkProviderAccountDeletedByID(id); err != nil {
		h.logger.Error("failed to delete Z.ai account", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}

	if h.zaiAgentMgr != nil {
		h.zaiAgentMgr.Reload()
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "account deleted", "id": id})
}

// ZaiAccountsUsage returns current usage for every active Z.ai account.
//
// Fork change: feeds the multi-account overview on the Z.ai tab. Without it the
// dashboard could only ever show one subscription, which is how a second one
// burns unnoticed — the same failure the exporter side of this work fixes.
// Shape mirrors /api/minimax/accounts/usage so the existing overview renderer
// can consume it.
func (h *Handler) ZaiAccountsUsage(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}
	accounts, err := h.store.QueryActiveProviderAccounts("zai")
	if err != nil {
		h.logger.Error("failed to query Z.ai accounts", "error", err)
		respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}

	result := make([]map[string]interface{}, 0, len(accounts))
	for _, acc := range accounts {
		current := h.buildZaiCurrent(acc.ID)
		result = append(result, map[string]interface{}{
			"accountId":   acc.ID,
			"accountName": acc.Name,
			"capturedAt":  current["capturedAt"],
			"quotas":      zaiOverviewQuotas(current),
		})
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": result})
}

// zaiOverviewQuotas flattens the per-quota maps of buildZaiCurrent into the
// list shape the overview cards read.
func zaiOverviewQuotas(current map[string]interface{}) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, 2)
	for _, q := range []struct {
		key   string
		name  string
		label string
	}{
		{"tokensLimit", "tokens", "Tokens Limit"},
		{"timeLimit", "time", "Time Limit"},
	} {
		raw, ok := current[q.key].(map[string]interface{})
		if !ok {
			continue
		}
		row := map[string]interface{}{
			"name":         q.name,
			"label":        q.label,
			"usagePercent": raw["percent"],
			"status":       raw["status"],
		}
		// buildZaiCurrent calls it renewsAt; the overview cards read resetAt.
		if reset, ok := raw["renewsAt"]; ok {
			row["resetAt"] = reset
		}
		rows = append(rows, row)
	}
	return rows
}
