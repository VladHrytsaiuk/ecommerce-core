// Package cloudflare adapts Cloudflare Stream to the Video provider port.
package cloudflare

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

type Config struct {
	AccountID      string
	APIToken       string
	WebhookSecret  string
	AllowedOrigins []string
	BaseURL        string // test seam; defaults to Cloudflare's HTTPS API.
	HTTPClient     *http.Client
}

type Adapter struct {
	accountID      string
	apiToken       string
	webhookSecret  []byte
	allowedOrigins []string
	baseURL        string
	httpClient     *http.Client
	now            func() time.Time
}

func New(cfg Config) (*Adapter, error) {
	accountID, token, secret := strings.TrimSpace(cfg.AccountID), strings.TrimSpace(cfg.APIToken), strings.TrimSpace(cfg.WebhookSecret)
	if accountID == "" || token == "" || secret == "" {
		return nil, fmt.Errorf("Cloudflare Stream account ID, API token and webhook secret are required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("Cloudflare Stream API URL must be an absolute HTTPS URL")
	}
	origins := make([]string, 0, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		parsed, parseErr := url.Parse(origin)
		if parseErr != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return nil, fmt.Errorf("invalid Cloudflare Stream allowed origin")
		}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("at least one Cloudflare Stream allowed origin is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Adapter{accountID: accountID, apiToken: token, webhookSecret: []byte(secret), allowedOrigins: origins, baseURL: baseURL, httpClient: client, now: time.Now}, nil
}

func (a *Adapter) CreateDirectUpload(ctx context.Context, metadata video.DirectUploadMetadata) (video.UploadInstruction, error) {
	if a == nil || a.httpClient == nil || metadata.AssetID == uuid.Nil {
		return video.UploadInstruction{}, fmt.Errorf("invalid Cloudflare Stream direct upload request")
	}
	payload := struct {
		AllowedOrigins     []string          `json:"allowedOrigins"`
		MaxDurationSeconds int               `json:"maxDurationSeconds,omitempty"`
		Meta               map[string]string `json:"meta"`
	}{AllowedOrigins: a.allowedOrigins, MaxDurationSeconds: metadata.MaxDurationSeconds, Meta: map[string]string{"video_asset_id": metadata.AssetID.String()}}
	body, err := json.Marshal(payload)
	if err != nil {
		return video.UploadInstruction{}, fmt.Errorf("marshal Cloudflare Stream request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/accounts/"+url.PathEscape(a.accountID)+"/stream/direct_upload", bytes.NewReader(body))
	if err != nil {
		return video.UploadInstruction{}, fmt.Errorf("build Cloudflare Stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := a.httpClient.Do(req)
	if err != nil {
		return video.UploadInstruction{}, fmt.Errorf("call Cloudflare Stream: %w", err)
	}
	defer response.Body.Close()
	var decoded struct {
		Success bool `json:"success"`
		Result  struct {
			UID       string `json:"uid"`
			UploadURL string `json:"uploadURL"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&decoded); err != nil {
		return video.UploadInstruction{}, fmt.Errorf("decode Cloudflare Stream response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !decoded.Success || decoded.Result.UID == "" || decoded.Result.UploadURL == "" {
		return video.UploadInstruction{}, fmt.Errorf("Cloudflare Stream rejected direct upload")
	}
	return video.UploadInstruction{ExternalID: decoded.Result.UID, UploadURL: decoded.Result.UploadURL}, nil
}

// DeleteAsset removes a stale direct-upload asset. A 404 is idempotent: the
// desired provider state (no asset) has already been reached.
func (a *Adapter) DeleteAsset(ctx context.Context, externalID string) error {
	if a == nil || a.httpClient == nil || strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("invalid Cloudflare Stream asset deletion request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.baseURL+"/accounts/"+url.PathEscape(a.accountID)+"/stream/"+url.PathEscape(externalID), nil)
	if err != nil {
		return fmt.Errorf("build Cloudflare Stream deletion request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiToken)
	response, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Cloudflare Stream deletion: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read Cloudflare Stream deletion response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Cloudflare Stream rejected asset deletion")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var decoded struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || !decoded.Success {
		return fmt.Errorf("Cloudflare Stream rejected asset deletion")
	}
	return nil
}

// VerifyWebhookSignature validates Stream's `time=<unix>,sig1=<hex>` HMAC.
// It rejects stale callbacks before comparing the hex-decoded hash in constant
// time, preventing both replay and timing attacks.
func (a *Adapter) VerifyWebhookSignature(_ context.Context, payload []byte, signature string) error {
	if a == nil || len(a.webhookSecret) == 0 || len(payload) == 0 {
		return video.ErrInvalidWebhookSignature
	}
	unix, given, ok := parseSignature(signature)
	if !ok {
		return video.ErrInvalidWebhookSignature
	}
	now := time.Now
	if a.now != nil {
		now = a.now
	}
	sentAt := time.Unix(unix, 0)
	if sentAt.Before(now().UTC().Add(-5*time.Minute)) || sentAt.After(now().UTC().Add(5*time.Minute)) {
		return video.ErrInvalidWebhookSignature
	}
	mac := hmac.New(sha256.New, a.webhookSecret)
	_, _ = mac.Write([]byte(fmt.Sprintf("%d.", unix)))
	_, _ = mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), given) {
		return video.ErrInvalidWebhookSignature
	}
	return nil
}

func (a *Adapter) ParseWebhook(_ context.Context, payload []byte) (video.WebhookEvent, error) {
	var decoded struct {
		Event     string  `json:"event"`
		UID       string  `json:"uid"`
		Thumbnail string  `json:"thumbnail"`
		Duration  float64 `json:"duration"`
		Status    struct {
			State string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil || strings.TrimSpace(decoded.UID) == "" {
		return video.WebhookEvent{}, video.ErrInvalidWebhookPayload
	}
	state := strings.ToLower(strings.TrimSpace(decoded.Status.State))
	eventName := strings.ToLower(strings.TrimSpace(decoded.Event))
	var outcome video.WebhookOutcome
	switch {
	case eventName == "video.encoding.success" || state == "ready":
		outcome = video.WebhookEncodingSuccess
	case eventName == "video.encoding.error" || state == "error":
		outcome = video.WebhookEncodingError
	default:
		return video.WebhookEvent{}, video.ErrInvalidWebhookPayload
	}
	if math.IsNaN(decoded.Duration) || math.IsInf(decoded.Duration, 0) || decoded.Duration < 0 || decoded.Duration > math.MaxInt32 {
		return video.WebhookEvent{}, video.ErrInvalidWebhookPayload
	}
	if outcome == video.WebhookEncodingSuccess && strings.TrimSpace(decoded.Thumbnail) == "" {
		return video.WebhookEvent{}, video.ErrInvalidWebhookPayload
	}
	if len(decoded.Thumbnail) > 2048 {
		return video.WebhookEvent{}, video.ErrInvalidWebhookPayload
	}
	if outcome == video.WebhookEncodingSuccess {
		poster, err := url.Parse(decoded.Thumbnail)
		if err != nil || poster.Scheme != "https" || poster.Host == "" {
			return video.WebhookEvent{}, video.ErrInvalidWebhookPayload
		}
	}
	return video.WebhookEvent{ExternalID: decoded.UID, Outcome: outcome, DurationSeconds: int(math.Ceil(decoded.Duration)), PosterURL: decoded.Thumbnail, OccurredAt: time.Now().UTC()}, nil
}

func parseSignature(header string) (int64, []byte, bool) {
	var rawTime, rawSignature string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || value == "" {
			continue
		}
		switch key {
		case "time":
			rawTime = value
		case "sig1":
			rawSignature = value
		}
	}
	if rawTime == "" || rawSignature == "" {
		return 0, nil, false
	}
	timestamp, err := strconv.ParseInt(rawTime, 10, 64)
	if err != nil || timestamp <= 0 {
		return 0, nil, false
	}
	decoded, err := hex.DecodeString(rawSignature)
	if err != nil || len(decoded) != sha256.Size {
		return 0, nil, false
	}
	return timestamp, decoded, true
}

var _ video.VideoProvider = (*Adapter)(nil)
