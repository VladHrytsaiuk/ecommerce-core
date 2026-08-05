package http

import (
	"errors"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type Handler struct{ service identityDomain.Service }

func NewHandler(service identityDomain.Service) *Handler { return &Handler{service: service} }

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	Role        string    `json:"role"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (h *Handler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid login request"})
		return
	}
	result, err := h.service.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		if errors.Is(err, identityDomain.ErrInvalidCredentials) {
			c.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "login unavailable"})
		return
	}
	c.JSON(stdhttp.StatusOK, loginResponse{AccessToken: result.AccessToken, TokenType: "Bearer", Role: result.Role, ExpiresAt: result.ExpiresAt})
}

func RegisterRoutes(api *gin.RouterGroup, service identityDomain.Service, sensitiveLimit gin.HandlerFunc) {
	if service == nil {
		return
	}
	group := api.Group("/auth")
	if sensitiveLimit != nil {
		group.Use(sensitiveLimit)
	}
	group.POST("/login", NewHandler(service).Login)
}
