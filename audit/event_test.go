package audit

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestBillingGoldenFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/billing-plan-updated-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if event.Action() != "catalog.plan.update" || RoutingKey(event.Service(), event.Data.Category) != "audit.billing-service.billing.v1" {
		t.Fatalf("unexpected golden contract: %#v", event)
	}
}

func TestNewEventProducesValidCloudEvent(t *testing.T) {
	event, err := NewEvent("auth-service", "test", "session.login", Data{
		Category:  CategoryAuthentication,
		Outcome:   OutcomeSuccess,
		Severity:  SeverityInfo,
		Component: "sessions",
		Actor:     Actor{Type: ActorUser, ID: "user-1"},
		Target:    Target{Type: "session", ID: "session-1"},
	}, time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if event.Action() != "session.login" || event.Service() != "auth-service" {
		t.Fatalf("unexpected event identity: %#v", event)
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > MaxEventBytes {
		t.Fatal("event exceeded maximum size")
	}
}

func TestNewEventTruncatesBoundedText(t *testing.T) {
	event, err := NewEvent("api", "test", "audit.event.viewed", Data{
		Category: CategoryAudit, Outcome: OutcomeSuccess, Severity: SeverityInfo, Component: "admin",
		Actor: Actor{Type: ActorUser}, Target: Target{Type: "audit_event"},
		Request: Request{UserAgent: string(make([]byte, MaxUserAgentBytes+20))},
		Reason:  string(make([]byte, MaxReasonBytes+20)),
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(event.Data.Request.UserAgent) != MaxUserAgentBytes || len(event.Data.Reason) != MaxReasonBytes {
		t.Fatal("bounded fields were not truncated")
	}
}

func TestRedactRecursivelyRemovesSecrets(t *testing.T) {
	got := Redact(map[string]any{
		"safe":   "value",
		"nested": map[string]any{"access_token": "secret", "items": []any{map[string]any{"cookie": "secret"}}},
	}).(map[string]any)
	nested := got["nested"].(map[string]any)
	if nested["access_token"] != "[REDACTED]" {
		t.Fatalf("secret was not redacted: %#v", got)
	}
	items := nested["items"].([]any)
	if items[0].(map[string]any)["cookie"] != "[REDACTED]" {
		t.Fatalf("secret in array was not redacted: %#v", got)
	}

	changes := RedactChanges([]Change{
		{Field: "password", Before: "old", After: "new"},
		{Field: "profile", After: map[string]any{"refresh_token": "secret"}},
	})
	if changes[0].After != "[REDACTED]" || changes[1].After.(map[string]any)["refresh_token"] != "[REDACTED]" {
		t.Fatalf("changes were not recursively redacted: %#v", changes)
	}
}
