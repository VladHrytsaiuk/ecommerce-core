package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestStorefrontRouteDoesNotConflictWithLocalizedCatalogRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	v1.Group("/catalog/:lang").GET("/products", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	productID := uuid.New()
	RegisterStorefrontRoutes(v1.Group("/catalog/products"), newTestStorefront(t, productID), apiresponse.NewErrorRenderer(nil))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products/"+productID.String()+"/videos", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	encoded := response.Body.String()
	if _, leaked := body["provider"]; leaked || containsSensitiveVideoField(encoded, "external_id") || containsSensitiveVideoField(encoded, "video_asset_id") || containsSensitiveVideoField(encoded, "stream-id") {
		t.Fatalf("storefront response leaked provider metadata: %s", encoded)
	}
}

func containsSensitiveVideoField(body, field string) bool { return strings.Contains(body, field) }

func newTestStorefront(t *testing.T, productID uuid.UUID) *videoApp.StorefrontService {
	t.Helper()
	reader := readyVideos{videos: []video.ProductVideo{{
		ID: uuid.New(), ProductID: productID, AssetID: uuid.New(), Role: "preview", Position: 0,
		Asset: video.Asset{Provider: "cloudflare", ExternalID: "stream-id", Status: video.AssetReady},
	}}}
	service, err := videoApp.NewStorefrontService(reader, tokenSigner{}, time.Hour)
	if err != nil {
		t.Fatalf("NewStorefrontService() error = %v", err)
	}
	return service
}

type readyVideos struct{ videos []video.ProductVideo }

func (r readyVideos) ListReadyProductVideos(_ context.Context, _ uuid.UUID) ([]video.ProductVideo, error) {
	return r.videos, nil
}

// tokenSigner stands in for Cloudflare. It reproduces the property that
// matters to this test: the asset identifier is carried inside an opaque
// token, never in the URL path where a client could read it off.
type tokenSigner struct{}

func (tokenSigner) SignPlayback(_ context.Context, externalID string, expiresAt time.Time) (video.Playback, error) {
	token := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + externalID + `"}`))
	base := "https://customer-abc123.cloudflarestream.com/" + token
	return video.Playback{HLSURL: base + "/manifest/video.m3u8", DASHURL: base + "/manifest/video.mpd", ExpiresAt: expiresAt}, nil
}

func TestStorefrontResponseCarriesASignedPlaybackGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	productID := uuid.New()
	RegisterStorefrontRoutes(router.Group("/api/v1/catalog/products"), newTestStorefront(t, productID), apiresponse.NewErrorRenderer(nil))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products/"+productID.String()+"/videos", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data []struct {
			Playback struct {
				HLS       string    `json:"hls"`
				DASH      string    `json:"dash"`
				ExpiresAt time.Time `json:"expires_at"`
			} `json:"playback"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("videos = %d, want 1", len(payload.Data))
	}
	grant := payload.Data[0].Playback
	if !strings.HasSuffix(grant.HLS, "/manifest/video.m3u8") || !strings.HasSuffix(grant.DASH, "/manifest/video.mpd") {
		t.Fatalf("playback URLs = %+v, want HLS and DASH manifests", grant)
	}
	// The deadline is what makes a copied link stop working, so it has to reach
	// the client rather than being implied.
	if grant.ExpiresAt.IsZero() || !grant.ExpiresAt.After(time.Now()) {
		t.Fatalf("playback expiry = %s, want a future deadline", grant.ExpiresAt)
	}
	// A public Stream URL puts the asset UID in this path segment. A signed one
	// puts the token there, which is the whole point.
	if strings.Contains(grant.HLS, "/stream-id/") {
		t.Fatalf("playback URL exposes the provider asset identifier: %s", grant.HLS)
	}
}
