package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

type MockAttributeRepository struct {
	mock.Mock
}

func (m *MockAttributeRepository) FindAll(ctx context.Context, lang string) ([]domain.Attribute, error) {
	args := m.Called(ctx, lang)
	if args.Get(0) != nil {
		return args.Get(0).([]domain.Attribute), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockAttributeRepository) FindByID(ctx context.Context, id int, lang string) (*domain.Attribute, error) {
	args := m.Called(ctx, id, lang)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Attribute), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockAttributeRepository) FindValues(ctx context.Context, attributeCode, lang string) ([]domain.AttrValueOption, error) {
	args := m.Called(ctx, attributeCode, lang)
	if args.Get(0) != nil {
		return args.Get(0).([]domain.AttrValueOption), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockAttributeRepository) Create(ctx context.Context, attr *domain.Attribute) error {
	args := m.Called(ctx, attr)
	return args.Error(0)
}

func (m *MockAttributeRepository) Update(ctx context.Context, attr *domain.Attribute) error {
	args := m.Called(ctx, attr)
	return args.Error(0)
}

func (m *MockAttributeRepository) Delete(ctx context.Context, id int) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockAttributeRepository) UpdateOrder(ctx context.Context, ids []int) error {
	args := m.Called(ctx, ids)
	return args.Error(0)
}

func setupAttributeService() (*MockAttributeRepository, domain.AttributeService) {
	mockRepo := new(MockAttributeRepository)
	loggerInstance := &noopLogger{}
	svc := NewAttributeService(mockRepo, loggerInstance)
	return mockRepo, svc
}

func TestAttributeService_GetAll(t *testing.T) {
	mockRepo, svc := setupAttributeService()
	ctx := context.Background()

	expected := []domain.Attribute{{ID: 1, Code: "color"}}
	mockRepo.On("FindAll", ctx, "uk").Return(expected, nil)

	res, err := svc.GetAll(ctx, "uk")
	require.NoError(t, err)
	assert.Equal(t, expected, res)
	mockRepo.AssertExpectations(t)
}

func TestAttributeService_GetByID(t *testing.T) {
	mockRepo, svc := setupAttributeService()
	ctx := context.Background()

	expected := &domain.Attribute{ID: 1, Code: "color"}
	mockRepo.On("FindByID", ctx, 1, "uk").Return(expected, nil)

	res, err := svc.GetByID(ctx, 1, "uk")
	require.NoError(t, err)
	assert.Equal(t, expected, res)
	mockRepo.AssertExpectations(t)
}

func TestAttributeService_Create_TranslationFallback(t *testing.T) {
	mockRepo, svc := setupAttributeService()
	ctx := context.Background()

	attr := &domain.Attribute{
		Code: "color",
		Translations: []domain.AttributeTranslation{
			{LanguageCode: "uk", Name: "Колір"},
			{LanguageCode: "en", Name: ""}, // Should fallback to uk
		},
	}

	mockRepo.On("Create", ctx, attr).Return(nil)

	err := svc.Create(ctx, attr)
	require.NoError(t, err)
	
	assert.Equal(t, "Колір", attr.Translations[1].Name)
	mockRepo.AssertExpectations(t)
}

func TestAttributeService_Update(t *testing.T) {
	mockRepo, svc := setupAttributeService()
	ctx := context.Background()

	attr := &domain.Attribute{
		ID: 1,
		Code: "color",
		Translations: []domain.AttributeTranslation{
			{LanguageCode: "uk", Name: "Колір"},
		},
	}

	mockRepo.On("Update", ctx, attr).Return(nil)

	err := svc.Update(ctx, attr)
	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestAttributeService_Delete(t *testing.T) {
	mockRepo, svc := setupAttributeService()
	ctx := context.Background()

	mockRepo.On("Delete", ctx, 1).Return(nil)

	err := svc.Delete(ctx, 1)
	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestAttributeService_UpdateOrder(t *testing.T) {
	mockRepo, svc := setupAttributeService()
	ctx := context.Background()

	ids := []int{2, 1, 3}
	mockRepo.On("UpdateOrder", ctx, ids).Return(nil)

	err := svc.UpdateOrder(ctx, ids)
	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
}
