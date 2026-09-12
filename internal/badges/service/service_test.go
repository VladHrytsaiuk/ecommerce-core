package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
)

// The bounds mirror the column widths in migrations/modules/badges/000001.
// Checking them here turns a value that is merely too long into a refusal the
// caller can act on, instead of a rolled-back transaction and a 500.

func TestCreateRefusesValuesWiderThanTheirColumn(t *testing.T) {
	valid := domain.CreateCommand{Slug: "sale", Color: "#ff0000", Translations: []domain.Translation{{Locale: "uk", Name: "Розпродаж"}}}

	for name, mutate := range map[string]func(*domain.CreateCommand){
		"slug over 120":  func(c *domain.CreateCommand) { c.Slug = strings.Repeat("s", 121) },
		"color over 32":  func(c *domain.CreateCommand) { c.Color = strings.Repeat("c", 33) },
		"locale over 10": func(c *domain.CreateCommand) { c.Translations[0].Locale = strings.Repeat("u", 11) },
		"name over 120":  func(c *domain.CreateCommand) { c.Translations[0].Name = strings.Repeat("я", 121) },
	} {
		t.Run(name, func(t *testing.T) {
			repository := &recordingBadgeRepository{}
			command := domain.CreateCommand{Slug: valid.Slug, Color: valid.Color,
				Translations: []domain.Translation{{Locale: valid.Translations[0].Locale, Name: valid.Translations[0].Name}}}
			mutate(&command)

			if _, err := New(repository).Create(context.Background(), command); !errors.Is(err, domain.ErrInvalidBadge) {
				t.Fatalf("Create() error = %v, want ErrInvalidBadge", err)
			}
			if repository.creates != 0 {
				t.Fatal("an oversized value reached the database")
			}
		})
	}
}

func TestUpdateAppliesTheSameBounds(t *testing.T) {
	// Update builds a CreateCommand and runs the same validator; a bound that
	// only guarded creation would let the same value in through an edit.
	repository := &recordingBadgeRepository{}
	command := domain.UpdateCommand{Slug: strings.Repeat("s", 121), Color: "#ff0000",
		Translations: []domain.Translation{{Locale: "uk", Name: "Розпродаж"}}}

	if _, err := New(repository).Update(context.Background(), uuid.New(), command); !errors.Is(err, domain.ErrInvalidBadge) {
		t.Fatalf("Update() error = %v, want ErrInvalidBadge", err)
	}
	if repository.updates != 0 {
		t.Fatal("an oversized value reached the database through an update")
	}
}

func TestValuesAtTheColumnLimitAreAccepted(t *testing.T) {
	repository := &recordingBadgeRepository{}
	command := domain.CreateCommand{
		Slug: strings.Repeat("s", 120), Color: strings.Repeat("c", 32),
		Translations: []domain.Translation{{Locale: strings.Repeat("u", 10), Name: strings.Repeat("я", 120)}},
	}

	if _, err := New(repository).Create(context.Background(), command); err != nil {
		t.Fatalf("Create() error = %v, want values at the limit accepted", err)
	}
	if repository.creates != 1 {
		t.Fatal("a valid badge did not reach the database")
	}
}

type recordingBadgeRepository struct {
	domain.Repository
	creates, updates int
}

func (r *recordingBadgeRepository) Create(_ context.Context, command domain.CreateCommand) (*domain.Badge, error) {
	r.creates++
	return &domain.Badge{ID: uuid.New(), Slug: command.Slug, Color: command.Color}, nil
}

func (r *recordingBadgeRepository) Update(_ context.Context, id uuid.UUID, command domain.UpdateCommand) (*domain.Badge, error) {
	r.updates++
	return &domain.Badge{ID: id, Slug: command.Slug, Color: command.Color}, nil
}
