//go:build integration

package orderanalytics

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

// Every query here reads domain_events by aggregate_id and topic. The table's
// only index led with aggregate_type, which none of them constrains, so the
// planner could not use it: the per-order read — one per paid order, inside the
// transaction holding the projection lock — was a parallel sequential scan, and
// the rebuild scanned the table twice.
//
// These hold migrations/core/000022. Nothing prunes domain_events, so a
// regression here gets worse every day the store runs.

func TestThePerOrderSnapshotDoesNotReadTheWholeEventTable(t *testing.T) {
	_, db, _ := newAnalyticsProvider(t)
	orderID := seedPaidOrder(t, db, 1, 1000)
	seedPaidEventBacklog(t, db, 50000)

	plan := explainAnalytics(t, db, `
SELECT o.id, event.id, event.occurred_at
FROM orders AS o
JOIN domain_events AS event ON event.aggregate_id = o.id AND event.topic = $1
WHERE o.id = $2
ORDER BY event.occurred_at DESC
LIMIT 1`, events.TopicOrderPaid, orderID)

	assertUsesTopicIndex(t, plan)
}

func TestTheRebuildRangeReadDoesNotReadTheWholeEventTable(t *testing.T) {
	// The rebuilder runs this over a month of orders. It joins domain_events
	// twice, so an unusable index cost two full scans, not one.
	_, db, _ := newAnalyticsProvider(t)
	seedPaidOrder(t, db, 1, 1000)
	seedPaidEventBacklog(t, db, 50000)

	plan := explainAnalytics(t, db, `
SELECT DISTINCT ON (o.id) o.id, paid.id, paid.occurred_at
FROM orders AS o
JOIN domain_events AS paid ON paid.aggregate_id = o.id AND paid.topic = $1
WHERE EXISTS (
    SELECT 1 FROM domain_events e
    WHERE e.aggregate_id = o.id AND e.topic IN ($2, $3)
      AND e.occurred_at >= $4 AND e.occurred_at < $5
)
ORDER BY o.id ASC, paid.occurred_at DESC`,
		events.TopicOrderPaid, events.TopicOrderPaid, events.TopicOrderRefunded,
		time.Now().Add(-30*24*time.Hour), time.Now().Add(24*time.Hour))

	assertUsesTopicIndex(t, plan)
}

func assertUsesTopicIndex(t *testing.T, plan string) {
	t.Helper()
	if strings.Contains(plan, "Seq Scan on domain_events") {
		t.Fatalf("the query reads the whole event table:\n%s", plan)
	}
	if !strings.Contains(plan, "domain_events_topic_aggregate_idx") {
		t.Fatalf("the topic index was not used:\n%s", plan)
	}
}

func explainAnalytics(t *testing.T, db *gorm.DB, query string, args ...any) string {
	t.Helper()
	var plan []string
	if err := db.Raw("EXPLAIN "+query, args...).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	return strings.Join(plan, "\n")
}

// seedPaidEventBacklog makes orders.paid.v1 the common topic it is in a real
// store. With only a handful of them the planner reaches for the
// (topic, idempotency_key) unique index and the missing index does not show —
// which is exactly why this was invisible until production volume.
func seedPaidEventBacklog(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	if err := db.Exec(`
INSERT INTO domain_events (id, topic, aggregate_type, aggregate_id, idempotency_key, payload, occurred_at)
SELECT gen_random_uuid(), ?, 'order', gen_random_uuid(), gen_random_uuid(), '{"version":1}',
       CURRENT_TIMESTAMP - (n || ' seconds')::interval
FROM generate_series(1, ?) AS n`, events.TopicOrderPaid, rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`ANALYZE domain_events`).Error; err != nil {
		t.Fatal(err)
	}
	var seeded int64
	if err := db.Raw(`SELECT COUNT(*) FROM domain_events WHERE topic = ?`, events.TopicOrderPaid).Scan(&seeded).Error; err != nil {
		t.Fatal(err)
	}
	if seeded < int64(rows) {
		t.Fatalf("seeded %d paid events, want at least %d; the plan would not be the one production takes", seeded, rows)
	}
}
