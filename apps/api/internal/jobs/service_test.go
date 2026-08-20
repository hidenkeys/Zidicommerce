package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
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

func TestProcessDueRequeuesStaleProcessingJobs(t *testing.T) {
	db, service := newJobTestService(t)
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.staleAfter = time.Minute
	lockedAt := now.Add(-2 * time.Minute)
	job := Job{ID: uuid.New(), JobType: "noop", Status: StatusProcessing, Payload: "{}", LockedAt: &lockedAt, LockedBy: "dead-worker", MaxAttempts: 3, AvailableAt: lockedAt}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	service.Register("noop", func(_ context.Context, _ Job) error { return nil })
	processed, err := service.ProcessDue(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected stale job to be processed, got %d", processed)
	}
	var updated Job
	if err := db.Where("id = ?", job.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != StatusCompleted || updated.LockedBy != "" || updated.CompletedAt == nil {
		t.Fatalf("expected stale job to complete after reclaim, got %+v", updated)
	}
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

func TestProcessDueMarksPermanentAfterMaxAttempts(t *testing.T) {
	db, service := newJobTestService(t)
	service.Register("always-fails", func(_ context.Context, _ Job) error {
		return errors.New("temporary failure")
	})
	job, err := service.Enqueue(context.Background(), EnqueueInput{JobType: "always-fails", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessDue(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	var updated Job
	if err := db.Where("id = ?", job.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != StatusFailedPermanently || updated.Attempts != 1 {
		t.Fatalf("expected permanently failed job, got %+v", updated)
	}
}

func TestConcurrentWorkersClaimJobOnce(t *testing.T) {
	db, first := newJobTestService(t)
	second := NewService(db, slog.Default())
	var handled atomic.Int32
	handler := func(_ context.Context, _ Job) error {
		handled.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	first.Register("single", handler)
	second.Register("single", handler)
	if _, err := first.Enqueue(context.Background(), EnqueueInput{JobType: "single"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = first.ProcessDue(context.Background(), 1)
	}()
	go func() {
		defer wg.Done()
		_, _ = second.ProcessDue(context.Background(), 1)
	}()
	wg.Wait()
	if handled.Load() != 1 {
		t.Fatalf("expected exactly one worker to process the job, got %d", handled.Load())
	}
}
