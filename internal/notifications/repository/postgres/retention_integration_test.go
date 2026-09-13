//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Which statuses may be deleted is a property of this one statement. Getting it
// wrong does not fail anywhere: it silently drops messages that were still
// going to be sent, or erases the record that some never went at all.

func TestOnlyTerminalJobsArePurged(t *testing.T) {
	repository, db := newTemplateRepository(t)
	ancient := time.Now().UTC().Add(-365 * 24 * time.Hour)
	kept := map[string]uuid.UUID{}
	for _, status := range []string{"pending", "sending", "failed"} {
		kept[status] = seedJob(t, db, status, ancient)
	}
	sent := seedJob(t, db, "sent", ancient)
	dead := seedJob(t, db, "dead", ancient)

	purged, err := repository.PurgeTerminal(context.Background(), time.Now().UTC(), time.Now().UTC(), 100)
	if err != nil {
		t.Fatalf("PurgeTerminal() error = %v", err)
	}
	if purged != 2 {
		t.Fatalf("purged %d jobs, want the sent and dead ones only", purged)
	}
	for status, id := range kept {
		if !jobExists(t, db, id) {
			// 'failed' is the scheduled dispatcher's retryable state: it carries
			// next_retry_at and will be picked up again.
			t.Fatalf("a %q job was deleted; that message was still going to be sent", status)
		}
	}
	if jobExists(t, db, sent) || jobExists(t, db, dead) {
		t.Fatal("a terminal job survived the purge")
	}
}

func TestEachStatusKeepsToItsOwnWindow(t *testing.T) {
	repository, db := newTemplateRepository(t)
	now := time.Now().UTC()
	// Both 60 days old: past a 30-day sent window, inside a 90-day dead one.
	sent := seedJob(t, db, "sent", now.Add(-60*24*time.Hour))
	dead := seedJob(t, db, "dead", now.Add(-60*24*time.Hour))

	if _, err := repository.PurgeTerminal(context.Background(), now.Add(-30*24*time.Hour), now.Add(-90*24*time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	if jobExists(t, db, sent) {
		t.Fatal("a sent job older than its window survived")
	}
	if !jobExists(t, db, dead) {
		t.Fatal("a dead job inside its window was deleted; the record that a message never went out is gone")
	}
}

func TestARecentTerminalJobIsLeftAlone(t *testing.T) {
	repository, db := newTemplateRepository(t)
	now := time.Now().UTC()
	recent := seedJob(t, db, "sent", now.Add(-time.Hour))

	if _, err := repository.PurgeTerminal(context.Background(), now.Add(-30*24*time.Hour), now.Add(-90*24*time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	if !jobExists(t, db, recent) {
		t.Fatal("a job inside its window was deleted")
	}
}

func TestDeliveryAttemptsLeaveWithTheirJob(t *testing.T) {
	// Otherwise the attempts table keeps growing after the job it describes is
	// gone, holding rows nothing can be joined back to.
	repository, db := newTemplateRepository(t)
	ancient := time.Now().UTC().Add(-365 * 24 * time.Hour)
	job := seedJob(t, db, "sent", ancient)
	if err := db.Exec(`INSERT INTO notification_attempts (id, job_id, attempt_number, provider, status, completed_at)
		VALUES (?, ?, 1, 'smtp', 'success', ?)`, uuid.New(), job, ancient).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := repository.PurgeTerminal(context.Background(), time.Now().UTC(), time.Now().UTC(), 100); err != nil {
		t.Fatal(err)
	}
	var orphaned int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_attempts WHERE job_id = ?`, job).Scan(&orphaned).Error; err != nil {
		t.Fatal(err)
	}
	if orphaned != 0 {
		t.Fatalf("%d attempt rows outlived their job", orphaned)
	}
}

func TestThePurgeIsBoundedByItsBatch(t *testing.T) {
	repository, db := newTemplateRepository(t)
	ancient := time.Now().UTC().Add(-365 * 24 * time.Hour)
	for range 10 {
		seedJob(t, db, "sent", ancient)
	}

	purged, err := repository.PurgeTerminal(context.Background(), time.Now().UTC(), time.Now().UTC(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 4 {
		t.Fatalf("purged %d jobs in one pass, want the batch size of 4", purged)
	}
	var remaining int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_jobs`).Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 6 {
		t.Fatalf("%d jobs remain, want 6", remaining)
	}
}

func TestTheDeadCountIsWhatAnOperatorWouldSee(t *testing.T) {
	repository, db := newTemplateRepository(t)
	now := time.Now().UTC()
	for range 3 {
		seedJob(t, db, "dead", now)
	}
	seedJob(t, db, "sent", now)
	seedJob(t, db, "failed", now)

	dead, err := repository.CountDead(context.Background())
	if err != nil {
		t.Fatalf("CountDead() error = %v", err)
	}
	if dead != 3 {
		t.Fatalf("CountDead() = %d, want 3", dead)
	}
}

func TestAnUnboundedPurgeIsRefused(t *testing.T) {
	repository, _ := newTemplateRepository(t)
	now := time.Now().UTC()
	for name, limit := range map[string]int{"zero": 0, "negative": -1, "absurd": 100000} {
		t.Run(name, func(t *testing.T) {
			if _, err := repository.PurgeTerminal(context.Background(), now, now, limit); err == nil {
				t.Fatal("PurgeTerminal() accepted an unbounded batch")
			}
		})
	}
}

func seedJob(t *testing.T, db *gorm.DB, status string, updatedAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	// A scheduler job carries a plaintext payload and an outbox job carries
	// ciphertext; the schema enforces that split, so the seed has to honour it.
	if err := db.Exec(`INSERT INTO notification_jobs
		(id, event_id, order_id, dedupe_key, channel, recipient_email, locale, template_key,
		 payload_ciphertext, payload, status, provider, dispatcher, type, retry_count, next_retry_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'email', 'buyer@example.test', 'en', 'order_paid',
		        '{}', '{}', ?, 'smtp', 'scheduler', 'order_paid', 0, ?, ?, ?)`,
		id, uuid.New(), uuid.New(), "dedupe-"+id.String(), status, updatedAt, updatedAt, updatedAt).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func jobExists(t *testing.T, db *gorm.DB, id uuid.UUID) bool {
	t.Helper()
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_jobs WHERE id = ?`, id).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count == 1
}

// Both retention queries used to read the whole table: every index here is
// partial on the statuses the dispatcher claims, which is right for the
// dispatcher and useless for the purge. At two hundred thousand rows that was a
// parallel sequential scan plus a top-N sort for the candidates, and another
// scan for the count — and the count ran once per batch, up to 256 times a
// tick. These hold the cold-side indexes that fixed it.

func TestTheRetentionQueriesDoNotReadTheWholeTable(t *testing.T) {
	_, db := newTemplateRepository(t)
	seedTerminalBacklog(t, db, 20000)

	for name, query := range map[string]struct {
		sql  string
		args []any
	}{
		"counting the dead backlog": {
			`SELECT COUNT(*) FROM notification_jobs WHERE status = 'dead'`, nil,
		},
		"selecting purge candidates": {
			`SELECT id FROM notification_jobs
			 WHERE (status = 'sent' AND updated_at < $1) OR (status = 'dead' AND updated_at < $2)
			 ORDER BY updated_at ASC LIMIT 1000`,
			[]any{time.Now().UTC(), time.Now().UTC()},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var plan []string
			if err := db.Raw("EXPLAIN "+query.sql, query.args...).Scan(&plan).Error; err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(plan, "\n")
			if strings.Contains(joined, "Seq Scan on notification_jobs") {
				t.Fatalf("the query reads the whole table:\n%s", joined)
			}
			if !strings.Contains(joined, "notification_jobs_terminal_idx") && !strings.Contains(joined, "notification_jobs_dead_idx") {
				t.Fatalf("neither terminal index was used:\n%s", joined)
			}
		})
	}
}

func TestTheCandidateQueryNeedsNoSort(t *testing.T) {
	// The purge orders by updated_at and stops at its batch size. Without an
	// index in that order the planner sorts everything that matched first,
	// which is most of the table.
	_, db := newTemplateRepository(t)
	seedTerminalBacklog(t, db, 20000)

	var plan []string
	if err := db.Raw(`EXPLAIN SELECT id FROM notification_jobs
		WHERE (status = 'sent' AND updated_at < $1) OR (status = 'dead' AND updated_at < $2)
		ORDER BY updated_at ASC LIMIT 1000`, time.Now().UTC(), time.Now().UTC()).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(plan, "\n"); strings.Contains(joined, "Sort") {
		t.Fatalf("the candidate query sorts:\n%s", joined)
	}
}

func seedTerminalBacklog(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO notification_jobs
			(id, event_id, order_id, dedupe_key, channel, recipient_email, locale, template_key,
			 payload_ciphertext, payload, status, provider, dispatcher, type, retry_count, next_retry_at, created_at, updated_at)
		SELECT gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'dedupe-' || g, 'email',
		       'buyer@example.test', 'en', 'order_paid', '{}', '{}',
		       CASE WHEN g % 500 = 0 THEN 'dead' WHEN g % 97 = 0 THEN 'pending' ELSE 'sent' END,
		       'smtp', 'scheduler', 'order_paid', 0, CURRENT_TIMESTAMP,
		       CURRENT_TIMESTAMP - INTERVAL '200 days', CURRENT_TIMESTAMP - INTERVAL '200 days'
		FROM generate_series(1, ?) g`, rows).Error; err != nil {
		t.Fatal(err)
	}
	// Without fresh statistics the planner has no reason to prefer the index.
	if err := db.Exec(`ANALYZE notification_jobs`).Error; err != nil {
		t.Fatal(err)
	}
}
