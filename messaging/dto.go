package messaging

import (
	"encoding/json"
	"time"
)

const (
	DefaultExchange           = "edad.events"
	DefaultDeadLetterExchange = "edad.events.dlx"

	NotificationRequestedRoutingKey = "notification.requested.v1"
	NotificationDeliveryRoutingKey  = "notification.delivery.requested.v1"

	StudyDocumentExtractionRequestedRoutingKey   = "study.document.extraction.requested.v1"
	StudyManualGenerationRequestedRoutingKey     = "study.manual.generation.requested.v1"
	StudyQuizGenerationRequestedRoutingKey       = "study.quiz.generation.requested.v1"
	StudyFlashcardsGenerationRequestedRoutingKey = "study.flashcards.generation.requested.v1"

	StudyDocumentExtractionCompletedRoutingKey   = "study.document.extraction.completed.v1"
	StudyDocumentExtractionFailedRoutingKey      = "study.document.extraction.failed.v1"
	StudyManualGenerationCompletedRoutingKey     = "study.manual.generation.completed.v1"
	StudyManualGenerationFailedRoutingKey        = "study.manual.generation.failed.v1"
	StudyQuizGenerationCompletedRoutingKey       = "study.quiz.generation.completed.v1"
	StudyQuizGenerationFailedRoutingKey          = "study.quiz.generation.failed.v1"
	StudyFlashcardsGenerationCompletedRoutingKey = "study.flashcards.generation.completed.v1"
	StudyFlashcardsGenerationFailedRoutingKey    = "study.flashcards.generation.failed.v1"
)

var StudyAIResultRoutingKeys = []string{
	StudyDocumentExtractionCompletedRoutingKey,
	StudyDocumentExtractionFailedRoutingKey,
	StudyManualGenerationCompletedRoutingKey,
	StudyManualGenerationFailedRoutingKey,
	StudyQuizGenerationCompletedRoutingKey,
	StudyQuizGenerationFailedRoutingKey,
	StudyFlashcardsGenerationCompletedRoutingKey,
	StudyFlashcardsGenerationFailedRoutingKey,
}

type EventEnvelope struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	AggregateID string          `json:"aggregate_id,omitempty"`
	Data        json.RawMessage `json:"data"`
}

type NotificationRequested struct {
	EventID     string            `json:"event_id"`
	EventType   string            `json:"event_type"`
	Channel     string            `json:"channel"`
	Priority    string            `json:"priority,omitempty"`
	Recipient   string            `json:"recipient"`
	Subject     string            `json:"subject"`
	TemplateKey string            `json:"template_key"`
	Data        map[string]string `json:"data"`
}

type StudyDocumentExtractionRequested struct {
	EventID    string `json:"event_id"`
	JobID      string `json:"job_id"`
	DocumentID string `json:"document_id"`
	UserID     string `json:"user_id"`
	ObjectKey  string `json:"object_key"`
	Filename   string `json:"filename"`
	MediaType  string `json:"media_type"`
	Attempt    int    `json:"attempt"`
}

type StudyGenerationDocument struct {
	DocumentID    string `json:"document_id"`
	ExtractionKey string `json:"extraction_key"`
}

type StudyGenerationRequest struct {
	EventID      string                    `json:"event_id"`
	JobID        string                    `json:"job_id"`
	StudyKitID   string                    `json:"study_kit_id"`
	UserID       string                    `json:"user_id"`
	Documents    []StudyGenerationDocument `json:"documents"`
	Language     string                    `json:"language"`
	Instructions string                    `json:"instructions,omitempty"`
	Attempt      int                       `json:"attempt"`
}

type StudyManualGenerationRequested struct {
	StudyGenerationRequest
	ManualID string `json:"manual_id"`
}

type StudyQuizGenerationRequested struct {
	StudyGenerationRequest
	QuizID           string   `json:"quiz_id"`
	Difficulty       string   `json:"difficulty"`
	QuestionCount    int      `json:"question_count"`
	Types            []string `json:"types"`
	PreviousQuizKeys []string `json:"previous_quiz_keys"`
}

type StudyFlashcardsGenerationRequested struct {
	StudyGenerationRequest
	FlashcardDeckID           string   `json:"flashcard_deck_id"`
	CardCount                 int      `json:"card_count"`
	Style                     string   `json:"style"`
	PreviousFlashcardDeckKeys []string `json:"previous_flashcard_deck_keys"`
}

type StudyAIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type StudyAIGenerationMetadata struct {
	Model              string `json:"model"`
	PromptVersion      string `json:"prompt_version"`
	SourceHash         string `json:"source_hash"`
	PromptCacheKey     string `json:"prompt_cache_key"`
	PromptTokens       int    `json:"prompt_tokens"`
	CachedTokens       int    `json:"cached_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	TotalTokens        int    `json:"total_tokens"`
	GenerationAttempts int    `json:"generation_attempts"`
}

type StudyAIResultPayload struct {
	JobID           string                     `json:"job_id"`
	StudyKitID      string                     `json:"study_kit_id,omitempty"`
	DocumentID      string                     `json:"document_id,omitempty"`
	ManualID        string                     `json:"manual_id,omitempty"`
	QuizID          string                     `json:"quiz_id,omitempty"`
	FlashcardDeckID string                     `json:"flashcard_deck_id,omitempty"`
	ExtractionKey   string                     `json:"extraction_key,omitempty"`
	ResultKey       string                     `json:"result_key,omitempty"`
	PageCount       int                        `json:"page_count,omitempty"`
	Warnings        []string                   `json:"warnings,omitempty"`
	Metadata        *StudyAIGenerationMetadata `json:"metadata,omitempty"`
	Error           *StudyAIError              `json:"error,omitempty"`
}

type StudyAIResultEvent struct {
	EventID       string               `json:"event_id"`
	EventType     string               `json:"event_type"`
	OccurredAt    time.Time            `json:"occurred_at"`
	CorrelationID string               `json:"correlation_id"`
	Payload       StudyAIResultPayload `json:"payload"`
}
