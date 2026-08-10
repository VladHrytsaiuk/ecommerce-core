package application

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	htmlTemplate "html/template"
	textTemplate "text/template"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

type TemplateRenderer struct {
	repository    notificationsDomain.Repository
	defaultLocale string
}

func NewTemplateRenderer(repository notificationsDomain.Repository, defaultLocale string) *TemplateRenderer {
	return &TemplateRenderer{repository: repository, defaultLocale: strings.TrimSpace(defaultLocale)}
}

func (r *TemplateRenderer) Render(ctx context.Context, key, locale string, data any) (notificationsDomain.EmailMessage, error) {
	if r == nil || r.repository == nil || r.defaultLocale == "" {
		return notificationsDomain.EmailMessage{}, fmt.Errorf("notification template renderer is not configured")
	}
	template, err := r.repository.FindTemplate(ctx, key, strings.TrimSpace(locale), r.defaultLocale)
	if err != nil {
		return notificationsDomain.EmailMessage{}, fmt.Errorf("find notification template: %w", err)
	}
	if template == nil {
		return notificationsDomain.EmailMessage{}, fmt.Errorf("notification template %q is unavailable", key)
	}
	subject, err := renderText("subject", template.Subject, data)
	if err != nil {
		return notificationsDomain.EmailMessage{}, err
	}
	html, err := renderHTML("html", template.HTML, data)
	if err != nil {
		return notificationsDomain.EmailMessage{}, err
	}
	text, err := renderText("text", template.Text, data)
	if err != nil {
		return notificationsDomain.EmailMessage{}, err
	}
	return notificationsDomain.EmailMessage{Subject: subject, HTML: html, Text: text}, nil
}

func renderText(name, source string, data any) (string, error) {
	parsed, err := textTemplate.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse notification %s template: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", fmt.Errorf("render notification %s template: %w", name, err)
	}
	return output.String(), nil
}

func renderHTML(name, source string, data any) (string, error) {
	parsed, err := htmlTemplate.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse notification %s template: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", fmt.Errorf("render notification %s template: %w", name, err)
	}
	return output.String(), nil
}

var _ notificationsDomain.TemplateRenderer = (*TemplateRenderer)(nil)
