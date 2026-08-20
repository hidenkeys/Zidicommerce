package runtime

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
)

type conditionRule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

func evaluateCondition(condition bot.Condition, ctx RuntimeContext) (bool, error) {
	var rules []conditionRule
	if err := json.Unmarshal([]byte(defaultArray(condition.Rules)), &rules); err != nil {
		return false, runtimeErrorf(ErrRuntimeConfigurationError, "This bot is not configured correctly.", "invalid condition rules for %s", condition.ConditionKey)
	}
	if len(rules) == 0 {
		return false, runtimeErrorf(ErrRuntimeConfigurationError, "This bot is not configured correctly.", "empty condition rules for %s", condition.ConditionKey)
	}
	combinator := strings.ToLower(strings.TrimSpace(condition.Combinator))
	if combinator == "" {
		combinator = "and"
	}
	matched := combinator == "and"
	for _, rule := range rules {
		result := evaluateRule(rule, ctx)
		if combinator == "or" && result {
			return true, nil
		}
		if combinator == "and" && !result {
			return false, nil
		}
		matched = result
	}
	return matched, nil
}

func evaluateRule(rule conditionRule, ctx RuntimeContext) bool {
	actual, exists := ctx.Resolve(rule.Field)
	switch strings.ToLower(strings.TrimSpace(rule.Operator)) {
	case "exists":
		return exists
	case "not_exists":
		return !exists
	case "equals":
		return exists && stringValue(actual) == stringValue(rule.Value)
	case "not_equals":
		return !exists || stringValue(actual) != stringValue(rule.Value)
	case "contains":
		return exists && strings.Contains(strings.ToLower(stringValue(actual)), strings.ToLower(stringValue(rule.Value)))
	case "greater_than":
		left, leftOK := numberValue(actual)
		right, rightOK := numberValue(rule.Value)
		return exists && leftOK && rightOK && left > right
	case "less_than":
		left, leftOK := numberValue(actual)
		right, rightOK := numberValue(rule.Value)
		return exists && leftOK && rightOK && left < right
	case "in":
		return exists && valueIn(actual, rule.Value)
	case "not_in":
		return !exists || !valueIn(actual, rule.Value)
	default:
		return false
	}
}

func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func valueIn(actual any, expected any) bool {
	switch v := expected.(type) {
	case []any:
		for _, item := range v {
			if stringValue(item) == stringValue(actual) {
				return true
			}
		}
		return false
	case []string:
		return slices.Contains(v, stringValue(actual))
	default:
		for _, item := range strings.Split(stringValue(expected), ",") {
			if strings.TrimSpace(item) == stringValue(actual) {
				return true
			}
		}
		return false
	}
}
