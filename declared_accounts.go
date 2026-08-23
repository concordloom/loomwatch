package main

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// applyDeclaredAccounts makes the store match the accounts named in
// configuration.
//
// Until this existed, the first account of a provider came from an environment
// variable and every further one had to be created by hand in the panel. That
// made the set of subscriptions a property of one SQLite file rather than of
// the deployment: it could not be reviewed, could not be recreated, and could
// not survive a volume being replaced - which is also why the chart could not
// honestly offer to run without persistence.
//
// Accounts are upserted, never removed. An account in the store that nobody
// declared is reported and left alone: deleting a subscription because a line
// went missing from a values file would take its history with it, and a typo
// should not be able to do that.
func applyDeclaredAccounts(db *store.Store, accounts []config.DeclaredAccount, logger *slog.Logger) error {
	if len(accounts) == 0 {
		return nil
	}

	declared := map[string]bool{}
	for _, a := range accounts {
		acc, err := db.GetOrCreateProviderAccount(a.Provider, a.Name)
		if err != nil {
			return fmt.Errorf("declared account %s/%s: %w", a.Provider, a.Name, err)
		}
		declared[a.Provider+"/"+a.Name] = true

		meta, err := mergeAccountMetadata(acc.Metadata, a)
		if err != nil {
			return fmt.Errorf("declared account %s/%s: %w", a.Provider, a.Name, err)
		}
		if meta == acc.Metadata {
			continue
		}
		if err := db.UpdateProviderAccountMetadata(acc.ID, meta); err != nil {
			return fmt.Errorf("declared account %s/%s: %w", a.Provider, a.Name, err)
		}
		logger.Info("applied declared account", "provider", a.Provider, "name", a.Name, "id", acc.ID)
	}

	reportUndeclaredAccounts(db, declared, logger)
	return nil
}

// mergeAccountMetadata writes the declared fields into the stored blob without
// discarding what else is in it.
//
// Overwriting wholesale is how a stored base_url, or anything a later version
// starts keeping here, disappears the first time an account is re-declared. The
// values are kept as raw JSON so a number stays a number: decoding into
// map[string]string turns a stored 30 into "" on the way back out.
func mergeAccountMetadata(stored string, a config.DeclaredAccount) (string, error) {
	existing := map[string]json.RawMessage{}
	if stored != "" && stored != "null" {
		if err := json.Unmarshal([]byte(stored), &existing); err != nil {
			return "", fmt.Errorf("stored metadata is not readable, refusing to overwrite it: %w", err)
		}
	}

	key, err := json.Marshal(a.APIKey())
	if err != nil {
		return "", fmt.Errorf("encoding api_key: %w", err)
	}
	existing["api_key"] = key

	if a.BaseURL != "" {
		base, err := json.Marshal(a.BaseURL)
		if err != nil {
			return "", fmt.Errorf("encoding base_url: %w", err)
		}
		existing["base_url"] = base
	}

	out, err := json.Marshal(existing)
	if err != nil {
		return "", fmt.Errorf("encoding metadata: %w", err)
	}
	return string(out), nil
}

// reportUndeclaredAccounts names the accounts the store has and configuration
// does not, so that drift is visible without being acted on.
func reportUndeclaredAccounts(db *store.Store, declared map[string]bool, logger *slog.Logger) {
	for provider := range map[string]bool{"zai": true, "minimax": true} {
		accounts, err := db.QueryActiveProviderAccounts(provider)
		if err != nil {
			continue
		}
		for _, acc := range accounts {
			if acc.Name == "default" || declared[provider+"/"+acc.Name] {
				continue
			}
			logger.Warn("account exists but is not declared in configuration; "+
				"it keeps polling and will not be removed, but it cannot be recreated "+
				"from configuration if this database is lost",
				"provider", provider, "name", acc.Name, "id", acc.ID)
		}
	}
}
