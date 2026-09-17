// Package http translates public identity HTTP contracts to provider-neutral
// application services. It does not construct providers or repositories.
package http

import (
	"errors"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
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
	// RefreshToken is returned once, with each sign-in and each refresh.
	RefreshToken string `json:"refresh_token,omitempty"`
	// RefreshExpiresAt is when the sign-in ends; refreshing never moves it.
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Register godoc
// @Summary Register a customer account
// @Description Accepts an email, a phone number, or both. Returns a session immediately; the account starts unverified.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body registerRequest true "Credentials"
// @Success 201 {object} sessionResponse
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid registration request"})
		return
	}
	guestSessionID := guestSessionID(c)
	session, err := h.service.RegisterPassword(c.Request.Context(), identityDomain.RegisterPasswordCommand{Email: request.Email, Phone: request.Phone, Password: request.Password, GuestSessionID: guestSessionID})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusCreated, session)
}

// Login godoc
// @Summary Exchange credentials for an access token
// @Description The login field accepts an email or a phone number. Failed attempts cost the same as successful ones, so timing does not reveal whether an account exists.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body loginRequest true "Credentials"
// @Success 200 {object} sessionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Router /api/auth/login [post]
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
	guestSessionID := guestSessionID(c)
	session, err := h.service.LoginPassword(c.Request.Context(), identityDomain.PasswordLoginCommand{Login: login, Password: request.Password, GuestSessionID: guestSessionID})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusOK, session)
}

// Refresh godoc
// @Summary Exchange a refresh token for a new session
// @Description Consumes the refresh token and returns a new access token and a new refresh token; the one presented stops working. Presenting a used refresh token again more than 30 seconds after its use ends the whole sign-in; within 30 seconds, as when two browser tabs refresh together, it is only refused. Refreshing never extends the sign-in past refresh_expires_at.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body refreshRequest true "Refresh token"
// @Success 200 {object} sessionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid refresh request"})
		return
	}
	session, err := h.service.RefreshSession(c.Request.Context(), request.RefreshToken)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusOK, session)
}

// Logout godoc
// @Summary End a sign-in
// @Description Revokes the refresh token and every refresh token issued from the same sign-in. An access token already issued stays valid until it expires. Answers 204 whether or not the token was known.
// @Tags Auth
// @Accept json
// @Param payload body refreshRequest true "Refresh token"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid logout request"})
		return
	}
	if err := h.service.RevokeSession(c.Request.Context(), request.RefreshToken); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "authentication unavailable"})
		return
	}
	c.Status(stdhttp.StatusNoContent)
}

type emailCodeRequest struct {
	Email string `json:"email" binding:"required"`
}

type emailCodeVerifyRequest struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

type signInCodeResponse struct {
	// ExpiresAt is when the code stops working.
	ExpiresAt time.Time `json:"expires_at"`
	// ResendAfter is the earliest another code may be requested.
	ResendAfter time.Time `json:"resend_after"`
}

// RequestEmailCode godoc
// @Summary Email a one-time sign-in code
// @Description Sends a six-digit code to the address. The same request signs in to an existing account and registers a new one, so the response is the same whether or not the address has an account. A new code replaces any earlier one. An address may be sent one code a minute, five an hour and ten a day; past that the response is 429 with Retry-After.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body emailCodeRequest true "Address"
// @Success 202 {object} signInCodeResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/auth/email-code [post]
func (h *AuthHandler) RequestEmailCode(c *gin.Context) {
	var request emailCodeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid sign-in code request"})
		return
	}
	issued, err := h.service.RequestSignInCode(c.Request.Context(), identityDomain.RequestSignInCodeCommand{Channel: identityDomain.SignInCodeEmail, Destination: request.Email})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	c.JSON(stdhttp.StatusAccepted, signInCodeResponse{ExpiresAt: issued.ExpiresAt, ResendAfter: issued.ResendAfter})
}

// VerifyEmailCode godoc
// @Summary Sign in with an emailed code
// @Description Exchanges the code for a session. An address with no account gets a customer account, with the address verified. An account whose address was never verified is taken over by the person who received the code: its password is removed and its other sign-ins end. A code allows five attempts; a wrong, expired, replaced or used code is 401. A guest cart in the request cookie is carried into the session.
// @Tags Auth
// @Accept json
// @Produce json
// @Param payload body emailCodeVerifyRequest true "Address and code"
// @Success 200 {object} sessionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/auth/email-code/verify [post]
func (h *AuthHandler) VerifyEmailCode(c *gin.Context) {
	var request emailCodeVerifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid sign-in code request"})
		return
	}
	session, err := h.service.VerifySignInCode(c.Request.Context(), identityDomain.VerifySignInCodeCommand{Channel: identityDomain.SignInCodeEmail, Destination: request.Email, Code: request.Code, GuestSessionID: guestSessionID(c)})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusOK, session)
}

// BeginOAuth godoc
// @Summary Start an OAuth sign-in
// @Description Redirects to the provider. Returns 404 where the deployment has configured no OAuth provider.
// @Tags Auth
// @Param provider path string true "Provider code" Enums(google)
// @Success 302 {string} string "Redirect to the provider"
// @Failure 404 {object} map[string]string
// @Router /api/auth/oauth/{provider}/login [get]
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

// CompleteOAuth godoc
// @Summary Complete an OAuth sign-in
// @Description Called by the provider. A guest cart in the request cookie is carried into the authenticated session.
// @Tags Auth
// @Produce json
// @Param provider path string true "Provider code" Enums(google)
// @Param code query string true "Authorization code"
// @Param state query string true "Opaque state issued at the start of the flow"
// @Success 200 {object} sessionResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /api/auth/oauth/{provider}/callback [get]
func (h *AuthHandler) CompleteOAuth(c *gin.Context) {
	if h.oauthRedirectURI == "" {
		c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "OAuth provider is not enabled"})
		return
	}
	guestSessionID := guestSessionID(c)
	session, err := h.service.CompleteOAuth(c.Request.Context(), identityDomain.CompleteOAuthCommand{Provider: c.Param("provider"), RedirectURI: h.oauthRedirectURI, Code: c.Query("code"), State: c.Query("state"), GuestSessionID: guestSessionID})
	if err != nil {
		handleAuthError(c, err)
		return
	}
	writeSession(c, stdhttp.StatusOK, session)
}

// Get godoc
// @Summary Read the authenticated account's profile
// @Tags Users
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security bearerAuth
// @Router /api/me/profile [get]
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

// Update godoc
// @Summary Patch the authenticated account's profile
// @Description The document is validated against the deployment's configured profile policy.
// @Tags Users
// @Accept json
// @Produce json
// @Param payload body map[string]interface{} true "Profile fields to change"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Security bearerAuth
// @Router /api/me/profile [patch]
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

// SignInMethods says which sign-in routes exist. A method the store does not
// offer has no route at all, rather than a route that answers with a refusal.
type SignInMethods struct {
	Password  bool
	OAuth     bool
	EmailCode bool
}

func RegisterRoutes(api *gin.RouterGroup, authService identityDomain.AuthService, profileService identityDomain.ProfileService, methods SignInMethods, oauthRedirectURI string, authMiddleware, sensitiveLimit gin.HandlerFunc, loginLimit ...gin.HandlerFunc) {
	if authService != nil {
		auth := api.Group("/auth")
		if sensitiveLimit != nil {
			auth.Use(sensitiveLimit)
		}
		handler := NewAuthHandler(authService, oauthRedirectURI)
		// Signing in, and asking for a code to sign in with, share the per-IP
		// login limit: the one guards against guessing, the other against using
		// the store to send mail to strangers.
		guarded := func(handler gin.HandlerFunc) []gin.HandlerFunc {
			if len(loginLimit) > 0 && loginLimit[0] != nil {
				return []gin.HandlerFunc{loginLimit[0], handler}
			}
			return []gin.HandlerFunc{handler}
		}
		if methods.Password {
			auth.POST("/register", handler.Register)
			auth.POST("/login", guarded(handler.Login)...)
		}
		if methods.EmailCode {
			auth.POST("/email-code", guarded(handler.RequestEmailCode)...)
			auth.POST("/email-code/verify", guarded(handler.VerifyEmailCode)...)
		}
		// Every method ends in the same session, so these exist whichever
		// methods are on.
		auth.POST("/refresh", handler.Refresh)
		auth.POST("/logout", handler.Logout)
		if methods.OAuth {
			auth.GET("/oauth/:provider/login", handler.BeginOAuth)
			auth.GET("/oauth/:provider/callback", handler.CompleteOAuth)
		}
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
	var throttled *identityDomain.SignInCodeThrottledError
	switch {
	case errors.Is(err, identityDomain.ErrSignInMethodDisabled):
		c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "sign-in method is not enabled"})
	case errors.As(err, &throttled):
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(throttled.RetryAfter)))
		c.AbortWithStatusJSON(stdhttp.StatusTooManyRequests, gin.H{"error": "too many sign-in codes requested for this address"})
	case errors.Is(err, identityDomain.ErrInvalidSignInDestination):
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid email address"})
	case errors.Is(err, identityDomain.ErrInvalidSignInCode):
		c.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "invalid or expired sign-in code"})
	case errors.Is(err, identityDomain.ErrInvalidRefreshToken):
		c.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
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

// retryAfterSeconds rounds up, so a client that waits exactly as told is not
// refused again for a fraction of a second.
func retryAfterSeconds(wait time.Duration) int {
	seconds := int((wait + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func toSessionResponse(session identityDomain.Session) sessionResponse {
	response := sessionResponse{AccessToken: session.AccessToken, TokenType: "Bearer", UserID: session.UserID.String(), Role: string(session.Role), ExpiresAt: session.ExpiresAt}
	if session.RefreshToken != "" {
		refreshExpiresAt := session.RefreshExpiresAt
		response.RefreshToken, response.RefreshExpiresAt = session.RefreshToken, &refreshExpiresAt
	}
	return response
}

func writeSession(c *gin.Context, status int, session identityDomain.Session) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, toSessionResponse(session))
}

func profileResponse(profile *identityDomain.Profile) gin.H {
	return gin.H{"schema_version": profile.SchemaVersion, "revision": profile.Revision, "attributes": profile.Attributes, "updated_at": profile.UpdatedAt}
}

func guestSessionID(c *gin.Context) *uuid.UUID {
	sessionID, err := cartowner.GuestSessionID(c)
	if err != nil {
		return nil
	}
	return sessionID
}
