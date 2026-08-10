// Package application implements promotion use cases without HTTP concerns.
package application

import (
	"context"
	"fmt"

	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
)

type AdminService struct{ repository promosDomain.AdminRepository }

func NewAdminService(repository promosDomain.AdminRepository) *AdminService {
	return &AdminService{repository: repository}
}

func (s *AdminService) Create(ctx context.Context, code promosDomain.Code) (*promosDomain.Code, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("promo admin service is not configured")
	}
	return s.repository.Create(ctx, code)
}

var _ promosDomain.AdminService = (*AdminService)(nil)
