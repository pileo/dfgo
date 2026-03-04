package server

import (
	"time"

	"dfgo/internal/attractor/simulate"
)

// SubmitRequest is the JSON body for POST /api/v1/pipelines.
type SubmitRequest struct {
	DOTSource      string            `json:"dot_source"`
	InitialContext map[string]string `json:"initial_context,omitempty"`
	AutoApprove    bool              `json:"auto_approve,omitempty"`
	Simulate       *simulate.Config  `json:"simulate,omitempty"`
}

// SubmitResponse is returned by POST /api/v1/pipelines.
type SubmitResponse struct {
	RunID string `json:"run_id"`
}

// StatusResponse is returned by GET /api/v1/pipelines/{id}.
type StatusResponse struct {
	RunID       string    `json:"run_id"`
	Status      string    `json:"status"`
	Pipeline    string    `json:"pipeline"`
	CurrentNode string    `json:"current_node,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// ContextResponse is returned by GET /api/v1/pipelines/{id}/context.
type ContextResponse struct {
	RunID   string            `json:"run_id"`
	Context map[string]string `json:"context"`
	Logs    []string          `json:"logs,omitempty"`
}

// QuestionResponse is an element of the GET /api/v1/pipelines/{id}/questions response.
type QuestionResponse struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options,omitempty"`
	Default string   `json:"default,omitempty"`
}

// AnswerRequest is the JSON body for POST /api/v1/pipelines/{id}/questions/{qid}/answer.
type AnswerRequest struct {
	Text     string `json:"text"`
	Selected int    `json:"selected"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
	Status string `json:"status"`
}
