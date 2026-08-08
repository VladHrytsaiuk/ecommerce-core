// Package http translates public identity HTTP contracts to provider-neutral
// application services. It does not construct providers or repositories.
package http

import (
	"errors"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

const maxProfileBodyBytes = 1 << 20

type AuthHandler struct {
	service          identityDomain.AuthService
	oauthRedirectURI string
}

func NewAuthHandler(service identityDomain.AuthService, oauthRedirectURI string) *AuthHandler {
	return &AuthHandler{service: service, oauthRedirectURI: oauthRedirectURI}
}

type ProfileHandler struct{ service identityDomain.ProfileService }

func NewProfileHandler(service identityDomain.ProfileService) *ProfileHandler {
	return &ProfileHandler{service: service}
}

type registerRequest struct {
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Password string  `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"`
	Email    string `json:"email"` // Backward-compatible client alias.
	Password string `json:"password" binding:"required"`
}

type sessionResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	UserID      string    `json:"user_id"`
	Role        string    `json:"role"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid registration request"})
		return
	}
	session, err := h.service.RegisterPassword(c.Request.Context(), identityDomain.RegisterPasswordCommand{Email: request.Email, Phone: request.Phone, Password: request.Password})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusCreated, session)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid login request"})
		return
	}
	login := strings.TrimSpace(request.Login)
	if login == "" {
		login = strings.TrimSpace(request.Email)
	}
	session, err := h.service.LoginPassword(c.Request.Context(), identityDomain.PasswordLoginCommand{Login: login, Password: request.Password})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusOK, session)
}

func (h *AuthHandler) BeginOAuth(c *gin.Context) {
	if h.oauthRedirectURI == "" {
		c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "OAuth provider is not enabled"})
		return
	}
	authorization, err := h.service.BeginOAuth(c.Request.Context(), identityDomain.BeginOAuthCommand{Provider: c.Param("provider"), RedirectURI: h.oauthRedirectURI})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	c.Redirect(stdhttp.StatusFound, authorization.RedirectURL)
}

func (h *AuthHandler) CompleteOAuth(c *gin.Context) {
	if h.oauthRedirectURI == "" {
		c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "OAuth provider is not enabled"})
		return
	}
	session, err := h.service.CompleteOAuth(c.Request.Context(), identityDomain.CompleteOAuthCommand{Provider: c.Param("provider"), RedirectURI: h.oauthRedirectURI, Code: c.Query("code"), State: c.Query("state")})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusOK, session)
}

func (h *ProfileHandler) Get(c *gin.Context) {
	userID, ok := middleware.AuthenticatedUserID(c)
	if !ok {
		c.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	profile, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, identityDomain.ErrProfileNotFound) {
			c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "profile unavailable"})
		return
	}
	c.JSON(stdhttp.StatusOK, profileResponse(profile))
}

func (h *ProfileHandler) Update(c *gin.Context) {
	userID, ok := middleware.AuthenticatedUserID(c)
	if !ok {
		c.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxProfileBodyBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxProfileBodyBytes {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid profile document"})
		return
	}
	profile, err := h.service.UpdateProfile(c.Request.Context(), userID, body)
	if err != nil {
		if errors.Is(err, identityDomain.ErrProfileConflict) {
			c.AbortWithStatusJSON(stdhttp.StatusConflict, gin.H{"error": "profile was updated concurrently; reload and retry"})
			return
		}
		if errors.Is(err, identityDomain.ErrInvalidProfile) {
			c.AbortWithStatusJSON(stdhttp.StatusUnprocessableEntity, gin.H{"error": "invalid profile document"})
			return
		}
		c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "profile unavailable"})
		return
	}
	c.JSON(stdhttp.StatusOK, profileResponse(profile))
}

func RegisterRoutes(api *gin.RouterGroup, authService identityDomain.AuthService, profileService identityDomain.ProfileService, oauthRedirectURI string, authMiddleware, sensitiveLimit gin.HandlerFunc) {
	if authService != nil {
		auth := api.Group("/auth")
		if sensitiveLimit != nil {
			auth.Use(sensitiveLimit)
		}
		handler := NewAuthHandler(authService, oauthRedirectURI)
		auth.POST("/register", handler.Register)
		auth.POST("/login", handler.Login)
		auth.GET("/oauth/:provider/login", handler.BeginOAuth)
		auth.GET("/oauth/:provider/callback", handler.CompleteOAuth)
	}
	if profileService != nil && authMiddleware != nil {
		profile := api.Group("/me/profile")
		profile.Use(authMiddleware)
		handler := NewProfileHandler(profileService)
		profile.GET("", handler.Get)
		profile.PATCH("", handler.Update)
	}
}

func handleAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityDomain.ErrInvalidCredentials):
		c.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "invalid credentials"})
	case errors.Is(err, identityDomain.ErrInvalidPassword):
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "password must be between 8 and 72 characters"})
	case errors.Is(err, identityDomain.ErrEmailAlreadyExists), errors.Is(err, identityDomain.ErrPhoneAlreadyExists):
		c.AbortWithStatusJSON(stdhttp.StatusConflict, gin.H{"error": "account already exists"})
	case errors.Is(err, identityDomain.ErrOAuthProviderUnavailable):
		c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "OAuth provider is not enabled"})
	case errors.Is(err, identityDomain.ErrInvalidOAuthState):
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid OAuth callback"})
	case errors.Is(err, identityDomain.ErrOAuthAccountLinkRequired):
		c.AbortWithStatusJSON(stdhttp.StatusConflict, gin.H{"error": "OAuth account linking is required"})
	default:
		c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "authentication unavailable"})
	}
}

func toSessionResponse(session identityDomain.Session) sessionResponse {
	return sessionResponse{AccessToken: session.AccessToken, TokenType: "Bearer", UserID: session.UserID.String(), Role: string(session.Role), ExpiresAt: session.ExpiresAt}
}

func writeSession(c *gin.Context, status int, session identityDomain.Session) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, toSessionResponse(session))
}

func profileResponse(profile *identityDomain.Profile) gin.H {
	return gin.H{"schema_version": profile.SchemaVersion, "revision": profile.Revision, "attributes": profile.Attributes, "updated_at": profile.UpdatedAt}
}
