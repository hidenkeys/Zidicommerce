package jobs

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newJobTestService(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Job{}); err != nil {
		t.Fatal(err)
	}
	return db, NewService(db, slog.Default())
}

func TestEnqueueIsIdempotentByTypeAndKey(t *testing.T) {
	db, service := newJobTestService(t)
	orgID := uuid.New()
	first, err := service.Enqueue(context.Background(), EnqueueInput{OrganizationID: &orgID, JobType: JobTypeChannelOutbound, Payload: map[string]any{"id": "one"}, IdempotencyKey: "same-key"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Enqueue(context.Background(), EnqueueInput{OrganizationID: &orgID, JobType: JobTypeChannelOutbound, Payload: map[string]any{"id": "one"}, IdempotencyKey: "same-key"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same job, got %s and %s", first.ID, second.ID)
	}
	var count int64
	if err := db.Model(&Job{}).Where("job_type = ?", JobTypeChannelOutbound).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one job, got %d", count)
	}
}

func TestProcessDueRetriesAndThenCompletes(t *testing.T) {
	db, service := newJobTestService(t)
	service.now = func() time.Time { return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC) }
	attempts := 0
	service.Register("flaky", func(_ context.Context, _ Job) error {
		attempts++
		if attempts == 1 {
			return errors.New("temporary failure")
		}
		return nil
	})
	job, err := service.Enqueue(context.Background(), EnqueueInput{JobType: "flaky", Payload: map[string]any{"ok": true}, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessDue(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected one processed job, got %d", processed)
	}
	var retry Job
	if err := db.Where("id = ?", job.ID).First(&retry).Error; err != nil {
		t.Fatal(err)
	}
	if retry.Status != StatusRetryPending || retry.Attempts != 1 {
		t.Fatalf("expected retry_pending after first failure, got %+v", retry)
	}
	service.now = func() time.Time { return retry.AvailableAt.Add(time.Second) }
	processed, err = service.ProcessDue(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected retry to process, got %d", processed)
	}
	var completed Job
	if err := db.Where("id = ?", job.ID).First(&completed).Error; err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || completed.CompletedAt == nil {
		t.Fatalf("expected completed job, got %+v", completed)
	}
}
