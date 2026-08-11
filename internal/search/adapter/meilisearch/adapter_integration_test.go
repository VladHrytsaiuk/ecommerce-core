package meilisearch_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	meili "github.com/meilisearch/meilisearch-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	adapter "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/meilisearch"
	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

func TestAdapterConfiguresAndSynchronizesDocument(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	const masterKey = "test-master-key-must-be-long-enough"
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "getmeili/meilisearch:v1.6",
			ExposedPorts: []string{"7700/tcp"},
			Env:          map[string]string{"MEILI_ENV": "development", "MEILI_MASTER_KEY": masterKey},
			WaitingFor:   wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(45 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("start Meilisearch: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "7700/tcp")
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("http://%s:%s", host, port.Port())
	indexUID := "products_test"
	index, err := adapter.New(ctx, adapter.Config{URL: url, MasterKey: masterKey, IndexUID: indexUID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = index.Close() })
	client := meili.New(url, meili.WithAPIKey(masterKey))
	settings, err := client.Index(indexUID).GetSettingsWithContext(ctx)
	if err != nil || !contains(settings.FilterableAttributes, "category_id") || !contains(settings.SortableAttributes, "price_min") {
		t.Fatalf("index settings = %+v, %v; want facet and price sort fields", settings, err)
	}
	productID := uuid.New()
	if err := index.AddOrUpdateDocuments(ctx, search.SearchDocument{ID: productID.String(), Locales: []string{"en"}, Names: []string{"Wireless headphones"}, Status: "active"}); err != nil {
		t.Fatalf("AddOrUpdateDocuments() error = %v", err)
	}
	response, err := client.Index(indexUID).SearchWithContext(ctx, "wireless", &meili.SearchRequest{})
	if err != nil || response.EstimatedTotalHits != 1 {
		t.Fatalf("Search() = %+v, %v; want one result", response, err)
	}
	if err := index.DeleteDocument(ctx, productID); err != nil {
		t.Fatalf("DeleteDocument() error = %v", err)
	}
	response, err = client.Index(indexUID).SearchWithContext(ctx, "wireless", &meili.SearchRequest{})
	if err != nil || response.EstimatedTotalHits != 0 {
		t.Fatalf("Search() after delete = %+v, %v; want no results", response, err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
