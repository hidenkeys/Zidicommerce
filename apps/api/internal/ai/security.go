package ai

import "strings"

type SecurityDecision struct {
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func classifySecurity(text string) SecurityDecision {
	normalized := normalizeAIText(text)
	checks := []struct {
		reason string
		terms  []string
	}{
		{"prompt_injection", []string{"ignore previous instructions", "ignore your previous instructions", "ignore all previous instructions", "ignore all instructions", "disregard previous instructions", "new instructions", "you are now"}},
		{"system_prompt_request", []string{"system prompt", "show me your prompt", "developer message", "internal instructions"}},
		{"database_request", []string{"raw database", "database schema", "sql schema", "sql query", "dump database", "all database records", "db records", "run sql", "select *", "select from"}},
		{"credential_request", []string{"api key", "password", "credentials", "secret key", "access token"}},
		{"tenant_data_request", []string{"all organizations", "every organization", "all merchants", "every merchant", "another merchant", "other organization", "other merchant", "another organization"}},
		{"customer_privacy_request", []string{"all customers", "every customer", "customer information", "customer data", "customer records"}},
		{"merchant_switch", []string{"switch merchant", "current merchant is", "you are serving", "search every organization", "ignore the current merchant", "shopping with another", "shopping with a different", "shopping with stridestreet", "shopping with glownest", "show me their products", "show me their shoes", "their products instead", "their shoes instead"}},
	}
	for _, check := range checks {
		for _, term := range check.terms {
			if strings.Contains(normalized, term) {
				return SecurityDecision{
					Blocked: true,
					Reason:  check.reason,
					Message: "I can't help with requests to bypass instructions, access internal data, or view another merchant's information. I can help with this merchant's products, stores, policies, and your own order details.",
				}
			}
		}
	}
	return SecurityDecision{}
}
