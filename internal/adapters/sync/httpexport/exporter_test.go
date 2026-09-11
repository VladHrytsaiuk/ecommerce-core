package httpexport

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	sync "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

const testSecret = "an-erp-shared-secret"

func TestExportOrderSignsTheBodyWithTheTimestamp(t *testing.T) {
	var gotSignature, gotTimestamp, gotIdempotency, gotTopic string
	var gotBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSignature = r.Header.Get("X-Sync-Signature")
		gotTimestamp = r.Header.Get("X-Sync-Timestamp")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		gotTopic = r.Header.Get("X-Sync-Topic")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	event := testEvent()
	if err := newTestExporter(t, server).ExportOrder(context.Background(), event); err != nil {
		t.Fatalf("ExportOrder() error = %v", err)
	}

	// The receiver recomputes this to establish the request came from us and
	// was not altered; the timestamp is inside it so a captured request cannot
	// be replayed later under a fresh one.
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(gotTimestamp))
	mac.Write([]byte("."))
	mac.Write(gotBody)
	if want := hex.EncodeToString(mac.Sum(nil)); gotSignature != want {
		t.Fatalf("signature = %s, want %s", gotSignature, want)
	}
	if gotIdempotency != event.IdempotencyKey.String() || gotTopic != event.Topic {
		t.Fatalf("headers = %s/%s, want %s/%s", gotIdempotency, gotTopic, event.IdempotencyKey, event.Topic)
	}
	if string(gotBody) != string(event.Payload) {
		t.Fatalf("body = %s, want the event payload unchanged", gotBody)
	}
}

func TestExportOrderTreatsADuplicateAsSuccess(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	// The dispatcher retries after any uncertain outcome, and a lost response
	// is indistinguishable from a lost request. Reporting an error for an
	// export the ERP already holds would duplicate the order there.
	if err := newTestExporter(t, server).ExportOrder(context.Background(), testEvent()); err != nil {
		t.Fatalf("ExportOrder() error = %v, want a duplicate treated as delivered", err)
	}
}

func TestExportOrderReportsARejection(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte("missing customer reference"))
	}))
	defer server.Close()

	err := newTestExporter(t, server).ExportOrder(context.Background(), testEvent())
	if err == nil {
		t.Fatal("ExportOrder() error = nil, want the rejection surfaced for retry")
	}
	// The reason has to survive into last_error or an operator cannot act on it.
	if got := err.Error(); !strings.Contains(got, "422") || !strings.Contains(got, "missing customer reference") {
		t.Fatalf("ExportOrder() error = %q, want the status and reason", got)
	}
}

func TestExportOrderRejectsAnEmptyPayload(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("an event with no payload must never reach the ERP")
	}))
	defer server.Close()

	event := testEvent()
	event.Payload = nil
	if err := newTestExporter(t, server).ExportOrder(context.Background(), event); err == nil {
		t.Fatal("ExportOrder() error = nil, want refusal")
	}
}

func TestNewRejectsUnusableConfiguration(t *testing.T) {
	for name, config := range map[string]Config{
		"missing endpoint": {Secret: testSecret},
		"missing secret":   {Endpoint: "https://erp.example.test/orders"},
		// The payload carries order totals and item lines, so plaintext
		// transport is refused rather than left to the deployment.
		"plaintext endpoint": {Endpoint: "http://erp.example.test/orders", Secret: testSecret},
		"not a URL":          {Endpoint: "://nope", Secret: testSecret},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(config); err == nil {
				t.Fatal("New() error = nil, want refusal")
			}
		})
	}
}

func newTestExporter(t *testing.T, server *httptest.Server) *Exporter {
	t.Helper()
	exporter, err := New(Config{Endpoint: server.URL, Secret: testSecret, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return exporter
}

func testEvent() sync.OutboxEvent {
	return sync.OutboxEvent{
		ID: uuid.New(), Topic: sync.TopicOrderCreated, AggregateID: uuid.New(),
		IdempotencyKey: uuid.New(), Payload: []byte(`{"version":1,"number":"A-1"}`),
		CreatedAt: time.Now().UTC(),
	}
}
