package cloudflare

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestCreateDirectUploadUsesBearerTokenAndAllowedOrigins(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts/account/stream/direct_upload" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		var request struct {
			AllowedOrigins []string          `json:"allowedOrigins"`
			Meta           map[string]string `json:"meta"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.AllowedOrigins) != 1 || request.AllowedOrigins[0] != "https://shop.example.test" || request.Meta["video_asset_id"] == "" {
			t.Fatalf("unexpected request body: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"uid":"stream-video","uploadURL":"https://upload.example.test/tus"}}`))
	}))
	defer server.Close()
	adapter := &Adapter{accountID: "account", apiToken: "token", allowedOrigins: []string{"https://shop.example.test"}, baseURL: server.URL, httpClient: server.Client()}

	instruction, err := adapter.CreateDirectUpload(context.Background(), video.DirectUploadMetadata{AssetID: uuid.New(), MaxDurationSeconds: 90})
	if err != nil {
		t.Fatalf("CreateDirectUpload() error = %v", err)
	}
	if instruction.ExternalID != "stream-video" || instruction.UploadURL == "" {
		t.Fatalf("unexpected instruction: %+v", instruction)
	}
}

func TestDeleteAssetUsesProviderIDOnlyServerSide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/accounts/account/stream/stream-video" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	adapter := &Adapter{accountID: "account", apiToken: "token", baseURL: server.URL, httpClient: server.Client()}
	if err := adapter.DeleteAsset(context.Background(), "stream-video"); err != nil {
		t.Fatalf("DeleteAsset() error = %v", err)
	}
}

func TestVerifyWebhookSignatureChecksTimestampAndHMAC(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"uid":"stream-video","status":{"state":"ready"},"thumbnail":"https://video.example.test/poster.jpg","duration":5.5}`)
	adapter := &Adapter{webhookSecret: []byte("secret"), now: func() time.Time { return now }}
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d.", now.Unix())))
	_, _ = mac.Write(body)
	signature := fmt.Sprintf("time=%d,sig1=%x", now.Unix(), mac.Sum(nil))
	if err := adapter.VerifyWebhookSignature(context.Background(), body, signature); err != nil {
		t.Fatalf("VerifyWebhookSignature() error = %v", err)
	}
	if err := adapter.VerifyWebhookSignature(context.Background(), body, "time=1,sig1=00"); err != video.ErrInvalidWebhookSignature {
		t.Fatalf("invalid signature error = %v", err)
	}
}

func TestParseWebhookNormalizesCloudflareReadyPayload(t *testing.T) {
	adapter := &Adapter{}
	event, err := adapter.ParseWebhook(context.Background(), []byte(`{"uid":"stream-video","status":{"state":"ready"},"thumbnail":"https://video.example.test/poster.jpg","duration":5.5}`))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if event.Outcome != video.WebhookEncodingSuccess || event.DurationSeconds != 6 || event.ExternalID != "stream-video" {
		t.Fatalf("unexpected event: %+v", event)
	}
}
