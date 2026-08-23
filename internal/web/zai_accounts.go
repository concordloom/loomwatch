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
		// hasKey answers "is this account going to be polled", so it is computed
		// from a key that was actually decoded out of the blob. It used to be
		// strings.Contains(acc.Metadata, "api_key"), which is true of any text
		// carrying that substring - including a damaged blob out of which the
		// agent manager can read nothing. An account in that state reported
		// itself as configured while sitting out of the polling rotation.
		hasKey := false
		baseURL := ""
		haveBaseURL := false
		var meta map[string]interface{}
		if acc.Metadata != "" {
			if err := json.Unmarshal([]byte(acc.Metadata), &meta); err != nil {
				// The parse stays non-fatal here. This is a read that decorates
				// one entry, so an unreadable row costs a display field; failing
				// the listing would instead hide every account because a single
				// row is damaged. The warning is what makes the row findable,
				// since the update path now refuses to merge it.
				h.logger.Warn("Z.ai account metadata is not readable",
					"account", acc.Name, "id", acc.ID, "error", err)
			} else {
				if k, ok := meta["api_key"].(string); ok && k != "" {
					hasKey = true
				}
				if b, ok := meta["base_url"].(string); ok {
					baseURL, haveBaseURL = b, true
				}
			}
		}
		entry := map[string]interface{}{
			"id":        acc.ID,
			"name":      acc.Name,
			"createdAt": acc.CreatedAt.Format(time.RFC3339),
			"hasKey":    hasKey,
		}
		if haveBaseURL {
			entry["baseUrl"] = baseURL
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

// jsonStringValue renders s as a JSON string so it can be stored in a metadata
// map whose values are kept as raw JSON. Marshalling a string is infallible, so
// the dropped error here is genuinely dead rather than ignored.
func jsonStringValue(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return json.RawMessage(b)
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

	// The stored metadata is read before the restore and the rename below touch
	// anything, so a request that turns out to be unmergeable leaves the account
	// exactly as it was rather than half-updated.
	//
	// Issue #21: the parse error here used to be dropped. An unreadable blob
	// then merged as if it were empty, and an update carrying only base_url
	// wrote back metadata with no api_key. What that destroys is the last copy
	// of the credential, not the polling: the agent manager decodes this blob
	// too and had already stopped polling the account the moment it stopped
	// parsing. Before the overwrite the key is still there as text and can be
	// recovered from the row; afterwards there is nothing left to recover.
	//
	// Values are held as RawMessage rather than string. The agent manager reads
	// this same blob into a struct and ignores fields it does not know, so a
	// metadata object with any non-string value in it feeds a perfectly healthy
	// account; decoding into map[string]string would report an error for that
	// account and flatten every such value it did manage to read.
	var existing map[string]json.RawMessage
	if req.APIKey != nil || req.BaseURL != nil {
		existing = map[string]json.RawMessage{}
		if acc.Metadata != "" {
			if err := json.Unmarshal([]byte(acc.Metadata), &existing); err != nil {
				// Refusing is the point: the blob is the only copy of the key,
				// and merging into an empty map would drop it. The exception is
				// a request that supplies a key itself - there is then nothing
				// left to lose, and it is the only way an operator can repair
				// an account whose metadata has already been damaged.
				if req.APIKey == nil || *req.APIKey == "" {
					// The decoder reports the offending character or type, never
					// the payload, so nothing here can carry key material.
					h.logger.Error("refusing to merge unreadable Z.ai account metadata",
						"account", acc.Name, "id", id, "error", err)
					respondError(w, http.StatusConflict,
						"stored account metadata is not readable, so this update would drop the saved API key; re-send it with an api_key to discard the damaged metadata and start over, which also clears base_url and any other stored field")
					return
				}
				h.logger.Warn("replacing unreadable Z.ai account metadata: the request carries a new API key",
					"account", acc.Name, "id", id, "error", err)
				existing = map[string]json.RawMessage{}
			}
			if existing == nil {
				// Metadata was the JSON literal null, which decodes into a nil
				// map. Nothing to preserve, but the merge still needs a map.
				existing = map[string]json.RawMessage{}
			}
		}
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
	// not silently drop a custom base_url, and a field this handler has never
	// heard of must come back out the way it went in. The same condition as the
	// parse above, which is what guarantees existing is non-nil here.
	if req.APIKey != nil || req.BaseURL != nil {
		if req.APIKey != nil && *req.APIKey != "" {
			existing["api_key"] = jsonStringValue(*req.APIKey)
		}
		if req.BaseURL != nil {
			if *req.BaseURL == "" {
				delete(existing, "base_url")
			} else {
				existing["base_url"] = jsonStringValue(*req.BaseURL)
			}
		}
		metaJSON, err := json.Marshal(existing)
		if err != nil {
			h.logger.Error("failed to encode Z.ai account metadata", "account", acc.Name, "id", id, "error", err)
			respondError(w, http.StatusInternalServerError, "failed to update account")
			return
		}
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
	// The short window comes first: it is the one hit under heavy use, so the
	// operator has to see it immediately on the subscription card.
	for _, q := range []struct {
		key   string
		name  string
		label string
	}{
		{"tokensShortLimit", "tokens_short", ""},
		{"tokensLimit", "tokens", "Tokens Limit"},
		{"timeLimit", "time", "Time Limit"},
	} {
		raw, ok := current[q.key].(map[string]interface{})
		if !ok {
			continue
		}
		label := q.label
		if label == "" {
			// The short window's name arrives with the data: the window
			// length is a property of the subscription, not a dashboard
			// constant.
			if w, ok := raw["window"].(string); ok && w != "" {
				label = "Tokens Limit (" + w + ")"
			} else {
				label = "Tokens Limit (short)"
			}
		}
		row := map[string]interface{}{
			"name":         q.name,
			"label":        label,
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

// zaiUsageAccounts returns the current state of every active Z.ai
// subscription.
//
// Fork change: it feeds the "All" tab and the menu bar, which until now showed
// only the default subscription. The response shape mirrors
// minimaxUsageAccounts so the frontend can treat both providers identically.
func (h *Handler) zaiUsageAccounts() []map[string]interface{} {
	if h.store == nil {
		return []map[string]interface{}{}
	}
	accounts, err := h.store.QueryActiveProviderAccounts("zai")
	if err != nil {
		h.logger.Error("failed to query Z.ai accounts", "error", err)
		return []map[string]interface{}{}
	}

	result := make([]map[string]interface{}, 0, len(accounts))
	for _, acc := range accounts {
		current := h.buildZaiCurrent(acc.ID)
		current["accountId"] = acc.ID
		current["accountName"] = acc.Name
		result = append(result, current)
	}
	return result
}
