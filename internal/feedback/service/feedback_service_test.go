package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"go.uber.org/zap"
)

// --- Mocks ---

type mockFeedbackRepo struct {
	createFn   func(ctx context.Context, f *domain.Feedback) error
	findByIDFn func(ctx context.Context, id uuid.UUID) (*domain.Feedback, error)
	findAllFn  func(ctx context.Context, pgn pagination.Params) ([]domain.Feedback, int64, error)
}

func (m *mockFeedbackRepo) Create(ctx context.Context, f *domain.Feedback) error {
	return m.createFn(ctx, f)
}

func (m *mockFeedbackRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Feedback, error) {
	return m.findByIDFn(ctx, id)
}

func (m *mockFeedbackRepo) FindAll(ctx context.Context, pgn pagination.Params) ([]domain.Feedback, int64, error) {
	return m.findAllFn(ctx, pgn)
}

type mockEmailProvider struct {
	email.Provider
	sendFeedbackEmailFn func(to string, feedbackType string, userEmail string, content string, mediaURL *string) error
}

func (m *mockEmailProvider) SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
	if m.sendFeedbackEmailFn != nil {
		return m.sendFeedbackEmailFn(to, feedbackType, userEmail, content, mediaURL)
	}
	return nil
}

type mockStorage struct {
	uploadFn func(ctx context.Context, file interface{}, folder, filename string) (string, error)
	deleteFn func(ctx context.Context, publicID string) error
}

func (m *mockStorage) Upload(ctx context.Context, file interface{}, folder, filename string) (string, error) {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, file, folder, filename)
	}
	return "", nil
}

func (m *mockStorage) Delete(ctx context.Context, publicID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, publicID)
	}
	return nil
}

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Info(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Warn(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Error(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Fatal(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Debugf(template string, args ...interface{}) {}
func (m *mockLogger) Infof(template string, args ...interface{})  {}
func (m *mockLogger) Warnf(template string, args ...interface{})  {}
func (m *mockLogger) Errorf(template string, args ...interface{}) {}
func (m *mockLogger) Fatalf(template string, args ...interface{}) {}
func (m *mockLogger) Debugw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Infow(msg string, kvs ...interface{})        {}
func (m *mockLogger) Warnw(msg string, kvs ...interface{})        {}
func (m *mockLogger) Errorw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Fatalw(msg string, kvs ...interface{})       {}
func (m *mockLogger) With(fields ...zap.Field) logger.Logger      { return m }
func (m *mockLogger) Sync() error                                 { return nil }

// --- Tests ---

func TestFeedbackService_Create_Success(t *testing.T) {
	repo := &mockFeedbackRepo{
		createFn: func(ctx context.Context, f *domain.Feedback) error {
			return nil
		},
	}

	emailChan := make(chan bool, 1)
	emailProv := &mockEmailProvider{
		sendFeedbackEmailFn: func(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
			if to == "admin@aquawheel.store" && feedbackType == "bug" && userEmail == "user@example.com" {
				emailChan <- true
			}
			return nil
		},
	}

	store := &mockStorage{
		uploadFn: func(ctx context.Context, file interface{}, folder, filename string) (string, error) {
			return "https://media.url/test.png", nil
		},
	}

	cfg := &config.Config{
		AdminNotificationEmail: "admin@aquawheel.store",
	}

	svc := NewFeedbackService(repo, emailProv, store, cfg, &mockLogger{})

	res, err := svc.Create(context.Background(), "bug", "user@example.com", "Page crashes", "dummy-file-content", "test.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Type != "bug" {
		t.Errorf("expected type 'bug', got %s", res.Type)
	}
	if res.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got %s", res.Email)
	}
	if res.Content != "Page crashes" {
		t.Errorf("expected content 'Page crashes', got %s", res.Content)
	}
	if res.MediaURL == nil || *res.MediaURL != "https://media.url/test.png" {
		t.Errorf("expected media URL 'https://media.url/test.png', got %v", res.MediaURL)
	}

	// Verify asynchronous email notification
	select {
	case <-emailChan:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for email notification")
	}
}

func TestFeedbackService_Create_InvalidType(t *testing.T) {
	svc := NewFeedbackService(&mockFeedbackRepo{}, &mockEmailProvider{}, &mockStorage{}, &config.Config{}, &mockLogger{})

	_, err := svc.Create(context.Background(), "invalid_type", "user@example.com", "Help!", nil, "")
	if !errors.Is(err, domain.ErrInvalidFeedbackType) {
		t.Errorf("expected ErrInvalidFeedbackType, got %v", err)
	}
}

func TestFeedbackService_GetByID(t *testing.T) {
	fID := uuid.New()
	repo := &mockFeedbackRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Feedback, error) {
			if id == fID {
				return &domain.Feedback{ID: fID, Type: "bug"}, nil
			}
			return nil, domain.ErrFeedbackNotFound
		},
	}

	svc := NewFeedbackService(repo, &mockEmailProvider{}, &mockStorage{}, &config.Config{}, &mockLogger{})

	res, err := svc.GetByID(context.Background(), fID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != fID {
		t.Errorf("expected ID %v, got %v", fID, res.ID)
	}
}

func TestFeedbackService_GetList(t *testing.T) {
	repo := &mockFeedbackRepo{
		findAllFn: func(ctx context.Context, pgn pagination.Params) ([]domain.Feedback, int64, error) {
			return []domain.Feedback{
				{Type: "bug"},
				{Type: "product_improvement"},
			}, 2, nil
		},
	}

	svc := NewFeedbackService(repo, &mockEmailProvider{}, &mockStorage{}, &config.Config{}, &mockLogger{})

	res, meta, err := svc.GetList(context.Background(), pagination.Params{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 2 {
		t.Errorf("expected 2 feedbacks, got %d", len(res))
	}
	if meta.TotalItems != 2 {
		t.Errorf("expected total items 2, got %d", meta.TotalItems)
	}
}
