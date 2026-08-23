package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DeclaredAccount is one provider subscription named in configuration rather
// than created by clicking through the panel.
//
// The credential is not here. The field names the environment variable holding
// it, so the accounts list can live in a Helm values file, in git, in a code
// review, while the key itself stays in whatever secret store the deployment
// already uses. Putting the key in the list would make the list unshareable,
// and an unshareable list is one that goes back to being clicked in by hand.
type DeclaredAccount struct {
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	APIKeyEnv string `json:"apiKeyEnv"`
	BaseURL   string `json:"baseUrl,omitempty"`
}

// APIKey resolves the credential this account points at.
func (a DeclaredAccount) APIKey() string {
	if a.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(a.APIKeyEnv)
}

// Validate rejects an entry that cannot describe a real account.
//
// It is deliberately strict about the environment variable being set. An entry
// whose credential is missing would otherwise create the account, fail to give
// it a key, and leave it sitting in the panel looking configured while nothing
// polls it - which is the failure this whole feature exists to prevent.
func (a DeclaredAccount) Validate() error {
	switch {
	case a.Provider == "":
		return fmt.Errorf("provider is required")
	case a.Name == "":
		return fmt.Errorf("name is required for provider %q", a.Provider)
	case !declarableProviders[a.Provider]:
		return fmt.Errorf("provider %q does not keep per-account credentials; "+
			"declarable providers are %s", a.Provider, declarableProviderList())
	case a.APIKeyEnv == "":
		return fmt.Errorf("%s/%s: apiKeyEnv is required - it names the environment "+
			"variable holding the key, so the key itself never enters this list",
			a.Provider, a.Name)
	case a.APIKey() == "":
		return fmt.Errorf("%s/%s: %s is empty or unset; the account would be created "+
			"without a credential and would sit there looking configured while nothing "+
			"polls it", a.Provider, a.Name, a.APIKeyEnv)
	}
	return nil
}

// declarableProviders are the ones that keep credentials per account. The rest
// take a single key from the environment and have one account by construction,
// so declaring several of them would describe something that cannot exist.
var declarableProviders = map[string]bool{
	"zai":     true,
	"minimax": true,
}

func declarableProviderList() string {
	names := make([]string, 0, len(declarableProviders))
	for p := range declarableProviders {
		names = append(names, p)
	}
	// Stable order: an error message that reorders itself between runs is one
	// people stop reading carefully.
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return strings.Join(names, ", ")
}

// ParseDeclaredAccounts reads the accounts list out of LOOMWATCH_ACCOUNTS.
//
// Every entry is validated before any of them is returned. A list that is half
// applied is worse than one that is rejected: the operator sees some
// subscriptions appear, some not, and no single place saying why.
func ParseDeclaredAccounts(raw string) ([]DeclaredAccount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil, nil
	}

	var accounts []DeclaredAccount
	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		return nil, fmt.Errorf("LOOMWATCH_ACCOUNTS is not a JSON array of accounts: %w", err)
	}

	seen := map[string]bool{}
	for _, a := range accounts {
		if err := a.Validate(); err != nil {
			return nil, err
		}
		key := a.Provider + "/" + a.Name
		if seen[key] {
			return nil, fmt.Errorf("%s is declared twice; one name is one account, and "+
				"two entries for it would race to write the same credential", key)
		}
		seen[key] = true
	}
	return accounts, nil
}
