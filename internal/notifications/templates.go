package notifications

import "strings"

type templateDefinition struct {
	Title string
	Body  string
}

var templates = map[string]templateDefinition{
	EventApprovalExpiring:    {Title: "Approval expiring soon"},
	EventApprovalExpired:     {Title: "Approval expired"},
	EventTokenAbuse:          {Title: "Possible approval token abuse"},
	EventPolicyInvalidOutput: {Title: "Policy invalid output"},
	EventPolicyEvalError:     {Title: "Policy evaluation error"},
	"NOTIFICATIONS.TEST":     {Title: "Notification test"},
}

func BuildMessage(eventType string, data map[string]string) NotificationMessage {
	template := templates[eventType]
	title := template.Title
	if title == "" {
		title = eventType
	}
	body := template.Body
	if body == "" {
		if summary := data["summary"]; summary != "" {
			body = summary
		} else if reason := data["reason"]; reason != "" {
			body = reason
		}
	}
	fields := make(map[string]string)
	for key, value := range data {
		if key == "summary" || key == "reason" {
			continue
		}
		if value == "" {
			continue
		}
		fields[normalizeFieldKey(key)] = value
	}
	return NotificationMessage{
		Title:  title,
		Body:   body,
		Fields: fields,
	}
}

func normalizeFieldKey(value string) string {
	if value == "" {
		return value
	}
	return strings.TrimSpace(value)
}
