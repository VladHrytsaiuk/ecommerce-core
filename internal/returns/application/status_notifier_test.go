package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

// Nothing consumed returns.status_changed.v1, so a customer filed a return, an
// administrator approved it, the warehouse received the goods and the refund
// went out — and the store never said a word. These pin what it now says and,
// as importantly, what it stays quiet about.

func TestEachTransitionTheBuyerIsWaitingOnSendsMail(t *testing.T) {
	for status, jobType := range map[returns.ReturnStatus]string{
		returns.ReturnStatusApproved: "return_approved",
		returns.ReturnStatusRejected: "return_rejected",
		returns.ReturnStatusReceived: "return_received",
		returns.ReturnStatusRefunded: "return_refunded",
	} {
		t.Run(string(status), func(t *testing.T) {
			world := newNotifierWorld(t)

			if err := world.notifier.Handle(context.Background(), world.delivery(status)); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(world.scheduler.scheduled) != 1 {
				t.Fatalf("scheduled %d mails, want one", len(world.scheduler.scheduled))
			}
			mail := world.scheduler.scheduled[0]
			if mail.jobType != jobType {
				t.Fatalf("job type = %q, want %q", mail.jobType, jobType)
			}
			if mail.email != "buyer@example.test" || mail.locale != "uk" {
				t.Fatalf("addressed %q in %q, want the order's own contact", mail.email, mail.locale)
			}
		})
	}
}

func TestATransitionTheBuyerHasAlreadySeenSendsNothing(t *testing.T) {
	// "new" is the request they just filed; "closed" is bookkeeping.
	for _, status := range []returns.ReturnStatus{returns.ReturnStatusNew, returns.ReturnStatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			world := newNotifierWorld(t)

			if err := world.notifier.Handle(context.Background(), world.delivery(status)); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(world.scheduler.scheduled) != 0 {
				t.Fatalf("scheduled %d mails for %q", len(world.scheduler.scheduled), status)
			}
		})
	}
}

func TestAnOrderWithNoContactIsAcknowledgedRatherThanRetried(t *testing.T) {
	// A guest order placed before contact capture has nobody to write to.
	// Failing would retry ten times and then park the delivery in the dead
	// queue for an address that is never going to appear.
	world := newNotifierWorld(t)
	world.orders.snapshot.ContactEmail = "   "

	if err := world.notifier.Handle(context.Background(), world.delivery(returns.ReturnStatusApproved)); err != nil {
		t.Fatalf("Handle() error = %v, want the delivery acknowledged", err)
	}
	if len(world.scheduler.scheduled) != 0 {
		t.Fatal("a mail was scheduled with no recipient")
	}
}

func TestTheMailIsScheduledInsideATransaction(t *testing.T) {
	// ScheduleEmail writes a notification job and refuses to run without the
	// caller's transaction.
	world := newNotifierWorld(t)

	if err := world.notifier.Handle(context.Background(), world.delivery(returns.ReturnStatusRefunded)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !world.scheduler.sawTransaction {
		t.Fatal("ScheduleEmail was called outside the caller's transaction")
	}
}

func TestTheNotificationCarriesNoProductOrReason(t *testing.T) {
	// The buyer is told which return moved and to what; the detail lives in
	// their account, not in an email the store cannot recall.
	world := newNotifierWorld(t)

	if err := world.notifier.Handle(context.Background(), world.delivery(returns.ReturnStatusReceived)); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(world.scheduler.scheduled[0].payload)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["return_id"] != world.returnID.String() || fields["status"] != "received" {
		t.Fatalf("payload = %v", fields)
	}
}

func TestAMalformedEventIsRejectedRatherThanGuessedAt(t *testing.T) {
	world := newNotifierWorld(t)
	for name, payload := range map[string]string{
		"not json":      `{`,
		"wrong version": `{"version":2,"return_id":"` + uuid.NewString() + `","order_id":"` + uuid.NewString() + `","to_status":"approved"}`,
		"no return id":  `{"version":1,"order_id":"` + uuid.NewString() + `","to_status":"approved"}`,
		"no order id":   `{"version":1,"return_id":"` + uuid.NewString() + `","to_status":"approved"}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := world.notifier.Handle(context.Background(), events.Delivery{
				EventID: uuid.New(), Topic: returns.TopicStatusChanged, Payload: []byte(payload),
			})
			if err == nil {
				t.Fatal("Handle() accepted a payload it could not read")
			}
		})
	}
}

func TestAnUnreadableOrderFailsSoTheDeliveryRetries(t *testing.T) {
	world := newNotifierWorld(t)
	world.orders.err = errors.New("database unavailable")

	if err := world.notifier.Handle(context.Background(), world.delivery(returns.ReturnStatusApproved)); err == nil {
		t.Fatal("Handle() acknowledged a delivery it could not act on")
	}
}

func TestTheNotifierAnnouncesTheTopicItHandles(t *testing.T) {
	// The worker fails any delivery whose topic it has no handler for, so this
	// string is what connects the routing table to this code.
	world := newNotifierWorld(t)
	if got := world.notifier.Topic(); got != returns.TopicStatusChanged {
		t.Fatalf("Topic() = %q, want %q", got, returns.TopicStatusChanged)
	}
}

// --- harness ---

type notifierWorld struct {
	notifier  *StatusChangedNotifier
	orders    *notifierOrders
	scheduler *recordingScheduler
	returnID  uuid.UUID
	orderID   uuid.UUID
}

func newNotifierWorld(t *testing.T) *notifierWorld {
	t.Helper()
	w := &notifierWorld{returnID: uuid.New(), orderID: uuid.New(), scheduler: &recordingScheduler{}}
	w.orders = &notifierOrders{snapshot: returns.OrderSnapshot{
		OrderID: w.orderID, Status: "delivered", ContactEmail: "buyer@example.test", Locale: "uk",
	}}
	notifier, err := NewStatusChangedNotifier(w.orders, markingTransaction{}, w.scheduler)
	if err != nil {
		t.Fatal(err)
	}
	w.notifier = notifier
	return w
}

func (w *notifierWorld) delivery(status returns.ReturnStatus) events.Delivery {
	event, err := returns.NewStatusChangedEvent(uuid.New(), w.returnID, w.orderID,
		returns.ReturnStatusNew, status, returns.ActorTypeAdmin, time.Now().UTC())
	if err != nil {
		panic(err)
	}
	payload, err := event.MarshalPayload()
	if err != nil {
		panic(err)
	}
	return events.Delivery{EventID: uuid.New(), AggregateID: w.returnID, Topic: returns.TopicStatusChanged, Payload: payload}
}

type notifierOrders struct {
	snapshot returns.OrderSnapshot
	err      error
}

func (o *notifierOrders) GetOrderSnapshot(context.Context, uuid.UUID) (returns.OrderSnapshot, error) {
	return o.snapshot, o.err
}

type transactionFlag struct{}

type markingTransaction struct{}

func (markingTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, transactionFlag{}, true))
}

type scheduledMail struct {
	jobType, locale, email string
	payload                any
}

type recordingScheduler struct {
	scheduled      []scheduledMail
	sawTransaction bool
	err            error
}

func (s *recordingScheduler) ScheduleEmail(ctx context.Context, jobType, locale, email string, payload any) error {
	if s.err != nil {
		return s.err
	}
	if ctx.Value(transactionFlag{}) == true {
		s.sawTransaction = true
	}
	s.scheduled = append(s.scheduled, scheduledMail{jobType: jobType, locale: locale, email: email, payload: payload})
	return nil
}
