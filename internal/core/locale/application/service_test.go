package application

import (
	"context"
	"testing"

	localeDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/domain"
)

func TestSynchronizeBuildsConfiguredThreeLocaleSet(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	if err := service.Synchronize(context.Background(), []string{"es", "en", "ca"}, "es"); err != nil {
		t.Fatalf("Synchronize() error = %v", err)
	}
	if len(repo.locales) != 3 || !repo.locales[0].IsDefault || repo.locales[0].Code != "es" {
		t.Fatalf("locales = %+v, want configured locale set with es default", repo.locales)
	}
}

type fakeRepository struct{ locales []localeDomain.Locale }

func (r *fakeRepository) Synchronize(_ context.Context, locales []localeDomain.Locale) error {
	r.locales = locales
	return nil
}
