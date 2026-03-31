package errors

import (
	"strconv"
	"strings"
)

// InstantDisableRule defines a single rule for instant key disabling.
type InstantDisableRule struct {
	Type  string // "status" or "keyword"
	Value string // HTTP status code string or keyword substring
}

// ParseInstantDisableRules parses a multi-line rules text into structured rules.
// Format: one rule per line, "status:401" for HTTP status codes, "keyword:invalid_api_key" for message keywords.
func ParseInstantDisableRules(rulesText string) []InstantDisableRule {
	if strings.TrimSpace(rulesText) == "" {
		return nil
	}

	lines := strings.Split(rulesText, "\n")
	rules := make([]InstantDisableRule, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}

		ruleType := strings.TrimSpace(line[:colonIdx])
		ruleValue := strings.TrimSpace(line[colonIdx+1:])
		if ruleValue == "" {
			continue
		}

		switch ruleType {
		case "status":
			if _, err := strconv.Atoi(ruleValue); err == nil {
				rules = append(rules, InstantDisableRule{Type: "status", Value: ruleValue})
			}
		case "keyword":
			rules = append(rules, InstantDisableRule{Type: "keyword", Value: strings.ToLower(ruleValue)})
		}
	}

	return rules
}

// ShouldInstantDisable checks if the given status code and error message match any instant disable rule.
func ShouldInstantDisable(rules []InstantDisableRule, statusCode int, errorMsg string) bool {
	if len(rules) == 0 {
		return false
	}

	statusStr := strconv.Itoa(statusCode)
	errorLower := strings.ToLower(errorMsg)

	for _, rule := range rules {
		switch rule.Type {
		case "status":
			if statusStr == rule.Value {
				return true
			}
		case "keyword":
			if errorLower != "" && strings.Contains(errorLower, rule.Value) {
				return true
			}
		}
	}

	return false
}
