package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	categoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware" // alias because of naming conflict with gin middleware
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

type categoryCacheEntry struct {
	categories []categoryDomain.Category
	fetchedAt  time.Time
}

type ProductHandler struct {
	service     domain.ProductService
	categorySvc categoryDomain.CategoryService
	redirect    redirectDomain.RedirectService
	l           logger.Logger

	// In-memory cache for category list (used for breadcrumbs)
	catCacheMu sync.RWMutex
	catCache   map[string]*categoryCacheEntry // key = lang
}

const categoryCacheTTL = 5 * time.Minute

func NewProductHandler(s domain.ProductService, catSvc categoryDomain.CategoryService, redirect redirectDomain.RedirectService, l logger.Logger) *ProductHandler {
	return &ProductHandler{
		service:     s,
		categorySvc: catSvc,
		redirect:    redirect,
		l:           l,
		catCache:    make(map[string]*categoryCacheEntry),
	}
}

// getCategoriesCached returns the category list from cache or fetches it from DB.
func (h *ProductHandler) getCategoriesCached(ctx context.Context, lang string) ([]categoryDomain.Category, error) {
	h.catCacheMu.RLock()
	if entry, ok := h.catCache[lang]; ok && time.Since(entry.fetchedAt) < categoryCacheTTL {
		cats := entry.categories
		h.catCacheMu.RUnlock()
		return cats, nil
	}
	h.catCacheMu.RUnlock()

	// Fetch from DB
	cats, err := h.categorySvc.GetList(ctx, lang, true)
	if err != nil {
		return nil, err
	}

	// Store in cache
	h.catCacheMu.Lock()
	h.catCache[lang] = &categoryCacheEntry{
		categories: cats,
		fetchedAt:  time.Now(),
	}
	h.catCacheMu.Unlock()

	return cats, nil
}

// resolveValueCode повертає дискретний код для фільтрації (av.value_code).
// Пріоритет: явно переданий код → slug зі value_string_uk → slug зі value_string_en.
// Без коду фільтр по характеристиці не спрацьовує.
func resolveValueCode(provided, uk, en string) string {
	if c := strings.TrimSpace(provided); c != "" {
		return c
	}
	src := strings.TrimSpace(uk)
	if src == "" {
		src = strings.TrimSpace(en)
	}
	if src == "" {
		return ""
	}
	code := slug.Make(src)
	// value_code — короткий дискретний ключ для фільтрації (колонка varchar(100)).
	// Якщо слаг виходить задовгим, значення є вільним текстом (напр. «склад»),
	// а не фасетом фільтра — код не проставляємо. Порожній код підтримується
	// штатно (enrichAttributeValueTranslations його пропускає).
	if len(code) > 100 {
		return ""
	}
	return code
}

// buildVariationName збирає локалізовану назву варіації з наданих uk/en.
// Порожні значення не зберігаємо (інакше getLocalized поверне "" замість fallback).
// nil = власної назви немає → у відповіді підставиться назва товару.
func buildVariationName(uk, en *string) domain.LocalizedMap {
	nm := domain.LocalizedMap{}
	if uk != nil {
		if v := strings.TrimSpace(*uk); v != "" {
			nm["uk"] = v
		}
	}
	if en != nil {
		if v := strings.TrimSpace(*en); v != "" {
			nm["en"] = v
		}
	}
	if len(nm) == 0 {
		return nil
	}
	return nm
}

// GetProducts godoc
// @Summary      Get products
// @Description  Get a paginated list of products with filters. Dynamic attributes: use attrs[code]=value1&attrs[code]=value2
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang     path     string  true   "Language code (uk, en)"
// @Param        page     query    int     false  "Page number"
// @Param        limit    query    int     false  "Items per page"
// @Param        sort_by  query    string  false  "Sort by (price, name, rating, created_at)"
// @Param        order    query    string  false  "Order (desc, asc)"
// @Param        category_id query string  false  "Filter by Category UUID"
// @Param        brand_id    query   []string  false  "Filter by Brand UUIDs"
// @Param        min_price   query   int       false  "Minimum price"
// @Param        max_price   query   int       false  "Maximum price"
// @Param        quantity_values query []string false "Quantity values (e.g. 36.00)"
// @Param        q        query    string  false  "Пошуковий запит (по назві або SKU)"
// @Param        attrs[code] query []string false "Динамічний фільтр характеристик. Замініть 'code' на код атрибута, напр.: attrs[color]=red"
// @Success      200         {object} pagination.PagedResponse{data=[]ProductBriefResponse}
// @Failure      400      {object} ErrorResponse
// @Failure      500      {object} ErrorResponse
// @Router       /api/{lang}/products [get]
func (h *ProductHandler) GetProducts(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)

	var req GetProductsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	categoryIDs := parseArrayParam(c.Request.URL.Query(), "category_id")
	categoryUUIDs := make([]uuid.UUID, 0, len(categoryIDs))
	for _, idStr := range categoryIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Category ID", Message: idStr})
			return
		}
		categoryUUIDs = append(categoryUUIDs, id)
	}

	brandIDs := parseArrayParam(c.Request.URL.Query(), "brand_id")
	brandUUIDs := make([]uuid.UUID, 0, len(brandIDs))
	for _, idStr := range brandIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Brand ID", Message: idStr})
			return
		}
		brandUUIDs = append(brandUUIDs, id)
	}

	quantityValues := parseArrayParam(c.Request.URL.Query(), "quantity_values")

	brandSlugs := parseArrayParam(c.Request.URL.Query(), "brand_slug")

	filter := domain.ProductFilter{
		CategoryID:     categoryUUIDs,
		BrandID:        brandUUIDs,
		BrandSlugs:     brandSlugs,
		MinPrice:       req.MinPrice,
		MaxPrice:       req.MaxPrice,
		QuantityValues: quantityValues,
		AttrValues:     make(map[string][]string),
		SearchQuery:    req.Q,
	}

	// Парсинг динамічних атрибутів виду `attrs[type]=value1&attrs[type]=value2` або `attrs[type]=value1,value2`
	for key, values := range c.Request.URL.Query() {
		if len(key) > 6 && key[:6] == "attrs[" && key[len(key)-1] == ']' {
			code := key[6 : len(key)-1]
			if code == "code" {
				// Swagger UI fallback: if key is literally attrs[code], parse value of type 'attrs[type]=capsules' or 'type=capsules'
				for _, val := range values {
					if strings.Contains(val, "=") {
						parts := strings.SplitN(val, "=", 2)
						innerKey := parts[0]
						innerVal := parts[1]
						if len(innerKey) > 6 && innerKey[:6] == "attrs[" && innerKey[len(innerKey)-1] == ']' {
							innerKey = innerKey[6 : len(innerKey)-1]
						}
						appendAttrValues(filter.AttrValues, innerKey, innerVal)
					}
				}
			} else {
				for _, val := range values {
					appendAttrValues(filter.AttrValues, code, val)
				}
			}
		}
	}

	variations, meta, err := h.service.GetList(c.Request.Context(), lang, filter, req.Params)
	if err != nil {
		h.l.Errorw("Failed to get variations list", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	resData := make([]ProductBriefResponse, len(variations))
	for i, v := range variations {
		resData[i] = mapVariationToBriefResponse(v, lang)
	}

	c.JSON(http.StatusOK, pagination.PagedResponse{
		Data:     resData,
		Metadata: meta,
	})
}

// GetRecommendedProducts godoc
// @Summary      Get recommended products
// @Description  Get a randomized list of up to 8 recommended products
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang     path     string  true   "Language code (uk, en)"
// @Success      200      {array}  ProductBriefResponse
// @Failure      500      {object} ErrorResponse
// @Router       /api/{lang}/products/recommended [get]
func (h *ProductHandler) GetRecommendedProducts(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	limit := 8

	variations, err := h.service.GetRecommended(c.Request.Context(), lang, limit)
	if err != nil {
		h.l.Errorw("Failed to get recommended products", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	resData := make([]ProductBriefResponse, len(variations))
	for i, v := range variations {
		resData[i] = mapVariationToBriefResponse(v, lang)
	}

	c.JSON(http.StatusOK, resData)
}

// QuickSearchProducts godoc
// @Summary      Quick search products
// @Description  Fast autocomplete search returning basic product info (requires min 3 chars)
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        q    query string true "Search query (min 3 chars)"
// @Param        limit query int false "Max results count (default 5)"
// @Success      200  {array} QuickSearchResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/products/search/quick [get]
func (h *ProductHandler) QuickSearchProducts(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	q := strings.TrimSpace(c.Query("q"))

	if utf8.RuneCountInString(q) < 3 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Search query must be at least 3 characters long"})
		return
	}

	limit := 5
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	products, err := h.service.QuickSearch(c.Request.Context(), q, lang, limit)
	if err != nil {
		h.l.Errorw("Failed to execute quick search", "err", err, "q", q)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	resData := make([]QuickSearchResponse, len(products))
	for i, p := range products {
		resData[i] = mapQuickSearchToResponse(p)
	}

	c.JSON(http.StatusOK, resData)
}

// GetProductByID godoc
// @Summary      Get product by UUID
// @Description  Get complete product details including variations and images
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        id   path string true "Product UUID"
// @Success      200  {object} ProductResponse
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/products/{id} [get]
func (h *ProductHandler) GetProductByID(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	idStr := c.Param("id")

	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid UUID format"})
		return
	}

	p, err := h.service.GetByID(c.Request.Context(), id, lang)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	var breadcrumbs []BreadcrumbResponse
	if h.categorySvc != nil && p.CategoryID != uuid.Nil {
		cats, err := h.getCategoriesCached(c.Request.Context(), lang)
		if err == nil {
			breadcrumbs = buildBreadcrumbs(cats, p.CategoryID, lang)
		}
	}

	c.JSON(http.StatusOK, mapProductToResponse(*p, lang, "", breadcrumbs))
}

// GetProductBySlug godoc
// @Summary      Get product by slug
// @Description  Get complete product details by SEO-friendly slug
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        slug path string true "Product slug"
// @Success      200  {object} ProductResponse
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/products/by-slug/{slug} [get]
func (h *ProductHandler) GetProductBySlug(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	slug := c.Param("slug")

	if slug == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Slug is required"})
		return
	}

	p, err := h.service.GetBySlug(c.Request.Context(), slug, lang)
	if err != nil {
		if errors.Is(err, domain.ErrSlugNotFound) {
			// Перевіряємо чи є цей слюг в історії
			if h.redirect != nil {
				entityType, newSlug, redirectErr := h.redirect.ResolveRedirect(c.Request.Context(), slug, lang)
				if redirectErr == nil && newSlug != "" {
					// Віддаємо 301 редірект
					// Формуємо JSON відповідь та встановлюємо заголовок Location для Next.js
					prefix := ""
					if lang != "uk" {
						prefix = "/" + lang
					}
					c.Header("Location", prefix+"/product/"+newSlug)
					c.JSON(http.StatusMovedPermanently, gin.H{
						"error":       "Moved Permanently",
						"redirect_to": newSlug,
						"entity_type": entityType,
					})
					return
				}
			}

			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	var breadcrumbs []BreadcrumbResponse
	if h.categorySvc != nil && p.CategoryID != uuid.Nil {
		cats, err := h.getCategoriesCached(c.Request.Context(), lang)
		if err == nil {
			breadcrumbs = buildBreadcrumbs(cats, p.CategoryID, lang)
		}
	}

	c.JSON(http.StatusOK, mapProductToResponse(*p, lang, slug, breadcrumbs))
}

// GetProductReviews godoc
// @Summary      Get product reviews
// @Description  Get a paginated list of approved reviews for a product
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        id   path string true "Product UUID"
// @Param        page query int false "Page number"
// @Param        limit query int false "Items per page"
// @Param        order query string false "Order by created date (desc, asc)"
// @Success      200  {object} pagination.PagedResponse{data=[]ProductReviewResponse}
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/products/{id}/reviews [get]
func (h *ProductHandler) GetProductReviews(c *gin.Context) {
	idStr := c.Param("id")

	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid UUID format"})
		return
	}

	var pgn pagination.Params
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	reviews, meta, err := h.service.GetReviews(c.Request.Context(), id, pgn)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, pagination.PagedResponse{
		Data:     mapReviewListToResponse(reviews),
		Metadata: meta,
	})
}

// CreateReview godoc
// @Summary      Create product review
// @Description  Submit a review for moderation
// @Tags         Products
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path string true "Product UUID"
// @Param        body body CreateReviewRequest true "Review body"
// @Success      201  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/products/{id}/reviews [post]
func (h *ProductHandler) CreateReview(c *gin.Context) {
	idStr := c.Param("id")

	prodID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid UUID format"})
		return
	}

	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized", Message: "User not authenticated"})
		return
	}

	var userID uuid.UUID
	switch v := userIDStr.(type) {
	case string:
		userID, _ = uuid.Parse(v)
	case uuid.UUID:
		userID = v
	}

	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized", Message: "Invalid User token data"})
		return
	}

	var req CreateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	status := "pending"
	if roleID, exists := c.Get("role_id"); exists {
		if r, ok := roleID.(int); ok && r == 2 { // 2 = RoleAdmin
			status = "approved"
		}
	}

	isReply := req.ParentID != nil
	rating := 0
	if !isReply {
		if req.Rating == nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "rating is required for a review"})
			return
		}
		rating = *req.Rating
	}

	review := &domain.ProductReview{
		ProductID: prodID,
		UserID:    userID,
		Rating:    rating,
		Comment:   req.Comment,
		ParentID:  req.ParentID,
		Status:    status,
	}

	if err := h.service.AddReview(c.Request.Context(), review); err != nil {
		if errors.Is(err, domain.ErrInvalidRating) || errors.Is(err, domain.ErrCommentTooLong) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	if status == "approved" {
		c.JSON(http.StatusCreated, gin.H{
			"message": "Review (reply) created and auto-approved successfully",
			"id":      review.ID,
		})
	} else {
		c.JSON(http.StatusCreated, gin.H{
			"message": "Review created successfully and is awaiting moderation",
			"id":      review.ID,
		})
	}
}

// ApproveReview godoc
// @Summary      Approve product review
// @Description  Approve a review and recalculate product rating (Admin only)
// @Tags         Admin
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path string true "Review UUID"
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/reviews/{id}/approve [patch]
func (h *ProductHandler) ApproveReview(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid UUID format"})
		return
	}

	if err := h.service.ApproveReview(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrReviewNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Review not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Review approved successfully"})
}

// RejectReview godoc
// @Summary      Reject product review
// @Description  Reject a review (and recalculate product rating if it was approved) (Admin only)
// @Tags         Admin
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path string true "Review UUID"
// @Param        body body RejectReviewRequest false "Rejection reason"
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/reviews/{id}/reject [patch]
func (h *ProductHandler) RejectReview(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid UUID format"})
		return
	}

	var req RejectReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid request body"})
		return
	}

	if err := h.service.RejectReview(c.Request.Context(), id, req.Reason); err != nil {
		if errors.Is(err, domain.ErrReviewNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Review not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Review rejected successfully"})
}

// ReopenReview returns a rejected review to the moderation queue (Admin only).
func (h *ProductHandler) ReopenReview(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid UUID format"})
		return
	}
	if err := h.service.ReopenReview(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrReviewNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Rejected review not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Review returned to moderation"})
}

// GetPendingReviews godoc
// @Summary      Get pending product reviews
// @Description  Get a paginated list of reviews that are pending approval (Admin only)
// @Tags         Admin
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        page  query int false "Page number"
// @Param        limit query int false "Items per page"
// @Param        order query string false "Order by created date (desc, asc)"
// @Success      200   {object} pagination.PagedResponse{data=[]ProductReviewResponse}
// @Failure      400   {object} ErrorResponse
// @Failure      401   {object} ErrorResponse
// @Failure      403   {object} ErrorResponse
// @Failure      500   {object} ErrorResponse
// @Router       /api/admin/reviews/pending [get]
func (h *ProductHandler) GetPendingReviews(c *gin.Context) {
	var pgn pagination.Params
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	reviews, meta, err := h.service.GetPendingReviews(c.Request.Context(), pgn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, pagination.PagedResponse{
		Data:     mapReviewListToResponse(reviews),
		Metadata: meta,
	})
}

// GetRejectedReviews returns the moderation archive (Admin only).
func (h *ProductHandler) GetRejectedReviews(c *gin.Context) {
	var pgn pagination.Params
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}
	reviews, meta, err := h.service.GetRejectedReviews(c.Request.Context(), pgn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, pagination.PagedResponse{Data: mapReviewListToResponse(reviews), Metadata: meta})
}

// GetProductFilters godoc
// @Summary      Get available filters
// @Description  Get dynamic filters (price, brands, attributes) available for products with counts
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang            path      string   true  "Language code"
// @Param        category_id     query     []string false "Filter by Category UUIDs"
// @Param        brand_id        query     []string false "Filter by Brand UUIDs"
// @Param        min_price       query     int      false "Minimum price"
// @Param        max_price       query     int      false "Maximum price"
// @Param        quantity_values query     []string false "Quantity values"
// @Param        attrs[code]     query     []string false "Динамічний фільтр характеристик. Замініть 'code' на код атрибута, напр.: attrs[color]=red"
// @Success      200  {object} FilterDiscoveryResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/products/filters [get]
func (h *ProductHandler) GetProductFilters(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)

	var req GetProductFiltersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	categoryIDs := parseArrayParam(c.Request.URL.Query(), "category_id")
	categoryUUIDs := make([]uuid.UUID, 0, len(categoryIDs))
	for _, idStr := range categoryIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Category ID", Message: idStr})
			return
		}
		categoryUUIDs = append(categoryUUIDs, id)
	}

	brandIDs := parseArrayParam(c.Request.URL.Query(), "brand_id")
	brandUUIDs := make([]uuid.UUID, 0, len(brandIDs))
	for _, idStr := range brandIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Brand ID", Message: idStr})
			return
		}
		brandUUIDs = append(brandUUIDs, id)
	}

	quantityValues := parseArrayParam(c.Request.URL.Query(), "quantity_values")

	brandSlugs := parseArrayParam(c.Request.URL.Query(), "brand_slug")

	filter := domain.ProductFilter{
		CategoryID:     categoryUUIDs,
		BrandID:        brandUUIDs,
		BrandSlugs:     brandSlugs,
		MinPrice:       req.MinPrice,
		MaxPrice:       req.MaxPrice,
		QuantityValues: quantityValues,
		AttrValues:     make(map[string][]string),
	}

	// Parsing dynamic attributes attrs[code]=value or attrs[code]=value1,value2
	for key, values := range c.Request.URL.Query() {
		if len(key) > 6 && key[:6] == "attrs[" && key[len(key)-1] == ']' {
			code := key[6 : len(key)-1]
			if code == "code" {
				// Swagger UI fallback: if key is literally attrs[code], parse value of type 'attrs[type]=capsules' or 'type=capsules'
				for _, val := range values {
					if strings.Contains(val, "=") {
						parts := strings.SplitN(val, "=", 2)
						innerKey := parts[0]
						innerVal := parts[1]
						if len(innerKey) > 6 && innerKey[:6] == "attrs[" && innerKey[len(innerKey)-1] == ']' {
							innerKey = innerKey[6 : len(innerKey)-1]
						}
						appendAttrValues(filter.AttrValues, innerKey, innerVal)
					}
				}
			} else {
				for _, val := range values {
					appendAttrValues(filter.AttrValues, code, val)
				}
			}
		}
	}

	fd, err := h.service.GetFilters(c.Request.Context(), filter, lang)
	if err != nil {
		h.l.Errorw("Failed to get product filters", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapFilterDiscoveryToResponse(*fd, lang))
}

// GetUnits godoc
// @Summary      Get units of measure
// @Description  Get all units of measure (capacity/weight) localized — for the admin product form (місткість).
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Success      200  {array}  UnitResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/{lang}/units [get]
func (h *ProductHandler) GetUnits(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)

	units, err := h.service.GetUnits(c.Request.Context())
	if err != nil {
		h.l.Errorw("Failed to get units", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	res := make([]UnitResponse, 0, len(units))
	for _, u := range units {
		res = append(res, UnitResponse{
			ID:        u.ID,
			Name:      getLocalized(u.Name, lang),
			ShortName: getLocalized(u.ShortName, lang),
		})
	}

	c.JSON(http.StatusOK, res)
}

// CreateProduct godoc
// @Summary      Create product
// @Description  Create a new product with translations, variations and attributes. Uses multipart form where 'payload' contains JSON data.
// @Tags         Admin Products
// @Accept       multipart/form-data
// @Produce      json
// @Param        payload  formData  string                true  "CreateProductRequest JSON (includes images_meta mapping)"
// @Param        images   formData  file                  false "Image files"
// @Success      201      {object}  map[string]interface{}
// @Success      202      {object}  CreateProductRequest  "Schema reference for payload"
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/products [post]
// CreateProduct створює новий товар (адмін-метод)
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(10 << 20); err != nil { // 10MB limit
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid form data"})
		return
	}

	payloadJSON := c.Request.FormValue("payload")
	if payloadJSON == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Payload is missing"})
		return
	}

	var req CreateProductRequest
	if err := json.Unmarshal([]byte(payloadJSON), &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON payload", Message: err.Error()})
		return
	}

	images := parseImageUploads(c, req.ImagesMeta)

	product := domain.Product{
		ID:         uuid.New(),
		BrandID:    req.BrandID,
		CategoryID: req.CategoryID,
		IsActive:   *req.IsActive,
	}

	if req.IsRecommended != nil {
		product.IsRecommended = *req.IsRecommended
	}

	// Бейджі товару
	for _, bid := range req.BadgeIDs {
		product.Badges = append(product.Badges, domain.ProductBadge{
			ProductID: &product.ID,
			BadgeID:   bid,
		})
	}

	product.Translations = []domain.ProductTranslation{
		{LanguageCode: "uk", Name: req.NameUk, Description: req.DescriptionUk, UsageInstructions: req.UsageUk, MetaTitle: req.MetaTitleUk, MetaDescription: req.MetaDescriptionUk, MetaKeywords: req.MetaKeywordsUk},
		{LanguageCode: "en", Name: req.NameEn, Description: req.DescriptionEn, UsageInstructions: req.UsageEn, MetaTitle: req.MetaTitleEn, MetaDescription: req.MetaDescriptionEn, MetaKeywords: req.MetaKeywordsEn},
	}

	for _, attr := range req.AttributeValues {
		av := domain.AttributeValue{
			ID:          uuid.New(),
			AttributeID: attr.AttributeID,
			ValueCode:   resolveValueCode(attr.ValueCode, attr.ValueStringUk, attr.ValueStringEn),
			UnitID:      attr.UnitID,
		}
		if attr.ValueStringUk != "" || attr.ValueStringEn != "" {
			av.ValueString = domain.LocalizedMap{"uk": attr.ValueStringUk, "en": attr.ValueStringEn}
		}
		if attr.ValueNumeric != nil {
			av.ValueNumeric = attr.ValueNumeric
		}
		product.AttributeValues = append(product.AttributeValues, av)
	}

	for _, v := range req.Variations {
		newVar := domain.ProductVariation{
			UnitID:   v.UnitID,
			OldPrice: v.OldPrice,
		}
		if v.SKU != nil {
			newVar.SKU = *v.SKU
		}
		if v.Barcode != nil {
			newVar.Barcode = *v.Barcode
		}
		if v.Price != nil {
			newVar.Price = *v.Price
		}
		if v.QuantityValue != nil {
			newVar.QuantityValue = *v.QuantityValue
		}
		if v.Weight != nil {
			newVar.Weight = *v.Weight
		} else {
			newVar.Weight = 0.5 // Default weight
		}
		if v.ID != nil && *v.ID != uuid.Nil {
			newVar.ID = *v.ID
		} else {
			newVar.ID = uuid.New()
		}
		if nm := buildVariationName(v.NameUk, v.NameEn); nm != nil {
			newVar.Name = nm
		}
		if v.IsActive != nil {
			newVar.IsActive = *v.IsActive
		} else {
			newVar.IsActive = true
		}
		for _, attr := range v.AttributeValues {
			av := domain.AttributeValue{
				ID:          uuid.New(),
				AttributeID: attr.AttributeID,
				ValueCode:   resolveValueCode(attr.ValueCode, attr.ValueStringUk, attr.ValueStringEn),
				UnitID:      attr.UnitID,
			}
			if attr.ValueStringUk != "" || attr.ValueStringEn != "" {
				av.ValueString = domain.LocalizedMap{"uk": attr.ValueStringUk, "en": attr.ValueStringEn}
			}
			if attr.ValueNumeric != nil {
				av.ValueNumeric = attr.ValueNumeric
			}
			newVar.AttributeValues = append(newVar.AttributeValues, av)
		}

		// Бейджі для варіації
		for _, bid := range v.BadgeIDs {
			newVar.Badges = append(newVar.Badges, domain.ProductBadge{
				VariationID: &newVar.ID,
				BadgeID:     bid,
			})
		}

		product.Variations = append(product.Variations, newVar)
	}

	product.IsBundle = req.IsBundle
	if req.IsBundle {
		if req.PriceStrategy != "" {
			product.PriceStrategy = req.PriceStrategy
		} else {
			product.PriceStrategy = domain.PriceStrategyManual
		}
		for _, bi := range req.BundleItems {
			product.BundleItems = append(product.BundleItems, domain.ProductBundleItem{
				VariationID: bi.VariationID,
				Quantity:    bi.Quantity,
			})
		}
	}

	if err := h.service.CreateProduct(c.Request.Context(), &product, images); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create product", Message: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": product.ID})
}

// UpdateProduct godoc
// @Summary      Update product
// @Description  Update existing product fields, translations, variations, and images. Uses multipart form.
// @Tags         Admin Products
// @Accept       multipart/form-data
// @Produce      json
// @Param        id       path      string                true  "Product ID"
// @Param        payload  formData  string                true  "UpdateProductRequest JSON (includes images_meta mapping)"
// @Param        images   formData  file                  false "New image files"
// @Success      200      {object}  map[string]interface{}
// @Success      202      {object}  UpdateProductRequest  "Schema reference for payload"
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/products/{id} [patch]
// UpdateProduct оновлює існуючий товар (адмін-метод)
func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid ID"})
		return
	}

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid form data"})
		return
	}

	payloadJSON := c.Request.FormValue("payload")
	if payloadJSON == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Payload is missing"})
		return
	}

	var req UpdateProductRequest
	if err := json.Unmarshal([]byte(payloadJSON), &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON payload", Message: err.Error()})
		return
	}

	images := parseImageUploads(c, req.ImagesMeta)

	product := domain.Product{
		ID: id,
	}
	if req.BrandID != nil {
		product.BrandID = *req.BrandID
	}
	if req.CategoryID != nil {
		product.CategoryID = *req.CategoryID
	}
	if req.IsActive != nil {
		product.IsActive = *req.IsActive
	}
	if req.IsRecommended != nil {
		product.IsRecommended = *req.IsRecommended
	}

	if req.IsBundle != nil {
		product.IsBundle = *req.IsBundle
	}
	if req.PriceStrategy != nil {
		product.PriceStrategy = *req.PriceStrategy
	}
	if req.BundleItems != nil {
		for _, bi := range *req.BundleItems {
			product.BundleItems = append(product.BundleItems, domain.ProductBundleItem{
				VariationID: bi.VariationID,
				Quantity:    bi.Quantity,
			})
		}
	}

	// Бейджі товару
	if req.BadgeIDs != nil {
		for _, bid := range *req.BadgeIDs {
			product.Badges = append(product.Badges, domain.ProductBadge{
				ProductID: &product.ID,
				BadgeID:   bid,
			})
		}
	}

	// Оновлюємо переклади лише якщо прийшло хоча б одне поле
	if req.NameUk != nil || req.DescriptionUk != nil || req.UsageUk != nil || req.MetaTitleUk != nil || req.MetaDescriptionUk != nil || req.MetaKeywordsUk != nil ||
		req.NameEn != nil || req.DescriptionEn != nil || req.UsageEn != nil || req.MetaTitleEn != nil || req.MetaDescriptionEn != nil || req.MetaKeywordsEn != nil {

		ukT := domain.ProductTranslation{LanguageCode: "uk"}
		enT := domain.ProductTranslation{LanguageCode: "en"}
		hasUk, hasEn := false, false

		if req.NameUk != nil {
			ukT.Name = *req.NameUk
			hasUk = true
		}
		if req.DescriptionUk != nil {
			ukT.Description = *req.DescriptionUk
			hasUk = true
		}
		if req.UsageUk != nil {
			ukT.UsageInstructions = *req.UsageUk
			hasUk = true
		}
		if req.MetaTitleUk != nil {
			ukT.MetaTitle = *req.MetaTitleUk
			hasUk = true
		}
		if req.MetaDescriptionUk != nil {
			ukT.MetaDescription = *req.MetaDescriptionUk
			hasUk = true
		}
		if req.MetaKeywordsUk != nil {
			ukT.MetaKeywords = *req.MetaKeywordsUk
			hasUk = true
		}

		if req.NameEn != nil {
			enT.Name = *req.NameEn
			hasEn = true
		}
		if req.DescriptionEn != nil {
			enT.Description = *req.DescriptionEn
			hasEn = true
		}
		if req.UsageEn != nil {
			enT.UsageInstructions = *req.UsageEn
			hasEn = true
		}
		if req.MetaTitleEn != nil {
			enT.MetaTitle = *req.MetaTitleEn
			hasEn = true
		}
		if req.MetaDescriptionEn != nil {
			enT.MetaDescription = *req.MetaDescriptionEn
			hasEn = true
		}
		if req.MetaKeywordsEn != nil {
			enT.MetaKeywords = *req.MetaKeywordsEn
			hasEn = true
		}

		if hasUk {
			product.Translations = append(product.Translations, ukT)
		}
		if hasEn {
			product.Translations = append(product.Translations, enT)
		}
	}

	// Атрибути: Full Sync якщо поле передано
	attributeValuesProvided := req.AttributeValues != nil
	if attributeValuesProvided {
		for _, attr := range *req.AttributeValues {
			av := domain.AttributeValue{
				ID:          uuid.New(),
				AttributeID: attr.AttributeID,
				ValueCode:   resolveValueCode(attr.ValueCode, attr.ValueStringUk, attr.ValueStringEn),
				UnitID:      attr.UnitID,
			}
			if attr.ValueStringUk != "" || attr.ValueStringEn != "" {
				av.ValueString = domain.LocalizedMap{"uk": attr.ValueStringUk, "en": attr.ValueStringEn}
			}
			if attr.ValueNumeric != nil {
				av.ValueNumeric = attr.ValueNumeric
			}
			product.AttributeValues = append(product.AttributeValues, av)
		}
	}

	// Варіації: Partial Update якщо поле передано
	var variationsUpdate *[]domain.ProductVariationUpdate
	if req.Variations != nil {
		updates := make([]domain.ProductVariationUpdate, 0, len(*req.Variations))
		for _, v := range *req.Variations {
			newVar := domain.ProductVariationUpdate{
				ID:            v.ID,
				SKU:           v.SKU,
				Barcode:       v.Barcode,
				Price:         v.Price,
				OldPrice:      v.OldPrice,
				QuantityValue: v.QuantityValue,
				Weight:        v.Weight,
				UnitID:        v.UnitID,
				IsActive:      v.IsActive,
			}
			// name_uk/name_en присутні (форма шле завжди) → повна заміна назви варіації.
			// Порожні обидва → порожня мапа = очистити (fallback на назву товару).
			if v.NameUk != nil || v.NameEn != nil {
				nm := buildVariationName(v.NameUk, v.NameEn)
				if nm == nil {
					nm = domain.LocalizedMap{}
				}
				newVar.Name = &nm
			}
			if v.AttributeValues != nil {
				attrs := make([]domain.AttributeValue, 0, len(v.AttributeValues))
				for _, attr := range v.AttributeValues {
					av := domain.AttributeValue{
						ID:          uuid.New(),
						AttributeID: attr.AttributeID,
						ValueCode:   resolveValueCode(attr.ValueCode, attr.ValueStringUk, attr.ValueStringEn),
						UnitID:      attr.UnitID,
					}
					if attr.ValueStringUk != "" || attr.ValueStringEn != "" {
						av.ValueString = domain.LocalizedMap{"uk": attr.ValueStringUk, "en": attr.ValueStringEn}
					}
					if attr.ValueNumeric != nil {
						av.ValueNumeric = attr.ValueNumeric
					}
					attrs = append(attrs, av)
				}
				newVar.AttributeValues = &attrs
			}

			// Бейджі варіації
			if v.BadgeIDs != nil {
				newVar.BadgeIDs = &v.BadgeIDs
			}

			updates = append(updates, newVar)
		}
		variationsUpdate = &updates
	}

	if err := h.service.UpdateProduct(c.Request.Context(), &product, variationsUpdate, images, req.ImagesToDelete, req.IsActive, req.IsBundle, attributeValuesProvided); err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update product", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// DeleteProduct godoc
// @Summary      Delete product
// @Description  Soft delete product and its related variations.
// @Tags         Admin Products
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Product ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/admin/products/{id} [delete]
// DeleteProduct видаляє товар (адмін-метод)
func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid ID"})
		return
	}

	if err := h.service.DeleteProduct(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete product", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product successfully deleted"})
}

// --- Image Management Handlers ---

// UploadProductImages godoc
// @Summary      Upload product images
// @Description  Upload new images to a product without sending the full product payload (Admin only)
// @Tags         Admin Product Images
// @Accept       multipart/form-data
// @Produce      json
// @Security     bearerAuth
// @Param        id   path      string  true  "Product UUID"
// @Param        payload formData string false "JSON array of ImageMeta to map file keys to metadata"
// @Param        images formData file true "Image files (keys should match those in images_meta)"
// @Success      201  {array}   ProductImageResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/admin/products/{id}/images [post]
func (h *ProductHandler) UploadProductImages(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Product ID"})
		return
	}

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid form data"})
		return
	}

	payloadJSON := c.Request.FormValue("payload")
	var meta []ImageMeta
	if payloadJSON != "" {
		if err := json.Unmarshal([]byte(payloadJSON), &meta); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON meta payload", Message: err.Error()})
			return
		}
	}

	images := parseImageUploads(c, meta)
	if len(images) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "No images provided"})
		return
	}

	result, err := h.service.UploadProductImages(c.Request.Context(), productID, images)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to upload images", Message: err.Error()})
		return
	}

	res := make([]ProductImageResponse, len(result))
	for i, img := range result {
		res[i] = ProductImageResponse{
			ID:        img.ID,
			ImageURL:  img.ImageURL,
			IsPrimary: img.IsPrimary,
			IsHover:   img.IsHover,
			SortOrder: img.SortOrder,
		}
	}

	c.JSON(http.StatusCreated, res)
}

// DeleteProductImage godoc
// @Summary      Delete product image
// @Description  Delete a specific image from a product (Admin only)
// @Tags         Admin Product Images
// @Security     bearerAuth
// @Param        id     path string true "Product UUID"
// @Param        img_id path string true "Image UUID"
// @Success      200    {object} map[string]string
// @Failure      400    {object} ErrorResponse
// @Failure      404    {object} ErrorResponse
// @Failure      500    {object} ErrorResponse
// @Router       /api/admin/products/{id}/images/{img_id} [delete]
func (h *ProductHandler) DeleteProductImage(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Product ID"})
		return
	}

	imageID, err := uuid.Parse(c.Param("img_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Image ID"})
		return
	}

	if err := h.service.DeleteProductImage(c.Request.Context(), productID, imageID); err != nil {
		if errors.Is(err, domain.ErrImageNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Image not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete image", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image successfully deleted"})
}

// UpdateProductImage godoc
// @Summary      Update product image
// @Description  Update image role (primary/hover), sort order, or variation assignment (Admin only)
// @Tags         Admin Product Images
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id     path string true "Product UUID"
// @Param        img_id path string true "Image UUID"
// @Param        body   body UpdateImageRequest true "Image update body"
// @Success      200    {object} map[string]string
// @Failure      400    {object} ErrorResponse
// @Failure      404    {object} ErrorResponse
// @Failure      500    {object} ErrorResponse
// @Router       /api/admin/products/{id}/images/{img_id} [patch]
func (h *ProductHandler) UpdateProductImage(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Product ID"})
		return
	}

	imageID, err := uuid.Parse(c.Param("img_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Image ID"})
		return
	}

	var req UpdateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
		return
	}

	if err := h.service.UpdateProductImage(c.Request.Context(), productID, imageID, req.IsPrimary, req.IsHover, req.SortOrder, req.VariationIDSet, req.VariationID, req.AltTextUk, req.AltTextEn); err != nil {
		if errors.Is(err, domain.ErrImageNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Image not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update image", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image updated successfully"})
}

// ReorderProductImages godoc
// @Summary      Reorder product images
// @Description  Update sort_order for multiple images at once (Admin only)
// @Tags         Admin Product Images
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path string true "Product UUID"
// @Param        body body ReorderImagesRequest true "Image IDs in desired order"
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/products/{id}/images/reorder [patch]
func (h *ProductHandler) ReorderProductImages(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid Product ID"})
		return
	}

	var req ReorderImagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
		return
	}

	if err := h.service.ReorderProductImages(c.Request.Context(), productID, req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to reorder images", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Images reordered successfully"})
}

// parseImageUploads витягує файли зображень з multipart form та застосовує метадані
func parseImageUploads(c *gin.Context, meta []ImageMeta) []domain.ImageUpload {
	var images []domain.ImageUpload
	form := c.Request.MultipartForm
	if form == nil || form.File == nil {
		return images
	}

	// Створюємо карту для швидкого пошуку метаданих за ключем
	metaMap := make(map[string]ImageMeta)
	for _, m := range meta {
		metaMap[m.Key] = m
	}

	for key, headers := range form.File {
		for _, header := range headers {
			file, err := header.Open()
			if err != nil {
				continue
			}
			defer file.Close()

			filename := header.Filename
			if header.Header.Get("Content-Type") == "image/svg+xml" && !strings.HasSuffix(strings.ToLower(filename), ".svg") {
				filename += ".svg"
			}

			imgInfo := domain.ImageUpload{
				Filename: filename,
				Content:  file,
			}

			// Якщо для цього ключа є метадані - використовуємо їх
			if m, ok := metaMap[key]; ok {
				imgInfo.IsPrimary = m.IsPrimary
				imgInfo.IsHover = m.IsHover
				imgInfo.SortOrder = m.SortOrder
				imgInfo.VariationID = m.VariationID
				imgInfo.AltText = domain.LocalizedMap{"uk": m.AltTextUk, "en": m.AltTextEn}
			} else {
				// Fallback logic for backward compatibility or simple uploads
				if strings.Contains(key, "primary") {
					imgInfo.IsPrimary = true
				}
				if strings.Contains(key, "hover") {
					imgInfo.IsHover = true
				}
				if imgInfo.SortOrder == 0 {
					imgInfo.SortOrder = 10
				}
			}

			images = append(images, imgInfo)
		}
	}

	return images
}

func appendAttrValues(attrMap map[string][]string, key string, val string) {
	if strings.Contains(val, ",") {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				attrMap[key] = append(attrMap[key], p)
			}
		}
	} else {
		val = strings.TrimSpace(val)
		if val != "" {
			attrMap[key] = append(attrMap[key], val)
		}
	}
}

func parseArrayParam(query url.Values, key string) []string {
	var result []string

	// 1. Direct repetition or comma separated (e.g. brand_id=val1&brand_id=val2 or brand_id=val1,val2)
	if vals, ok := query[key]; ok {
		for _, v := range vals {
			if v != "" {
				if strings.Contains(v, ",") {
					parts := strings.Split(v, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							result = append(result, p)
						}
					}
				} else {
					result = append(result, v)
				}
			}
		}
	}

	// 2. Bracket notation (e.g. brand_id[]=val1&brand_id[]=val2 or brand_id[]=val1,val2)
	bracketKey := key + "[]"
	if vals, ok := query[bracketKey]; ok {
		for _, v := range vals {
			if v != "" {
				if strings.Contains(v, ",") {
					parts := strings.Split(v, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							result = append(result, p)
						}
					}
				} else {
					result = append(result, v)
				}
			}
		}
	}

	// 3. Indexed notation (e.g. brand_id[0]=val1&brand_id[1]=val2)
	for k, vals := range query {
		if strings.HasPrefix(k, key+"[") && strings.HasSuffix(k, "]") {
			for _, v := range vals {
				if v != "" {
					result = append(result, v)
				}
			}
		}
	}

	return result
}

// UploadMedia
// @Summary      Upload media file
// @Description  Uploads a general media file (e.g., category icon) to Cloudinary and returns the public URL.
// @Tags         Admin Products
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        folder query string false "Folder name in Cloudinary (default: 'general')"
// @Param        file   formData file true "File to upload"
// @Success      200    {object} map[string]string "url of uploaded file"
// @Failure      400    {object} ErrorResponse
// @Failure      500    {object} ErrorResponse
// @Router       /api/admin/media/upload [post]
func (h *ProductHandler) UploadMedia(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "file is required", Message: "file is required"})
		return
	}

	// Валідація розміру файлу (макс. 2 MB)
	const maxFileSize = 2 * 1024 * 1024 // 2 MB
	if fileHeader.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "file size exceeds the limit of 2 MB",
		})
		return
	}

	// Валідація типу файлу (тільки зображення)
	contentType := fileHeader.Header.Get("Content-Type")
	allowedTypes := map[string]bool{
		"image/jpeg":    true,
		"image/png":     true,
		"image/gif":     true,
		"image/webp":    true,
		"image/svg+xml": true,
	}
	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "only image files (jpeg, png, gif, webp, svg) are allowed",
		})
		return
	}

	folder := c.Query("folder")
	if folder == "" {
		folder = "general"
	}

	filename := fileHeader.Filename
	if contentType == "image/svg+xml" && !strings.HasSuffix(strings.ToLower(filename), ".svg") {
		filename += ".svg"
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to open file", Message: "failed to open file"})
		return
	}
	defer file.Close()

	url, err := h.service.UploadMedia(c.Request.Context(), file, folder, filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}
