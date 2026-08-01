package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func newBadgeSvc(repo *MockBadgeRepository) domain.BadgeService {
	return NewBadgeService(repo, &noopLogger{})
}

func TestBadgeService_GetAll(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	badges := []domain.Badge{
		{ID: 1, Name: domain.LocalizedMap{"uk": "Новинка"}, ColorHex: "#000"},
		{ID: 2, Name: domain.LocalizedMap{"uk": "Акція"}, ColorHex: "#FFF"},
	}

	repo.On("FindAll", mock.Anything).Return(badges, nil)

	res, err := svc.GetAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, res, 2)
	assert.Equal(t, 1, res[0].ID)
}

func TestBadgeService_GetByID(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	expected := &domain.Badge{ID: 1, Name: domain.LocalizedMap{"uk": "Хіт"}, ColorHex: "#CCC"}
	repo.On("FindByID", mock.Anything, 1).Return(expected, nil)

	res, err := svc.GetByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 1, res.ID)
	assert.Equal(t, "Хіт", res.Name["uk"])
}

func TestBadgeService_GetByID_NotFound(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	repo.On("FindByID", mock.Anything, 99).Return(nil, domain.ErrBadgeNotFound)

	res, err := svc.GetByID(context.Background(), 99)
	assert.ErrorIs(t, err, domain.ErrBadgeNotFound)
	assert.Nil(t, res)
}

func TestBadgeService_Create(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	badge := &domain.Badge{Name: domain.LocalizedMap{"uk": "Тест"}, ColorHex: "#111"}
	repo.On("Create", mock.Anything, badge).Return(nil)

	err := svc.Create(context.Background(), badge)
	require.NoError(t, err)
}

func TestBadgeService_Update(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	badge := &domain.Badge{ID: 1, Name: domain.LocalizedMap{"uk": "Тест оновлений"}, ColorHex: "#222"}
	repo.On("Update", mock.Anything, badge).Return(nil)

	err := svc.Update(context.Background(), badge)
	require.NoError(t, err)
}

func TestBadgeService_Delete_Success(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	repo.On("CountUsage", mock.Anything, 1).Return(int64(0), nil)
	repo.On("Delete", mock.Anything, 1).Return(nil)

	err := svc.Delete(context.Background(), 1)
	require.NoError(t, err)
	repo.AssertCalled(t, "Delete", mock.Anything, 1)
}

func TestBadgeService_Delete_InUse(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	repo.On("CountUsage", mock.Anything, 1).Return(int64(5), nil)

	err := svc.Delete(context.Background(), 1)
	assert.ErrorIs(t, err, domain.ErrBadgeInUse)
	repo.AssertNotCalled(t, "Delete")
}

func TestBadgeService_Delete_CountError(t *testing.T) {
	repo := &MockBadgeRepository{}
	svc := newBadgeSvc(repo)

	dbErr := errors.New("db error")
	repo.On("CountUsage", mock.Anything, 1).Return(int64(0), dbErr)

	err := svc.Delete(context.Background(), 1)
	assert.ErrorIs(t, err, dbErr)
	repo.AssertNotCalled(t, "Delete")
}
