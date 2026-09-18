package app

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
)

// buildHTTPDependencies assembles every middleware the router attaches. The
// router itself only wires already-constructed handlers, so this is where all
// transport policy — limits, headers, locale negotiation — is decided.
func buildHTTPDependencies(cfg *config.Config, storeConfig StoreConfig, tokenMaker token.Maker, loginLimiter ratelimit.Service) (HTTPDependencies, error) {
	cors, err := middleware.NewCORS(cfg.CORSAllowOrigins)
	if err != nil {
		return HTTPDependencies{}, fmt.Errorf("configure CORS: %w", err)
	}
	errorRenderer := apiresponse.NewErrorRenderer(logger.Log)
	// Two limiters, one middleware. Which policy applies is decided by what is
	// handed in, not by a flag inside the middleware.
	//
	// Login keeps the strict limiter: when it cannot reach its store the
	// request is refused, because losing brute-force protection is worse than
	// failing a login. Public browsing takes the same limiter behind a
	// fallback, so a Redis outage degrades limits to per-process windows
	// instead of returning 503 for the whole versioned API — catalog, checkout
	// and orders included.
	browsingLimiter := ratelimit.NewFallback(loginLimiter, ratelimit.NewLocalService(), logger.Log)
	return HTTPDependencies{
		Recovery:      middleware.PanicRecovery(errorRenderer),
		Observability: observability.Middleware(),
		Timeout:       middleware.TimeoutMiddleware(cfg.RequestTimeout),
		RequestLogging: func(c *gin.Context) {
			logger.WithContext(c.Request.Context()).Infow("Inbound Request", "method", c.Request.Method, "path", c.Request.URL.Path, "ip", c.ClientIP())
			c.Next()
		},
		CORS:             cors,
		SecurityHeaders:  middleware.SecurityHeaders(),
		ErrorRenderer:    errorRenderer,
		Swagger:          ginSwagger.WrapHandler(swaggerFiles.Handler),
		Health:           func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) },
		LocaleMiddleware: middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: storeConfig.DefaultLocale, FallbackLocale: storeConfig.FallbackLocale, SupportedLocales: storeConfig.SupportedLocales}),
		OptionalAuth:     middleware.OptionalAuthMiddleware(tokenMaker),
		LoginRateLimit:   middleware.LoginRateLimitMiddleware(loginLimiter, cfg.JWTSecret),
		CodeRateLimit:    middleware.CodeRateLimitMiddleware(loginLimiter, cfg.JWTSecret),
		APIRateLimit:     middleware.RateLimitByIP(browsingLimiter, "api:v1", cfg.APIRateLimitPerMin, time.Minute, cfg.JWTSecret, errorRenderer),
		RequestBodyLimit: middleware.MaxRequestBodyBytes(1<<20, errorRenderer),
		// Includes multipart framing while ReadImagePart independently enforces
		// a strict 15 MiB limit for the file itself.
		MediaRequestBodyLimit: middleware.MaxRequestBodyBytes(16<<20, errorRenderer),
	}, nil
}
