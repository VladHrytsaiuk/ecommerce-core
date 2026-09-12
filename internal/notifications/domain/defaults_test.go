package domain

import (
	"html/template"
	"strings"
	"testing"
	textTemplate "text/template"
)

// notification_templates shipped empty and nothing seeded it, so every lookup
// failed with "template unavailable", every job retried ten times and died, and
// a store could take orders without sending one confirmation. These defaults
// are the floor that stops that, which makes two things worth holding: that a
// template exists for every key the system can ask for, and that each one
// actually renders.

func TestEveryTemplateKeyTheSystemCanAskForHasADefault(t *testing.T) {
	// A key added without a default is mail that will never leave.
	wanted := []string{
		OrderPaidTemplate,
		BackInStockTemplate,
		SupportAgentReplyTemplate,
		AbandonedCartTemplate,
		ReturnApprovedTemplate,
		ReturnRejectedTemplate,
		ReturnReceivedTemplate,
		ReturnRefundedTemplate,
	}
	present := make(map[string]struct{}, len(DefaultTemplates))
	for _, candidate := range DefaultTemplates {
		if _, duplicate := present[candidate.Key]; duplicate {
			t.Fatalf("two defaults claim the key %q", candidate.Key)
		}
		present[candidate.Key] = struct{}{}
	}
	for _, key := range wanted {
		if _, ok := present[key]; !ok {
			t.Fatalf("no default template for %q; every job asking for it dies after ten attempts", key)
		}
	}
	if len(DefaultTemplates) != len(wanted) {
		t.Fatalf("%d defaults for %d keys; a default nothing asks for is dead weight", len(DefaultTemplates), len(wanted))
	}
}

func TestEveryDefaultTemplateParses(t *testing.T) {
	// A template that fails to parse is caught at send time, one job at a time,
	// after the store is already live.
	for _, candidate := range DefaultTemplates {
		t.Run(candidate.Key, func(t *testing.T) {
			if strings.TrimSpace(candidate.Subject) == "" || strings.TrimSpace(candidate.Text) == "" || strings.TrimSpace(candidate.HTML) == "" {
				t.Fatalf("template %q has an empty part: %+v", candidate.Key, candidate)
			}
			if _, err := textTemplate.New("subject").Parse(candidate.Subject); err != nil {
				t.Fatalf("subject does not parse: %v", err)
			}
			if _, err := textTemplate.New("text").Parse(candidate.Text); err != nil {
				t.Fatalf("text body does not parse: %v", err)
			}
			if _, err := template.New("html").Parse(candidate.HTML); err != nil {
				t.Fatalf("html body does not parse: %v", err)
			}
		})
	}
}

func TestATemplateOnlyReferencesFieldsItsSenderSupplies(t *testing.T) {
	// The render data differs per key. A default naming a field its caller does
	// not pass renders as "<no value>" in a live customer email.
	supplied := map[string][]string{
		OrderPaidTemplate:         {"OrderNumber"},
		SupportAgentReplyTemplate: {"Subject", "MessageBody"},
	}
	for _, candidate := range DefaultTemplates {
		fields := supplied[candidate.Key]
		for _, body := range []string{candidate.Subject, candidate.Text, candidate.HTML} {
			for _, reference := range placeholders(body) {
				if !contains(fields, reference) {
					t.Fatalf("template %q refers to {{.%s}}, which its sender does not supply", candidate.Key, reference)
				}
			}
		}
	}
}

func placeholders(body string) []string {
	var found []string
	for _, part := range strings.Split(body, "{{.")[1:] {
		if end := strings.IndexAny(part, "}| "); end > 0 {
			found = append(found, part[:end])
		}
	}
	return found
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
