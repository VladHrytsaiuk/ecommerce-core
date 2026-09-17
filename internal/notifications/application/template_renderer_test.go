package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

func TestTemplateRendererFallsBackToDefaultLocaleAndEscapesHTML(t *testing.T) {
	repository := &templateRepository{templates: map[string]*notificationsDomain.Template{
		"en": {Key: notificationsDomain.OrderPaidTemplate, Locale: "en", Subject: "Receipt {{.OrderNumber}}", HTML: "<p>{{.OrderNumber}}</p>", Text: "Receipt {{.OrderNumber}}"},
	}}
	renderer := NewTemplateRenderer(repository, "en")
	message, err := renderer.Render(context.Background(), notificationsDomain.OrderPaidTemplate, "es", struct{ OrderNumber string }{OrderNumber: "<ES-100>"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if message.Subject != "Receipt <ES-100>" || message.Text != "Receipt <ES-100>" || message.HTML != "<p>&lt;ES-100&gt;</p>" {
		t.Fatalf("rendered message = %#v", message)
	}
	if repository.requestedLocale != "es" || repository.fallbackLocale != "en" {
		t.Fatalf("renderer lookup = %q/%q, want es/en", repository.requestedLocale, repository.fallbackLocale)
	}
}

type templateRepository struct {
	templates                       map[string]*notificationsDomain.Template
	requestedLocale, fallbackLocale string
}

func (*templateRepository) FindOrderContact(context.Context, uuid.UUID) (*notificationsDomain.OrderContact, error) {
	return nil, nil
}
func (r *templateRepository) FindTemplate(_ context.Context, _ string, locale, fallback string) (*notificationsDomain.Template, error) {
	r.requestedLocale, r.fallbackLocale = locale, fallback
	if template := r.templates[locale]; template != nil {
		return template, nil
	}
	return r.templates[fallback], nil
}
func (*templateRepository) CreateOrderPaidJob(context.Context, notificationsDomain.Job) (*notificationsDomain.Job, bool, error) {
	return nil, false, nil
}
func (*templateRepository) ClaimOrderPaidJob(context.Context, uuid.UUID, time.Time, time.Duration) (*notificationsDomain.Job, bool, error) {
	return nil, false, nil
}
func (*templateRepository) RecordAttempt(context.Context, notificationsDomain.Job, string, notificationsDomain.DeliveryReceipt, error, time.Time, int) error {
	return nil
}

var _ notificationsDomain.Repository = (*templateRepository)(nil)
