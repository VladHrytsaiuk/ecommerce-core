// Package meilisearch implements the Search index port with the official SDK.
package meilisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	meili "github.com/meilisearch/meilisearch-go"

	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

type Config struct {
	URL         string
	MasterKey   string
	IndexUID    string
	Timeout     time.Duration
	TaskTimeout time.Duration
}

// DefaultTaskTimeout bounds an entire asynchronous Meilisearch task lifecycle,
// including polling. It is deliberately longer than one HTTP request but much
// shorter than an Outbox delivery lease.
const DefaultTaskTimeout = 30 * time.Second

type Adapter struct {
	client      meili.ServiceManager
	index       meili.IndexManager
	taskTimeout time.Duration
}

func (a *Adapter) Close() error {
	if a != nil && a.client != nil {
		a.client.Close()
	}
	return nil
}

func New(ctx context.Context, config Config) (*Adapter, error) {
	config.URL = strings.TrimRight(strings.TrimSpace(config.URL), "/")
	config.IndexUID = strings.TrimSpace(config.IndexUID)
	if config.URL == "" || config.MasterKey == "" || config.IndexUID == "" {
		return nil, fmt.Errorf("Meilisearch URL, master key and index UID are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.TaskTimeout <= 0 {
		config.TaskTimeout = DefaultTaskTimeout
	}
	startupCtx, cancel := context.WithTimeout(ctx, config.TaskTimeout)
	defer cancel()
	client := meili.New(config.URL, meili.WithAPIKey(config.MasterKey), meili.WithCustomClient(&http.Client{Timeout: config.Timeout}))
	if _, err := client.HealthWithContext(startupCtx); err != nil {
		return nil, fmt.Errorf("connect to Meilisearch: %w", err)
	}
	if _, err := client.GetIndexWithContext(startupCtx, config.IndexUID); err != nil {
		task, createErr := client.CreateIndexWithContext(startupCtx, &meili.IndexConfig{Uid: config.IndexUID, PrimaryKey: "id"})
		if createErr != nil {
			// A second replica can win index creation between Get and Create.
			if _, retryErr := client.GetIndexWithContext(startupCtx, config.IndexUID); retryErr != nil {
				return nil, fmt.Errorf("create Meilisearch index: %w", createErr)
			}
		} else if err := wait(startupCtx, client, task); err != nil {
			return nil, fmt.Errorf("create Meilisearch index task: %w", err)
		}
	}
	adapter := &Adapter{client: client, index: client.Index(config.IndexUID), taskTimeout: config.TaskTimeout}
	if err := adapter.configure(startupCtx); err != nil {
		return nil, err
	}
	return adapter, nil
}

func (a *Adapter) configure(ctx context.Context) error {
	if a == nil || a.index == nil {
		return fmt.Errorf("Meilisearch adapter is not configured")
	}
	settings := &meili.Settings{
		SearchableAttributes: []string{"names", "descriptions", "slugs", "brand"},
		FilterableAttributes: []string{"locales", "category_id", "status", "brand", "attributes", "price_min", "price_max", "currency", "in_stock"},
		SortableAttributes:   []string{"price_min", "price_max", "updated_at"},
	}
	task, err := a.index.UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("configure Meilisearch index: %w", err)
	}
	return wait(ctx, a.client, task)
}

func (a *Adapter) AddOrUpdateDocuments(ctx context.Context, documents ...search.SearchDocument) error {
	if a == nil || a.index == nil || len(documents) == 0 {
		return fmt.Errorf("Meilisearch document batch is required")
	}
	taskCtx, cancel := a.taskContext(ctx)
	defer cancel()
	task, err := a.index.AddDocumentsWithContext(taskCtx, documents, "id")
	if err != nil {
		return fmt.Errorf("submit Meilisearch documents: %w", err)
	}
	return wait(taskCtx, a.client, task)
}

func (a *Adapter) DeleteDocument(ctx context.Context, productID uuid.UUID) error {
	if a == nil || a.index == nil || productID == uuid.Nil {
		return fmt.Errorf("Meilisearch product ID is required")
	}
	taskCtx, cancel := a.taskContext(ctx)
	defer cancel()
	task, err := a.index.DeleteDocumentWithContext(taskCtx, productID.String())
	if err != nil {
		return fmt.Errorf("delete Meilisearch document: %w", err)
	}
	return wait(taskCtx, a.client, task)
}

func (a *Adapter) taskContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := DefaultTaskTimeout
	if a != nil && a.taskTimeout > 0 {
		timeout = a.taskTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (a *Adapter) Search(ctx context.Context, query search.ProductSearchQuery) (search.ProductSearchResult, error) {
	if a == nil || a.index == nil {
		return search.ProductSearchResult{}, fmt.Errorf("Meilisearch adapter is not configured")
	}
	offset := int64((query.Page - 1) * query.Limit)
	response, err := a.index.SearchWithContext(ctx, query.Query, &meili.SearchRequest{
		Offset: offset, Limit: int64(query.Limit), Filter: query.Filter, Sort: query.Sort,
		Facets: []string{"brand", "category_id", "attributes", "in_stock"},
	})
	if err != nil {
		return search.ProductSearchResult{}, fmt.Errorf("query Meilisearch: %w", err)
	}
	documents := make([]search.SearchDocument, 0, len(response.Hits))
	for _, hit := range response.Hits {
		encoded, err := json.Marshal(hit)
		if err != nil {
			return search.ProductSearchResult{}, fmt.Errorf("encode Meilisearch hit: %w", err)
		}
		var document search.SearchDocument
		if err := json.Unmarshal(encoded, &document); err != nil {
			return search.ProductSearchResult{}, fmt.Errorf("decode Meilisearch hit: %w", err)
		}
		documents = append(documents, document)
	}
	return search.ProductSearchResult{Documents: documents, Total: response.EstimatedTotalHits, Facets: decodeFacets(response.FacetDistribution)}, nil
}

func (a *Adapter) Autocomplete(ctx context.Context, query search.ProductSearchQuery) ([]search.Suggestion, error) {
	if a == nil || a.index == nil {
		return nil, fmt.Errorf("Meilisearch adapter is not configured")
	}
	response, err := a.index.SearchWithContext(ctx, query.Query, &meili.SearchRequest{
		Limit: int64(query.Limit), Filter: query.Filter,
		AttributesToRetrieve: []string{"id", "locales", "names", "slugs"},
	})
	if err != nil {
		return nil, fmt.Errorf("autocomplete from Meilisearch: %w", err)
	}
	suggestions := make([]search.Suggestion, 0, len(response.Hits))
	for _, hit := range response.Hits {
		encoded, err := json.Marshal(hit)
		if err != nil {
			return nil, fmt.Errorf("encode Meilisearch suggestion: %w", err)
		}
		var document search.SearchDocument
		if err := json.Unmarshal(encoded, &document); err != nil || document.ID == "" {
			if err != nil {
				return nil, fmt.Errorf("decode Meilisearch suggestion: %w", err)
			}
			continue
		}
		name, slug := localizedValue(document, query.Locale)
		if name != "" {
			suggestions = append(suggestions, search.Suggestion{ProductID: document.ID, Name: name, Slug: slug})
		}
	}
	return suggestions, nil
}

func localizedValue(document search.SearchDocument, locale string) (string, string) {
	for index, candidate := range document.Locales {
		if candidate == locale && index < len(document.Names) {
			name := document.Names[index]
			if index < len(document.Slugs) {
				return name, document.Slugs[index]
			}
			return name, ""
		}
	}
	if len(document.Names) == 0 {
		return "", ""
	}
	slug := ""
	if len(document.Slugs) > 0 {
		slug = document.Slugs[0]
	}
	return document.Names[0], slug
}

func decodeFacets(value interface{}) search.Facets {
	if value == nil {
		return search.Facets{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return search.Facets{}
	}
	var facets search.Facets
	if err := json.Unmarshal(encoded, &facets); err != nil {
		return search.Facets{}
	}
	return facets
}

func wait(ctx context.Context, client meili.ServiceManager, task *meili.TaskInfo) error {
	if task == nil {
		return fmt.Errorf("Meilisearch task is missing")
	}
	result, err := client.WaitForTaskWithContext(ctx, task.TaskUID, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if result.Status != meili.TaskStatusSucceeded {
		return fmt.Errorf("Meilisearch task %d ended with %s", task.TaskUID, result.Status)
	}
	return nil
}

var _ search.DocumentIndexer = (*Adapter)(nil)
var _ search.ProductSearchEngine = (*Adapter)(nil)
