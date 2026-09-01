package domain

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type NotificationScheduler interface {
	ScheduleEmail(context.Context, string, string, any) error
}

const BackInStockJobType = "back_in_stock"

type DurableJob struct {
	ID         uuid.UUID
	LockToken  uuid.UUID
	Type       string
	Email      string
	Payload    []byte
	RetryCount int
}
type DurableJobStore interface {
	ClaimDue(context.Context, time.Time) (*DurableJob, error)
	Complete(context.Context, DurableJob) error
	Fail(context.Context, DurableJob, time.Time, bool) error
}
