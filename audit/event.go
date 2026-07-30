package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	SpecVersion       = "1.0"
	DataContentType   = "application/json"
	MaxEventBytes     = 64 * 1024
	MaxMetadataBytes  = 32 * 1024
	MaxUserAgentBytes = 512
	MaxReasonBytes    = 512
)

type Category string

const (
	CategoryAuthentication Category = "authentication"
	CategoryAuthorization  Category = "authorization"
	CategoryAdministration Category = "administration"
	CategoryBilling        Category = "billing"
	CategoryData           Category = "data"
	CategoryContent        Category = "content"
	CategoryConfiguration  Category = "configuration"
	CategorySecurity       Category = "security"
	CategoryAudit          Category = "audit"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type ActorType string

const (
	ActorUser      ActorType = "user"
	ActorService   ActorType = "service"
	ActorSystem    ActorType = "system"
	ActorAnonymous ActorType = "anonymous"
)

type Actor struct {
	Type  ActorType `json:"type"`
	ID    string    `json:"id,omitempty"`
	Roles []string  `json:"roles,omitempty"`
}

type Target struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type Request struct {
	RequestID     string `json:"request_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	IPAddress     string `json:"ip_address,omitempty"`
	UserAgent     string `json:"user_agent,omitempty"`
	SessionHash   string `json:"session_hash,omitempty"`
}

type Change struct {
	Field  string `json:"field"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

type Data struct {
	Category   Category       `json:"category"`
	Outcome    Outcome        `json:"outcome"`
	Severity   Severity       `json:"severity"`
	Component  string         `json:"component"`
	Actor      Actor          `json:"actor"`
	Target     Target         `json:"target"`
	Request    Request        `json:"request,omitempty"`
	ReasonCode string         `json:"reason_code,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Changes    []Change       `json:"changes,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Event is a CloudEvents 1.0 structured JSON event carrying an Edadai audit payload.
type Event struct {
	SpecVersion     string    `json:"specversion"`
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	Subject         string    `json:"subject,omitempty"`
	Time            time.Time `json:"time"`
	DataContentType string    `json:"datacontenttype"`
	DataSchema      string    `json:"dataschema,omitempty"`
	Data            Data      `json:"data"`
}

var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func NewEvent(service, environment, action string, data Data, at time.Time) (Event, error) {
	service = normalizeSegment(service)
	environment = normalizeSegment(environment)
	action = strings.TrimSpace(action)
	if service == "" || environment == "" || !actionPattern.MatchString(action) {
		return Event{}, errors.New("invalid audit event source or action")
	}
	data.Request.UserAgent = truncateUTF8(data.Request.UserAgent, MaxUserAgentBytes)
	data.Reason = truncateUTF8(data.Reason, MaxReasonBytes)
	data.Changes = RedactChanges(data.Changes)
	if data.Metadata != nil {
		if redacted, ok := Redact(data.Metadata).(map[string]any); ok {
			data.Metadata = redacted
		}
	}

	event := Event{
		SpecVersion:     SpecVersion,
		ID:              uuid.Must(uuid.NewV7()).String(),
		Source:          fmt.Sprintf("urn:edadai:%s:%s", service, environment),
		Type:            fmt.Sprintf("com.edadai.audit.%s.%s.v1", data.Category, action),
		Time:            at.UTC(),
		DataContentType: DataContentType,
		DataSchema:      "https://edad.ai/schemas/audit-event-data-v1.json",
		Data:            data,
	}
	if data.Target.Type != "" {
		event.Subject = data.Target.Type
		if data.Target.ID != "" {
			event.Subject += "/" + data.Target.ID
		}
	}
	return event, event.Validate()
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	end := maximum
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func (e Event) Validate() error {
	if e.SpecVersion != SpecVersion || e.DataContentType != DataContentType {
		return errors.New("unsupported CloudEvents version or content type")
	}
	eventID, err := uuid.Parse(e.ID)
	if err != nil || eventID.Version() != 7 {
		return errors.New("audit event id must be a UUIDv7")
	}
	source, err := url.Parse(e.Source)
	if err != nil || source.Scheme != "urn" || !strings.HasPrefix(e.Source, "urn:edadai:") {
		return errors.New("invalid audit event source")
	}
	if e.Time.IsZero() {
		return errors.New("audit event time is required")
	}
	if !validCategory(e.Data.Category) || !validOutcome(e.Data.Outcome) || !validSeverity(e.Data.Severity) {
		return errors.New("invalid audit category, outcome, or severity")
	}
	if !validActorType(e.Data.Actor.Type) {
		return errors.New("invalid audit actor type")
	}
	if strings.TrimSpace(e.Data.Component) == "" || strings.TrimSpace(e.Data.Target.Type) == "" {
		return errors.New("audit component and target type are required")
	}
	if len(e.Data.Request.UserAgent) > MaxUserAgentBytes || len(e.Data.Reason) > MaxReasonBytes {
		return errors.New("audit request or reason exceeds size limit")
	}
	if e.Data.Request.IPAddress != "" && net.ParseIP(e.Data.Request.IPAddress) == nil {
		return errors.New("invalid audit source IP")
	}
	metadata, err := json.Marshal(e.Data.Metadata)
	if err != nil || len(metadata) > MaxMetadataBytes {
		return errors.New("audit metadata is invalid or too large")
	}
	body, err := json.Marshal(e)
	if err != nil || len(body) > MaxEventBytes {
		return errors.New("audit event is invalid or too large")
	}
	return nil
}

func (e Event) Action() string {
	prefix := fmt.Sprintf("com.edadai.audit.%s.", e.Data.Category)
	if !strings.HasPrefix(e.Type, prefix) || !strings.HasSuffix(e.Type, ".v1") {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(e.Type, prefix), ".v1")
}

func (e Event) Service() string {
	parts := strings.Split(e.Source, ":")
	if len(parts) != 4 {
		return ""
	}
	return parts[2]
}

func RoutingKey(service string, category Category) string {
	return fmt.Sprintf("audit.%s.%s.v1", normalizeSegment(service), category)
}

func normalizeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func validCategory(value Category) bool {
	switch value {
	case CategoryAuthentication, CategoryAuthorization, CategoryAdministration, CategoryBilling,
		CategoryData, CategoryContent, CategoryConfiguration, CategorySecurity, CategoryAudit:
		return true
	default:
		return false
	}
}

func validOutcome(value Outcome) bool {
	return value == OutcomeSuccess || value == OutcomeFailure || value == OutcomeDenied
}

func validSeverity(value Severity) bool {
	return value == SeverityInfo || value == SeverityWarning || value == SeverityHigh || value == SeverityCritical
}

func validActorType(value ActorType) bool {
	return value == ActorUser || value == ActorService || value == ActorSystem || value == ActorAnonymous
}
