package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type FlowCompilation struct {
	JSON     map[string]any    `json:"json"`
	Errors   []ValidationIssue `json:"errors"`
	Warnings []ValidationIssue `json:"warnings"`
}

func FlowScreenID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "S_" + strings.ToUpper(hex.EncodeToString(sum[:6]))
}

// Commerce actions and conditions execute on the data endpoint, never in the
// device. Every rendered screen exchanges one answer with the same interpreter.
func CompileJourneyFlow(d JourneyDefinition) FlowCompilation {
	r := FlowCompilation{JSON: map[string]any{}, Errors: []ValidationIssue{}, Warnings: []ValidationIssue{}}
	validation := ValidateJourney(d)
	r.Errors = append(r.Errors, validation.Issues...)
	steps := map[string]JourneyStep{}
	for _, j := range d.Steps {
		steps[j.Key] = j
	}
	screens := []map[string]any{}
	routing := map[string][]string{}
	var nextScreens func(string, map[string]bool) map[string]bool
	nextScreens = func(key string, seen map[string]bool) map[string]bool {
		out := map[string]bool{}
		if seen[key] {
			return out
		}
		seen[key] = true
		j, ok := steps[key]
		if !ok {
			return out
		}
		if isJourneyQuestion(j) {
			out[FlowScreenID(key)] = true
			return out
		}
		if j.Kind == "end" || j.Kind == "handoff" {
			out["FINISH"] = true
			return out
		}
		for _, e := range journeyEdges(j) {
			for s := range nextScreens(e, seen) {
				out[s] = true
			}
		}
		return out
	}
	for _, j := range d.Steps {
		if !isJourneyQuestion(j) {
			continue
		}
		if j.Kind == "entry_menu" {
			r.Errors = append(r.Errors, journeyError(j.Key, "Use guided chat for the entry menu; its choices can launch separately published Flows"))
			continue
		}
		if j.AnswerType == "location" || j.AnswerType == "media" {
			r.Errors = append(r.Errors, journeyError(j.Key, "This request is available in guided chat; use a text address/details field for Flow mode"))
			continue
		}
		id := FlowScreenID(j.Key)
		data := map[string]any{"prompt": map[string]any{"type": "string", "__example__": j.Text}, "error": map[string]any{"type": "string", "__example__": ""}, "turn": map[string]any{"type": "string", "__example__": "preview"}, "options": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}}}, "__example__": []map[string]any{{"id": "example", "title": "Example option"}}}}
		label := truncateFlowLabel(j.Title, 20)
		if label != j.Title {
			r.Warnings = append(r.Warnings, journeyError(j.Key, "Flow field label is shortened to 20 characters; the full customer question remains visible"))
		}
		component := map[string]any{"type": "TextInput", "name": "answer", "label": label, "required": j.Required, "input-type": "text"}
		if j.Source != "" || len(j.Options) > 0 {
			component = map[string]any{"type": "Dropdown", "name": "answer", "label": label, "required": j.Required, "data-source": "${data.options}"}
		} else {
			switch j.AnswerType {
			case "number":
				component["input-type"] = "number"
			case "email":
				component["input-type"] = "email"
			case "phone":
				component["input-type"] = "phone"
			case "date":
				component = map[string]any{"type": "DatePicker", "name": "answer", "label": label, "required": j.Required}
			case "yes_no":
				component = map[string]any{"type": "RadioButtonsGroup", "name": "answer", "label": label, "required": j.Required, "data-source": []map[string]string{{"id": "yes", "title": "Yes"}, {"id": "no", "title": "No"}}}
			}
			if j.Kind == "quantity" {
				component["input-type"] = "number"
			}
		}
		children := []map[string]any{{"type": "TextBody", "text": "${data.prompt}"}, {"type": "TextBody", "text": "${data.error}"}, component, {"type": "Footer", "label": "Continue", "on-click-action": map[string]any{"name": "data_exchange", "payload": map[string]any{"answer": "${form.answer}", "turn": "${data.turn}"}}}}
		if j.Help != "" {
			children = append(children[:2], append([]map[string]any{{"type": "TextCaption", "text": j.Help}}, children[2:]...)...)
		}
		screens = append(screens, map[string]any{"id": id, "title": truncateFlowLabel(j.Title, 30), "refresh_on_back": true, "data": data, "layout": map[string]any{"type": "SingleColumnLayout", "children": []map[string]any{{"type": "Form", "name": "form", "children": children}}}})
		targets := map[string]bool{}
		for _, edge := range journeyEdges(j) {
			for target := range nextScreens(edge, map[string]bool{}) {
				targets[target] = true
			}
		}
		routing[id] = sortedJourneyKeys(targets)
		if j.Min != nil || j.Max != nil {
			r.Warnings = append(r.Warnings, journeyError(j.Key, "Numeric limits are enforced by the server before navigation"))
		}
		if j.VisibleWhen != "" {
			r.Warnings = append(r.Warnings, journeyError(j.Key, "Conditional visibility is evaluated by the data endpoint"))
		}
	}
	if len(screens) == 0 {
		r.Errors = append(r.Errors, journeyError("", "A Flow needs at least one customer input screen"))
	}
	if len(screens) > 30 {
		r.Errors = append(r.Errors, journeyError("", "Split this journey into fewer than 30 customer screens"))
	}
	screens = append(screens, map[string]any{"id": "FINISH", "title": "Request received", "terminal": true, "success": true, "data": map[string]any{"prompt": map[string]any{"type": "string", "__example__": "Your request has been received."}}, "layout": map[string]any{"type": "SingleColumnLayout", "children": []map[string]any{{"type": "TextBody", "text": "${data.prompt}"}, {"type": "Footer", "label": "Done", "on-click-action": map[string]any{"name": "complete", "payload": map[string]any{"completed": true}}}}}})
	routing["FINISH"] = []string{}
	// Meta permits refresh of the current screen and implicit reverse navigation,
	// but its routing model must declare only forward edges, never self routes.
	rank := map[string]int{}
	visited := map[string]bool{}
	var orderScreens func(string)
	orderScreens = func(key string) {
		if visited[key] {
			return
		}
		visited[key] = true
		j, ok := steps[key]
		if !ok {
			return
		}
		if isJourneyQuestion(j) {
			rank[FlowScreenID(key)] = len(rank)
		}
		for _, next := range journeyEdges(j) {
			orderScreens(next)
		}
	}
	orderScreens(d.Start)
	rank["FINISH"] = len(rank)
	forward := map[string]map[string]bool{}
	for id := range routing {
		forward[id] = map[string]bool{}
	}
	for _, j := range d.Steps {
		id := FlowScreenID(j.Key)
		for _, target := range routing[id] {
			if target == id {
				r.Warnings = append(r.Warnings, journeyError(j.Key, "Repeated answers refresh this screen through the data endpoint"))
				continue
			}
			from, to := id, target
			if rank[from] > rank[to] {
				from, to = to, from
				r.Warnings = append(r.Warnings, journeyError(j.Key, "Return navigation uses the implicit reverse route; verify this branch in the Meta test preview"))
			}
			if forward[from] != nil {
				forward[from][to] = true
			}
		}
	}
	for id, targets := range forward {
		routing[id] = sortedJourneyKeys(targets)
		if len(targets) > 10 {
			r.Errors = append(r.Errors, journeyError("", "A Flow screen can have at most 10 forward routes; simplify the branches"))
		}
	}
	r.JSON = map[string]any{"version": "7.3", "data_api_version": "3.0", "routing_model": routing, "screens": screens}
	return r
}
func truncateFlowLabel(value string, max int) string {
	r := []rune(value)
	if len(r) > max {
		return string(r[:max])
	}
	return value
}
