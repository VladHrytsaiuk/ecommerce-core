package http

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type CustomerV1Handler struct {
	service identityDomain.CustomerProfileService
	errors  *apiresponse.ErrorRenderer
}

func RegisterV1CustomerRoutes(v1 *gin.RouterGroup, service identityDomain.CustomerProfileService, auth gin.HandlerFunc, renderer *apiresponse.ErrorRenderer) {
	if v1 == nil || service == nil || auth == nil || renderer == nil {
		return
	}
	h := &CustomerV1Handler{service: service, errors: renderer}
	me := v1.Group("/customers/me")
	me.Use(auth)
	me.GET("/profile", h.GetProfile)
	me.PATCH("/profile", h.PatchProfile)
	me.GET("/addresses", h.ListAddresses)
	me.POST("/addresses", h.CreateAddress)
	me.PUT("/addresses/:addressID", h.UpdateAddress)
	me.DELETE("/addresses/:addressID", h.DeleteAddress)
}

type customerProfilePatchRequest struct {
	DateOfBirth *string                    `json:"date_of_birth"`
	Gender      *string                    `json:"gender"`
	Metadata    map[string]json.RawMessage `json:"metadata"`
}

type customerAddressRequest struct {
	Title     string `json:"title" binding:"required"`
	Country   string `json:"country" binding:"required"`
	City      string `json:"city" binding:"required"`
	Line1     string `json:"line1" binding:"required"`
	Line2     string `json:"line2"`
	ZipCode   string `json:"zip_code" binding:"required"`
	IsDefault bool   `json:"is_default"`
}

type customerProfileResponse struct {
	DateOfBirth *string                    `json:"date_of_birth,omitempty"`
	Gender      string                     `json:"gender,omitempty"`
	Metadata    map[string]json.RawMessage `json:"metadata"`
	UpdatedAt   *time.Time                 `json:"updated_at,omitempty"`
}

type customerAddressResponse struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Country   string    `json:"country"`
	City      string    `json:"city"`
	Line1     string    `json:"line1"`
	Line2     string    `json:"line2,omitempty"`
	ZipCode   string    `json:"zip_code"`
	IsDefault bool      `json:"is_default"`
}

// GetProfile godoc
// @Summary Get current customer profile (v1)
// @Tags Customers v1
// @Produce json
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/profile [get]
func (h *CustomerV1Handler) GetProfile(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	profile, err := h.service.GetCustomerProfile(c.Request.Context(), userID)
	if errors.Is(err, identityDomain.ErrProfileNotFound) {
		apiresponse.Success(c, stdhttp.StatusOK, customerProfileResponse{Metadata: map[string]json.RawMessage{}})
		return
	}
	if err != nil {
		h.abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, profileResponseV1(profile))
}

// PatchProfile godoc
// @Summary Update current customer profile (v1)
// @Tags Customers v1
// @Accept json
// @Produce json
// @Param payload body customerProfilePatchRequest true "Customer profile patch"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Failure 422 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/profile [patch]
func (h *CustomerV1Handler) PatchProfile(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	var request customerProfilePatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return
	}
	patch := identityDomain.CustomerProfilePatch{Gender: request.Gender, Metadata: request.Metadata}
	if request.DateOfBirth != nil {
		value, err := time.Parse("2006-01-02", strings.TrimSpace(*request.DateOfBirth))
		if err != nil {
			h.errors.Abort(c, apiresponse.ValidationFailed(identityDomain.ErrInvalidCustomerProfile, apiresponse.InvalidParam{Field: "date_of_birth", Code: "invalid_date"}))
			return
		}
		patch.DateOfBirth = &value
	}
	profile, err := h.service.PatchCustomerProfile(c.Request.Context(), userID, patch)
	if err != nil {
		h.abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, profileResponseV1(profile))
}

// ListAddresses godoc
// @Summary List current customer's addresses (v1)
// @Tags Customers v1
// @Produce json
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/addresses [get]
func (h *CustomerV1Handler) ListAddresses(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	addresses, err := h.service.ListCustomerAddresses(c.Request.Context(), userID)
	if err != nil {
		h.abort(c, err)
		return
	}
	response := make([]customerAddressResponse, 0, len(addresses))
	for _, address := range addresses {
		response = append(response, addressResponse(address))
	}
	apiresponse.Success(c, stdhttp.StatusOK, response)
}

// CreateAddress godoc
// @Summary Create a current customer's address (v1)
// @Tags Customers v1
// @Accept json
// @Produce json
// @Param payload body customerAddressRequest true "Address"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Failure 422 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/addresses [post]
func (h *CustomerV1Handler) CreateAddress(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	input, ok := h.addressInput(c)
	if !ok {
		return
	}
	address, err := h.service.CreateCustomerAddress(c.Request.Context(), userID, input)
	if err != nil {
		h.abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusCreated, addressResponse(*address))
}

// UpdateAddress godoc
// @Summary Update a current customer's address (v1)
// @Tags Customers v1
// @Accept json
// @Produce json
// @Param addressID path string true "Address UUID"
// @Param payload body customerAddressRequest true "Address"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 401 {object} apiresponse.ProblemDetails
// @Failure 404 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/addresses/{addressID} [put]
func (h *CustomerV1Handler) UpdateAddress(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	addressID, err := uuid.Parse(c.Param("addressID"))
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err, apiresponse.InvalidParam{Field: "addressID", Code: "invalid_uuid"}))
		return
	}
	input, ok := h.addressInput(c)
	if !ok {
		return
	}
	address, err := h.service.UpdateCustomerAddress(c.Request.Context(), userID, addressID, input)
	if err != nil {
		h.abort(c, err)
		return
	}
	apiresponse.Success(c, stdhttp.StatusOK, addressResponse(*address))
}

// DeleteAddress godoc
// @Summary Delete a current customer's address (v1)
// @Tags Customers v1
// @Produce json
// @Param addressID path string true "Address UUID"
// @Success 204
// @Failure 401 {object} apiresponse.ProblemDetails
// @Failure 404 {object} apiresponse.ProblemDetails
// @Router /api/v1/customers/me/addresses/{addressID} [delete]
func (h *CustomerV1Handler) DeleteAddress(c *gin.Context) {
	userID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		h.errors.Abort(c, apiresponse.Unauthenticated(nil))
		return
	}
	addressID, err := uuid.Parse(c.Param("addressID"))
	if err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err, apiresponse.InvalidParam{Field: "addressID", Code: "invalid_uuid"}))
		return
	}
	if err := h.service.DeleteCustomerAddress(c.Request.Context(), userID, addressID); err != nil {
		h.abort(c, err)
		return
	}
	apiresponse.NoContent(c)
}

func (h *CustomerV1Handler) addressInput(c *gin.Context) (identityDomain.AddressInput, bool) {
	var request customerAddressRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.errors.Abort(c, apiresponse.InvalidPayload(err))
		return identityDomain.AddressInput{}, false
	}
	return identityDomain.AddressInput{Title: request.Title, Country: request.Country, City: request.City, Line1: request.Line1, Line2: request.Line2, ZipCode: request.ZipCode, IsDefault: request.IsDefault}, true
}

func (h *CustomerV1Handler) abort(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityDomain.ErrAddressNotFound):
		h.errors.Abort(c, apiresponse.NotFound(err, "The requested address was not found."))
	case errors.Is(err, identityDomain.ErrInvalidCustomerAddress), errors.Is(err, identityDomain.ErrInvalidCustomerProfile):
		h.errors.Abort(c, apiresponse.ValidationFailed(err))
	default:
		h.errors.Abort(c, err)
	}
}

func profileResponseV1(profile *identityDomain.CustomerProfile) customerProfileResponse {
	result := customerProfileResponse{Gender: profile.Gender, Metadata: profile.Metadata, UpdatedAt: &profile.UpdatedAt}
	if profile.DateOfBirth != nil {
		value := profile.DateOfBirth.Format("2006-01-02")
		result.DateOfBirth = &value
	}
	return result
}
func addressResponse(address identityDomain.CustomerAddress) customerAddressResponse {
	return customerAddressResponse{ID: address.ID, Title: address.Title, Country: address.Country, City: address.City, Line1: address.Line1, Line2: address.Line2, ZipCode: address.ZipCode, IsDefault: address.IsDefault}
}
