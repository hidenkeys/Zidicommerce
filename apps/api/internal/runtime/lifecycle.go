package runtime

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
)

const (
	lifecycleTrackEmpty = "lifecycle:track_empty"
	maxInvalidRetries   = 3
)

type lifecycleCommand string

const (
	commandNone     lifecycleCommand = ""
	commandGreeting lifecycleCommand = "greeting"
	commandMenu     lifecycleCommand = "menu"
	commandRestart  lifecycleCommand = "restart"
	commandCancel   lifecycleCommand = "cancel"
	commandBack     lifecycleCommand = "back"
)

var (
	greetingExact = map[string]struct{}{
		"hi": {}, "hii": {}, "hiii": {}, "hello": {}, "helloo": {}, "hey": {}, "heyy": {}, "heyyy": {}, "heya": {}, "hiya": {},
		"howdy": {}, "yo": {}, "sup": {}, "start": {}, "restart": {}, "reset": {}, "menu": {}, "main menu": {},
		"good morning": {}, "good afternoon": {}, "good evening": {}, "good night": {},
	}
	leadingGreeting = regexp.MustCompile(`^(?i)(hi+|hey+|hello+)\b`)
)

func classifyCommand(text string) lifecycleCommand {
	normalized := normalizeCommand(text)
	if normalized == "" {
		return commandNone
	}
	switch normalized {
	case "start", "restart", "reset":
		return commandRestart
	case "menu", "main menu":
		return commandMenu
	case "cancel", "stop", "quit", "end":
		return commandCancel
	case "back", "go back", "previous":
		return commandBack
	}
	if _, ok := greetingExact[normalized]; ok {
		return commandGreeting
	}
	if leadingGreeting.MatchString(strings.TrimSpace(text)) && len(strings.Fields(normalized)) <= 4 {
		return commandGreeting
	}
	return commandNone
}

func isHardResetText(text string) bool {
	return classifyCommand(text) == commandRestart
}

func isResetText(text string) bool {
	switch classifyCommand(text) {
	case commandRestart, commandMenu, commandGreeting:
		return true
	default:
		return false
	}
}

func normalizeCommand(text string) string {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	trimmed = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return -1
		}
		return r
	}, trimmed)
	return strings.Join(strings.Fields(trimmed), " ")
}

func truncateLogText(text string, max int) string {
	trimmed := strings.TrimSpace(text)
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "…"
}

func unrecognizedOptionMessage() string {
	return "I didn't recognize that option.\n\nPlease choose one of the options above, or type 'menu' to return to the main menu."
}

func emptyOrdersMessage() string {
	return "Sorry, I couldn't find any recent orders for this number.\n\nYou can:\n1. Place an order\n2. Contact support\n3. Return to the main menu"
}

func trackEmptyOptions() []MessageOption {
	return []MessageOption{
		{ID: "order", Label: "Place an order"},
		{ID: "support", Label: "Contact support"},
		{ID: "menu", Label: "Main menu"},
	}
}

func returnToEntry(snapshot bot.VersionConfiguration, session *ConversationSession, runtimeContext RuntimeContext) {
	session.CurrentStepKey = snapshot.Version.StartStepKey
	session.ExpectedInput = ""
	session.Status = SessionActive
	runtimeContext.System["invalid_input_retries"] = 0
	delete(runtimeContext.System, "module_stack")
	session.SystemContext = jsonMap(runtimeContext.System)
}

func stepByKey(snapshot bot.VersionConfiguration, key string) (bot.Step, bool) {
	if strings.TrimSpace(key) == "" {
		return bot.Step{}, false
	}
	for _, step := range snapshot.Steps {
		if step.StepKey == key {
			return step, true
		}
	}
	return bot.Step{}, false
}

func skipSelectableQuestion(snapshot bot.VersionConfiguration, currentNext string) string {
	step, ok := stepByKey(snapshot, currentNext)
	if !ok {
		return currentNext
	}
	if (step.Type == bot.StepQuestion || step.Type == bot.StepChoice) && strings.TrimSpace(step.NextStepKey) != "" {
		return step.NextStepKey
	}
	return currentNext
}

func findHandoffStep(snapshot bot.VersionConfiguration) string {
	for _, step := range snapshot.Steps {
		if step.Type == bot.StepHandoff {
			return step.StepKey
		}
	}
	return ""
}

func findStepKeyByModule(snapshot bot.VersionConfiguration, moduleKey string) string {
	want := strings.ToUpper(strings.TrimSpace(moduleKey))
	for _, module := range snapshot.Modules {
		if strings.ToUpper(module.ModuleKey) != want {
			continue
		}
		params := parseJSONMap(module.Parameters)
		if entry := strings.TrimSpace(stringValue(params["entry_step"])); entry != "" {
			return entry
		}
	}
	for _, step := range snapshot.Steps {
		if step.Type != bot.StepModule || step.ModuleID == nil {
			continue
		}
		for _, module := range snapshot.Modules {
			if module.ID == *step.ModuleID && strings.ToUpper(module.ModuleKey) == want {
				return step.StepKey
			}
		}
	}
	return ""
}
