package domain

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// NotificationScheduler enqueues an email for the durable worker to send.
//
// The locale is the recipient's, and an empty one means the caller does not
// know it — the scheduler then falls back to the store's default rather than
// to a hard-coded language. No module currently records a per-recipient
// locale, so all of them pass empty today; the parameter exists so one that
// starts recording it has somewhere to put it.
type NotificationScheduler interface {
	ScheduleEmail(ctx context.Context, jobType, locale, email string, payload any) error
}

const BackInStockJobType = "back_in_stock"

type DurableJob struct {
	ID        uuid.UUID
	LockToken uuid.UUID
	Type      string
	Email     string
	// Locale is the language this job's template is rendered in. It was stored
	// but never read: the worker rendered every scheduled email with a
	// hard-coded "en", so a store that had seeded both English and its own
	// templates sent English to everyone.
	Locale     string
	Payload    []byte
	RetryCount int
}
type DurableJobStore interface {
	ClaimDue(context.Context, time.Time) (*DurableJob, error)
	Complete(context.Context, DurableJob) error
	Fail(context.Context, DurableJob, time.Time, bool) error
}
