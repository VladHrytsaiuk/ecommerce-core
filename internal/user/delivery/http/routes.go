package http

import (
	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// RegisterRoutes підключає всі ендпоінти домену `user` до головного роутера Gin.
func RegisterRoutes(
	apiGroup *gin.RouterGroup,
	adminApiGroup *gin.RouterGroup,
	authH *AuthHandler,
	userH *UserHandler,
	authMiddleware gin.HandlerFunc,
	optionalAuthMiddleware gin.HandlerFunc,
	otpSendRateLimit gin.HandlerFunc,
	customerRateLimiter gin.HandlerFunc,
	adminRateLimiter gin.HandlerFunc,
) {
	// ==========================================
	// 1. Auth & Users (Публічні або спеціальні)
	// ==========================================
	authGrp := apiGroup.Group("/auth")
	{
		authGrp.POST("/logout", authH.Logout)

		// Ендпоінти з обмеженням частоти (Rate Limit) для запобігання SendGrid/SMS abuse (справжня відправка)
		otpSendLimited := authGrp.Group("")
		otpSendLimited.Use(otpSendRateLimit)
		{
			otpSendLimited.POST("/register", authH.Register)
			otpSendLimited.POST("/resend-verification", authH.ResendVerification)
			otpSendLimited.POST("/forgot-password", authH.ForgotPassword)
			otpSendLimited.POST("/verify-phone/request", optionalAuthMiddleware, authH.RequestPhoneVerification)
		}

		// Підтвердження та налаштування паролів (не відправляють листи/SMS, тому використовують стандартний лімітер клієнтів)
		authGrp.POST("/verify-email", customerRateLimiter, authH.VerifyEmail)
		authGrp.POST("/reset-password", customerRateLimiter, authH.ResetPassword)
		authGrp.POST("/setup-password", customerRateLimiter, authH.SetupPassword)
		authGrp.POST("/verify-phone/confirm", optionalAuthMiddleware, customerRateLimiter, authH.ConfirmPhoneVerification)

		authGrp.POST("/login", customerRateLimiter, authH.Login)
		authGrp.POST("/google", authH.LoginWithGoogle)
		authGrp.POST("/refresh", customerRateLimiter, authH.Refresh)
		authGrp.GET("/ws", authH.WSConnect)
	}

	// ==========================================
	// 1.5 Admin Auth (Тільки для адміністраторів)
	// ==========================================
	if adminApiGroup != nil {
		adminAuthGrp := adminApiGroup.Group("/auth")
		{
			adminAuthGrp.POST("/login", adminRateLimiter, authH.AdminLogin)
			adminAuthGrp.POST("/refresh", adminRateLimiter, authH.AdminRefresh)
		}

		// adminApiGroup також містить публічні /auth/login і /auth/refresh,
		// тому захист для користувачів додаємо явно, не успадковуючи його від групи.
		adminUsersGrp := adminApiGroup.Group("/users")
		adminUsersGrp.Use(authMiddleware, middleware.AdminMiddleware())
		{
			adminUsersGrp.GET("/me", userH.GetMe)
			adminUsersGrp.PATCH("/me", userH.UpdateMe)

			ownerUsersGrp := adminUsersGrp.Group("")
			ownerUsersGrp.Use(middleware.OwnerMiddleware())
			ownerUsersGrp.GET("", authH.ListAdmins)
			ownerUsersGrp.POST("", authH.CreateAdmin)
			ownerUsersGrp.DELETE("/:id", authH.DeleteAdmin)
		}
	}

	// ==========================================
	// 2. User Profile & Addresses (Захищені)
	// ==========================================
	usersGrp := apiGroup.Group("/users")
	usersGrp.Use(authMiddleware) // Захищаємо всі маршрути в цій групі
	{
		// Профіль
		usersGrp.GET("/me", userH.GetMe)
		usersGrp.PATCH("/me", userH.UpdateMe)
		usersGrp.DELETE("/me", userH.DeleteMe)

		// Збережені адреси
		usersGrp.GET("/me/addresses", userH.GetAddresses)
		usersGrp.POST("/me/addresses", userH.CreateAddress)
		usersGrp.POST("/me/addresses/checkout", userH.SaveCheckoutAddress)
		usersGrp.PATCH("/me/addresses/:id", userH.UpdateAddress)
		usersGrp.DELETE("/me/addresses/:id", userH.DeleteAddress)
	}
}
