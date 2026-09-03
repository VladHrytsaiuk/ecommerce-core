package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

func TestStorefrontRouteDoesNotConflictWithLocalizedCatalogRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	v1.Group("/catalog/:lang").GET("/products", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	productID := uuid.New()
	RegisterStorefrontRoutes(v1.Group("/catalog/products"), readyVideos{videos: []video.ProductVideo{{ID: uuid.New(), ProductID: productID, AssetID: uuid.New(), Role: "preview", Position: 0, Asset: video.Asset{Provider: "cloudflare", ExternalID: "stream-id", Status: video.AssetReady}}}}, apiresponse.NewErrorRenderer(nil))

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

type readyVideos struct{ videos []video.ProductVideo }

func (r readyVideos) ListReadyProductVideos(_ context.Context, _ uuid.UUID) ([]video.ProductVideo, error) {
	return r.videos, nil
}
