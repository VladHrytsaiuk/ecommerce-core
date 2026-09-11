// Package httpexport delivers Sync order events to an ERP over authenticated
// HTTP. It is the provider-neutral default for a core that is deployed against
// many different back-office systems: anything that can receive a signed POST
// needs no bespoke adapter, and a protocol-specific one can replace it without
// the Sync module changing.
package httpexport

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sync "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

const (
	maxResponseBodySize = 1 << 20
	defaultTimeout      = 30 * time.Second
)

type Config struct {
	// Endpoint receives the export. HTTPS is required: the payload carries
	// order totals and item lines.
	Endpoint string
	// Secret signs the body. The receiver recomputes the HMAC to establish that
	// the request came from this deployment and was not altered in transit.
	Secret     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Exporter struct {
	endpoint string
	secret   []byte
	client   *http.Client
	now      func() time.Time
}

func New(config Config) (*Exporter, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	secret := strings.TrimSpace(config.Secret)
	if endpoint == "" || secret == "" {
		return nil, fmt.Errorf("sync export endpoint and secret are required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("sync export endpoint must be an absolute HTTPS URL")
	}
	client := config.HTTPClient
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &Exporter{endpoint: endpoint, secret: []byte(secret), client: client, now: time.Now}, nil
}

// ExportOrder delivers one event and treats a duplicate as success.
//
// The idempotency key travels in a header so the receiver can recognise a
// redelivery: the dispatcher retries after any uncertain outcome, and a
// network timeout cannot distinguish "never arrived" from "arrived and the
// response was lost". Returning an error for an export the ERP already
// recorded would duplicate the order there.
func (e *Exporter) ExportOrder(ctx context.Context, event sync.OutboxEvent) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("sync exporter is not configured")
	}
	if len(event.Payload) == 0 {
		return fmt.Errorf("sync export event %s has no payload", event.ID)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(event.Payload))
	if err != nil {
		return fmt.Errorf("build sync export request: %w", err)
	}
	timestamp := strconv.FormatInt(e.now().UTC().Unix(), 10)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", event.IdempotencyKey.String())
	request.Header.Set("X-Sync-Topic", event.Topic)
	request.Header.Set("X-Sync-Timestamp", timestamp)
	// The timestamp is inside the signature, so a captured request cannot be
	// replayed later with a fresh one.
	request.Header.Set("X-Sync-Signature", e.sign(timestamp, event.Payload))

	response, err := e.client.Do(request)
	if err != nil {
		return fmt.Errorf("call sync export endpoint: %w", err)
	}
	defer response.Body.Close()
	// Drain before closing so the connection returns to the pool instead of
	// being torn down on every export.
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize))

	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		return nil
	case response.StatusCode == http.StatusConflict:
		// The receiver already holds this idempotency key. The desired state
		// has been reached, so retrying would only re-deliver it.
		return nil
	default:
		return fmt.Errorf("sync export rejected with status %d: %s", response.StatusCode, summarize(body))
	}
}

func (e *Exporter) sign(timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, e.secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// summarize keeps a rejection diagnosable without copying an arbitrary
// upstream body into the durable last_error column.
func summarize(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "no response body"
	}
	if len(text) > 200 {
		return text[:200] + "…"
	}
	return text
}

var _ sync.OrderExporter = (*Exporter)(nil)
