package runtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

type RuntimeContext struct {
	Session   ConversationSession
	Variables map[string]any
	System    map[string]any
}

func (c RuntimeContext) Resolve(path string) (any, bool) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$.")
	if path == "" {
		return nil, false
	}
	if strings.HasPrefix(path, "variables.") {
		return lookupPath(c.Variables, strings.TrimPrefix(path, "variables."))
	}
	if value, ok := lookupPath(c.System, path); ok {
		return value, true
	}
	if value, ok := lookupPath(c.Variables, path); ok {
		return value, true
	}
	switch path {
	case "session.id":
		return c.Session.ID.String(), true
	case "session.status":
		return c.Session.Status, true
	default:
		return nil, false
	}
}

func (c RuntimeContext) Render(template string, fallback string) string {
	if template == "" {
		return ""
	}
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return fallback
		}
		value, ok := c.Resolve(parts[1])
		if !ok || value == nil {
			return fallback
		}
		return fmt.Sprint(value)
	})
}

func parseJSONMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func jsonMap(value map[string]any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func jsonValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func lookupPath(root map[string]any, path string) (any, bool) {
	current := any(root)
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, false
		}
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = asMap[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setPath(root map[string]any, path string, value any) {
	path = strings.TrimPrefix(strings.TrimSpace(path), "variables.")
	if path == "" {
		return
	}
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		if next, ok := current[part].(map[string]any); ok {
			current = next
			continue
		}
		next := map[string]any{}
		current[part] = next
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
