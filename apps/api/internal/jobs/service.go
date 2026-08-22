package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler func(context.Context, Job) error

type Service struct {
	db         *gorm.DB
	log        *slog.Logger
	handlers   map[string]Handler
	now        func() time.Time
	workerID   string
	staleAfter time.Duration
}

type EnqueueInput struct {
	OrganizationID *uuid.UUID
	JobType        string
	Payload        any
	IdempotencyKey string
	CorrelationID  string
	MaxAttempts    int
	AvailableAt    *time.Time
}

type PermanentError struct {
	Err error
}

func (e PermanentError) Error() string {
	if e.Err == nil {
		return "permanent job failure"
	}
	return e.Err.Error()
}

func NewService(db *gorm.DB, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, log: logger, handlers: map[string]Handler{}, now: func() time.Time { return time.Now().UTC() }, workerID: "worker-" + uuid.NewString(), staleAfter: 15 * time.Minute}
}

func (s *Service) Register(jobType string, handler Handler) {
	jobType = strings.TrimSpace(jobType)
	if jobType != "" && handler != nil {
		s.handlers[jobType] = handler
	}
}

func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) (Job, error) {
	input.JobType = strings.TrimSpace(input.JobType)
	if input.JobType == "" {
		return Job{}, errors.New("job type is required")
	}
	payload := "{}"
	if input.Payload != nil {
		body, err := json.Marshal(input.Payload)
		if err != nil {
			return Job{}, err
		}
		payload = string(body)
	}
	availableAt := s.now()
	if input.AvailableAt != nil {
		availableAt = *input.AvailableAt
	}
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey != "" {
		var existing Job
		err := s.db.WithContext(ctx).Where("job_type = ? AND idempotency_key = ?", input.JobType, idempotencyKey).First(&existing).Error
		if err == nil {
			return existing, nil
		}
		if err != gorm.ErrRecordNotFound {
			return Job{}, err
		}
	}
	job := Job{ID: uuid.New(), OrganizationID: input.OrganizationID, JobType: input.JobType, Status: StatusQueued, Payload: payload, IdempotencyKey: idempotencyKey, CorrelationID: strings.TrimSpace(input.CorrelationID), MaxAttempts: maxAttempts, AvailableAt: availableAt}
	if job.IdempotencyKey == "" {
		err := s.db.WithContext(ctx).Create(&job).Error
		return job, err
	}
	err := s.db.WithContext(ctx).Create(&job).Error
	if err == nil {
		return job, nil
	}
	var existing Job
	findErr := s.db.WithContext(ctx).Where("job_type = ? AND idempotency_key = ?", job.JobType, job.IdempotencyKey).First(&existing).Error
	if findErr == nil {
		return existing, nil
	}
	return Job{}, err
}

func (s *Service) ProcessDue(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	if err := s.requeueStaleProcessing(ctx); err != nil {
		return 0, err
	}
	processed := 0
	for processed < limit {
		job, found, err := s.claimOne(ctx)
		if err != nil || !found {
			return processed, err
		}
		processed++
		if err := s.process(ctx, job); err != nil {
			s.log.Warn("background job failed", "job_id", job.ID, "job_type", job.JobType, "attempts", job.Attempts+1, "error", publicError(err))
		}
	}
	return processed, nil
}

func (s *Service) requeueStaleProcessing(ctx context.Context) error {
	if s.staleAfter <= 0 {
		return nil
	}
	now := s.now()
	cutoff := now.Add(-s.staleAfter)
	return s.db.WithContext(ctx).
		Model(&Job{}).
		Where("status = ? AND locked_at IS NOT NULL AND locked_at <= ?", StatusProcessing, cutoff).
		Updates(map[string]any{
			"status":       StatusRetryPending,
			"available_at": now,
			"locked_at":    nil,
			"locked_by":    "",
			"last_error":   "job lock expired",
			"updated_at":   now,
		}).Error
}

func (s *Service) Start(ctx context.Context, interval time.Duration, batchSize int) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.ProcessDue(ctx, batchSize); err != nil {
					s.log.Warn("background worker tick failed", "error", err)
				}
			}
		}
	}()
}

func (s *Service) claimOne(ctx context.Context) (Job, bool, error) {
	if s.db.Dialector.Name() == "postgres" {
		var job Job
		err := s.db.WithContext(ctx).Raw(`
			UPDATE background_jobs
			SET status = ?, locked_at = ?, locked_by = ?, started_at = COALESCE(started_at, ?), updated_at = ?
			WHERE id = (
				SELECT id
				FROM background_jobs
				WHERE status IN (?, ?) AND available_at <= ?
				ORDER BY available_at ASC, created_at ASC
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			RETURNING *
		`, StatusProcessing, s.now(), s.workerID, s.now(), s.now(), StatusQueued, StatusRetryPending, s.now()).Scan(&job).Error
		if err != nil {
			return Job{}, false, err
		}
		if job.ID == uuid.Nil {
			return Job{}, false, nil
		}
		return job, true, nil
	}
	for attempt := 0; attempt < 5; attempt++ {
		var job Job
		err := s.db.WithContext(ctx).
			Where("status IN ? AND available_at <= ?", []string{StatusQueued, StatusRetryPending}, s.now()).
			Order("available_at ASC, created_at ASC").First(&job).Error
		if err == gorm.ErrRecordNotFound {
			return Job{}, false, nil
		}
		if err != nil {
			if isDatabaseContention(err) {
				time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				continue
			}
			return Job{}, false, err
		}
		now := s.now()
		result := s.db.WithContext(ctx).Model(&Job{}).
			Where("id = ? AND status IN ?", job.ID, []string{StatusQueued, StatusRetryPending}).
			Updates(map[string]any{"status": StatusProcessing, "locked_at": &now, "locked_by": s.workerID, "started_at": &now, "updated_at": now})
		if result.Error != nil {
			if isDatabaseContention(result.Error) {
				time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				continue
			}
			return Job{}, false, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		job.Status = StatusProcessing
		job.LockedAt = &now
		job.LockedBy = s.workerID
		job.StartedAt = &now
		job.UpdatedAt = now
		return job, true, nil
	}
	return Job{}, false, nil
}

func isDatabaseContention(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
}

func (s *Service) process(ctx context.Context, job Job) error {
	handler, ok := s.handlers[job.JobType]
	if !ok {
		return s.fail(ctx, job, PermanentError{Err: fmt.Errorf("no handler registered for job type %s", job.JobType)})
	}
	err := handler(ctx, job)
	if err == nil {
		now := s.now()
		return s.db.WithContext(ctx).Model(&job).Updates(map[string]any{"status": StatusCompleted, "completed_at": &now, "locked_at": nil, "locked_by": "", "last_error": "", "updated_at": now}).Error
	}
	return s.fail(ctx, job, err)
}

func (s *Service) fail(ctx context.Context, job Job, err error) error {
	now := s.now()
	attempts := job.Attempts + 1
	status := StatusRetryPending
	availableAt := now.Add(backoff(attempts))
	var permanent PermanentError
	if errors.As(err, &permanent) || attempts >= job.MaxAttempts {
		status = StatusFailedPermanently
		availableAt = now
	}
	updateErr := s.db.WithContext(ctx).Model(&job).Updates(map[string]any{"status": status, "attempts": attempts, "available_at": availableAt, "locked_at": nil, "locked_by": "", "last_error": publicError(err), "updated_at": now}).Error
	if updateErr != nil {
		return updateErr
	}
	return err
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := math.Pow(2, float64(attempt-1)) * 30
	if seconds > 30*60 {
		seconds = 30 * 60
	}
	return time.Duration(seconds) * time.Second
}

func publicError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
