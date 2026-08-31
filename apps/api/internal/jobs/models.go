package jobs

import (
	"time"

	"github.com/google/uuid"
)

const (
	JobTypeChannelOutbound        = "channel.outbound.send"
	JobTypeNotificationDelivery   = "notification.deliver"
	JobTypePaymentReconcile       = "payment.reconcile"
	JobTypeMerchantImport         = "merchant.import"
	JobTypeServiceDispatchTimeout = "fieldservice.dispatch.timeout"
	JobTypeKnowledgeEmbedding     = "knowledge.embedding.refresh"

	StatusQueued            = "queued"
	StatusProcessing        = "processing"
	StatusCompleted         = "completed"
	StatusRetryPending      = "retry_pending"
	StatusFailed            = "failed"
	StatusFailedPermanently = "failed_permanently"
	StatusCancelled         = "cancelled"
)

type Job struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	JobType        string     `json:"job_type"`
	Status         string     `json:"status"`
	Payload        string     `json:"payload"`
	IdempotencyKey string     `json:"idempotency_key"`
	CorrelationID  string     `json:"correlation_id"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	AvailableAt    time.Time  `json:"available_at"`
	LockedAt       *time.Time `json:"locked_at,omitempty"`
	LockedBy       string     `json:"locked_by"`
	LastError      string     `json:"last_error"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Job) TableName() string { return "background_jobs" }
