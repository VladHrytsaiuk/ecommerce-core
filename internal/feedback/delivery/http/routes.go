package http

import (
	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func RegisterFeedbackRoutes(
	apiGrp *gin.RouterGroup,
	adminGrp *gin.RouterGroup,
	feedbackLimiter gin.HandlerFunc,
	s domain.FeedbackService,
	l logger.Logger,
) {
	h := NewFeedbackHandler(s, l)

	// Публічний ендпоінт створення відгуку з лімітером
	apiGrp.POST("/feedback", feedbackLimiter, h.CreateFeedback)

	// Адмінський ендпоінт для отримання списку
	adminGrp.GET("/feedbacks", h.GetFeedbacks)
}
