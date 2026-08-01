package http

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/domain"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

type FeedbackHandler struct {
	service domain.FeedbackService
	l       logger.Logger
}

func NewFeedbackHandler(service domain.FeedbackService, l logger.Logger) *FeedbackHandler {
	return &FeedbackHandler{
		service: service,
		l:       l,
	}
}

// CreateFeedback godoc
// @Summary      Submit feedback
// @Description  Allows users to submit a feedback form with a type (product_improvement or bug), email, content, and an optional media attachment.
// @Tags         Feedback
// @Accept       multipart/form-data
// @Produce      json
// @Param        type     formData string true  "Feedback type (product_improvement, bug)"
// @Param        email    formData string true  "User email address"
// @Param        content  formData string true  "Feedback text"
// @Param        file     formData file   false "Optional media file (image/pdf, max 5MB)"
// @Success      201      {object} CreateFeedbackResponse
// @Failure      400      {object} ErrorResponse
// @Failure      500      {object} ErrorResponse
// @Router       /api/feedback [post]
func (h *FeedbackHandler) CreateFeedback(c *gin.Context) {
	feedbackType := strings.TrimSpace(c.PostForm("type"))
	email := strings.TrimSpace(c.PostForm("email"))
	content := strings.TrimSpace(c.PostForm("content"))

	if feedbackType == "" || email == "" || content == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "fields 'type', 'email', and 'content' are required",
		})
		return
	}

	// Валідація пошти (простий формат)
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "invalid email format",
		})
		return
	}

	var file io.ReadSeeker
	var filename string

	fileHeader, err := c.FormFile("file")
	if err == nil && fileHeader != nil {
		// 1. Валідація розміру (макс. 5 MB)
		const maxFileSize = 5 * 1024 * 1024
		if fileHeader.Size > maxFileSize {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "file size exceeds the limit of 5 MB",
			})
			return
		}

		// 2. Валідація MIME-типу (MIME Sniffing)
		openedFile, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "Internal Server Error",
				Message: "failed to open uploaded file",
			})
			return
		}
		defer openedFile.Close()

		// Читаємо перші 512 байт для визначення типу
		buffer := make([]byte, 512)
		n, err := openedFile.Read(buffer)
		if err != nil && err != io.EOF {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "Internal Server Error",
				Message: "failed to read file content",
			})
			return
		}

		// Повертаємо вказівник на початок
		if _, err := openedFile.Seek(0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "Internal Server Error",
				Message: "failed to process file stream",
			})
			return
		}

		detectedMimeType := http.DetectContentType(buffer[:n])
		allowedTypes := map[string]bool{
			"image/jpeg":      true,
			"image/png":       true,
			"image/webp":      true,
			"application/pdf": true,
		}

		if !allowedTypes[detectedMimeType] {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: "only image files (jpeg, png, webp) and pdf are allowed",
			})
			return
		}

		file = openedFile
		filename = fileHeader.Filename
	}

	result, err := h.service.Create(c.Request.Context(), feedbackType, email, content, file, filename)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidFeedbackType) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Bad Request",
				Message: err.Error(),
			})
			return
		}
		h.l.Errorw("failed to submit feedback", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "failed to save feedback",
		})
		return
	}

	c.JSON(http.StatusCreated, CreateFeedbackResponse{
		ID:        result.ID.String(),
		Type:      result.Type,
		Email:     result.Email,
		Content:   result.Content,
		MediaURL:  result.MediaURL,
		CreatedAt: result.CreatedAt,
	})
}

// GetFeedbacks godoc
// @Summary      Get list of feedbacks
// @Description  Returns a paginated list of feedbacks (Admin only)
// @Tags         Admin Feedback
// @Produce      json
// @Security     bearerAuth
// @Param        page      query int false "Page number"
// @Param        per_page  query int false "Items per page"
// @Success      200       {object} map[string]interface{}
// @Failure      401       {object} ErrorResponse
// @Failure      403       {object} ErrorResponse
// @Failure      500       {object} ErrorResponse
// @Router       /api/admin/feedbacks [get]
func (h *FeedbackHandler) GetFeedbacks(c *gin.Context) {
	pgn := pagination.Params{}
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	feedbacks, meta, err := h.service.GetList(c.Request.Context(), pgn)
	if err != nil {
		h.l.Errorw("failed to retrieve feedbacks", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "failed to retrieve feedbacks",
		})
		return
	}

	response := make([]FeedbackResponse, len(feedbacks))
	for i, f := range feedbacks {
		response[i] = FeedbackResponse{
			ID:        f.ID.String(),
			Type:      f.Type,
			Email:     f.Email,
			Content:   f.Content,
			MediaURL:  f.MediaURL,
			CreatedAt: f.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": response,
		"meta": meta,
	})
}
