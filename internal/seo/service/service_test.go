package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
)

// The column widths these bounds mirror live in migrations/modules/seo/000001.
// Without them a value that is only too long reaches PostgreSQL, the audited
// facade's transaction rolls back on "value too long for type character
// varying", and the administrator gets a 500 naming neither field nor limit.

func TestUpsertRefusesValuesWiderThanTheirColumn(t *testing.T) {
	valid := domain.UpsertCommand{ResourceType: "product", ResourceID: uuid.New(), Locale: "uk", Title: "Крем"}

	for name, mutate := range map[string]func(*domain.UpsertCommand){
		"title over 255":        func(c *domain.UpsertCommand) { c.Title = strings.Repeat("я", 256) },
		"description over 500":  func(c *domain.UpsertCommand) { c.Description = strings.Repeat("я", 501) },
		"resource type over 64": func(c *domain.UpsertCommand) { c.ResourceType = strings.Repeat("p", 65) },
		"locale over 10":        func(c *domain.UpsertCommand) { c.Locale = strings.Repeat("u", 11) },
		"keywords unbounded":    func(c *domain.UpsertCommand) { c.Keywords = strings.Repeat("к", 2001) },
		"image ref unbounded":   func(c *domain.UpsertCommand) { c.OGImageRef = strings.Repeat("u", 2001) },
	} {
		t.Run(name, func(t *testing.T) {
			repository := &recordingSEORepository{}
			command := valid
			mutate(&command)

			if _, err := New(repository).Upsert(context.Background(), command); !errors.Is(err, domain.ErrInvalidMetadata) {
				t.Fatalf("Upsert() error = %v, want ErrInvalidMetadata", err)
			}
			if repository.upserts != 0 {
				t.Fatal("an oversized value reached the database")
			}
		})
	}
}

func TestUpsertAcceptsValuesAtTheColumnLimit(t *testing.T) {
	// The bounds must be inclusive, or a title that exactly fills the column
	// would be refused for no reason.
	repository := &recordingSEORepository{}
	command := domain.UpsertCommand{
		ResourceType: "product", ResourceID: uuid.New(), Locale: "uk",
		Title:       strings.Repeat("я", 255),
		Description: strings.Repeat("я", 500),
	}

	if _, err := New(repository).Upsert(context.Background(), command); err != nil {
		t.Fatalf("Upsert() error = %v, want values at the limit accepted", err)
	}
	if repository.upserts != 1 {
		t.Fatal("a valid upsert did not reach the database")
	}
}

func TestUpsertCountsRunesNotBytes(t *testing.T) {
	// A Cyrillic title of 200 characters is 400 bytes. Measuring bytes would
	// refuse a title well inside a VARCHAR(255), which PostgreSQL counts in
	// characters.
	repository := &recordingSEORepository{}
	command := domain.UpsertCommand{ResourceType: "product", ResourceID: uuid.New(), Locale: "uk", Title: strings.Repeat("я", 200)}

	if _, err := New(repository).Upsert(context.Background(), command); err != nil {
		t.Fatalf("Upsert() error = %v; the limit must count characters, not bytes", err)
	}
}

type recordingSEORepository struct {
	domain.Repository
	upserts int
}

func (r *recordingSEORepository) Upsert(_ context.Context, command domain.UpsertCommand) (*domain.Metadata, error) {
	r.upserts++
	return &domain.Metadata{ResourceType: command.ResourceType, ResourceID: command.ResourceID, Locale: command.Locale, Title: command.Title}, nil
}
