package domain

import "time"

type OperationStatus string

const (
	OperationQueued  OperationStatus = "queued"
	OperationRunning OperationStatus = "running"
	OperationSuccess OperationStatus = "success"
	OperationFailed  OperationStatus = "failed"
)

type Operation struct {
	ID           string          `json:"id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id,omitempty"`
	Status       OperationStatus `json:"status"`
	Message      string          `json:"message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
}
