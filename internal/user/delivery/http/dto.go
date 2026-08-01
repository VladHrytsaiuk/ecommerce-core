package http

// ErrorResponse представляє стандартний формат помилки API
type ErrorResponse struct {
	Error   string `json:"error" example:"Unauthorized"`
	Message string `json:"message" example:"Детальний опис помилки"`
}

// ------ Auth DTOs ------

// RegisterRequest містить дані для реєстрації нового користувача
type RegisterRequest struct {
	FirstName string `json:"first_name" binding:"required" example:"Іван"`
	LastName  string `json:"last_name" binding:"required" example:"Франко"`
	Email     string `json:"email" binding:"required,email" example:"ivan@example.com"`
	Password  string `json:"password" binding:"required,min=8" example:"Secret123!"`
}

// CreateAdminRequest містить початкові дані нового адміністратора або Власника.
type CreateAdminRequest struct {
	FirstName string `json:"first_name" binding:"required" example:"Іван"`
	LastName  string `json:"last_name" binding:"required" example:"Франко"`
	Email     string `json:"email" binding:"required,email" example:"ivan@example.com"`
	Password  string `json:"password" binding:"required,min=8" example:"Secret123!"`
	RoleID    int    `json:"role_id" binding:"required,oneof=2 3" example:"2"`
}

// LoginRequest містить дані для входу
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email" example:"ivan@example.com"`
	Password string `json:"password" binding:"required" example:"Secret123!"`
}

// GoogleLoginRequest містить ID токен для входу через Google
type GoogleLoginRequest struct {
	IDToken string `json:"id_token" binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// RefreshRequest містить дані для оновлення токенів
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// RegisterResponse повертається при успішній реєстрації
type RegisterResponse struct {
	Message string       `json:"message" example:"Registration successful. Please check your email for verification link."`
	User    UserResponse `json:"user"`
}

// AuthResponse повертається при успішній авторизації
type AuthResponse struct {
	AccessToken  string       `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string       `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	User         UserResponse `json:"user"`
}

// VerifyEmailRequest містить email та код для підтвердження пошти
type VerifyEmailRequest struct {
	Email string `json:"email" binding:"required,email" example:"ivan@example.com"`
	Code  string `json:"code" binding:"required" example:"123456"`
}

// ForgotPasswordRequest містить email для відновлення паролю
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email" example:"ivan@example.com"`
}

// ResetPasswordRequest містить код і новий пароль
type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email" example:"ivan@example.com"`
	Code        string `json:"code" binding:"required" example:"123456"`
	NewPassword string `json:"new_password" binding:"required,min=8" example:"NewSecret123!"`
}

// SetupPasswordRequest містить токен та новий пароль для гостьового чекауту
type SetupPasswordRequest struct {
	SetupToken string `json:"setup_token" binding:"required" example:"a1b2c3d4-..."`
	Password   string `json:"password" binding:"required,min=8" example:"MySecurePassword123!"`
}

// MessageResponse стандартна відповідь про успішну дію (без даних)
type MessageResponse struct {
	Message string `json:"message" example:"Операцію успішно виконано"`
}

// ------ User DTOs ------

// UserResponse описує публічні дані профілю
type UserResponse struct {
	ID              string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	RoleID          int     `json:"role_id" example:"1"`
	FirstName       string  `json:"first_name" example:"Іван"`
	LastName        string  `json:"last_name" example:"Франко"`
	Email           string  `json:"email" example:"ivan@example.com"`
	Phone           *string `json:"phone" example:"+380501234567"`
	IsEmailVerified bool    `json:"is_email_verified" example:"true"`
	IsPhoneVerified bool    `json:"is_phone_verified" example:"false"`
	WantsNewsletter bool    `json:"wants_newsletter" example:"true"`
	AuthProvider    string  `json:"auth_provider" example:"local"`
	AvatarURL       string  `json:"avatar_url" example:"https://example.com/photo.jpg"`
}

// UpdateUserRequest використовує вказівники для PATCH-запиту (оновляться лише передані поля)
type UpdateUserRequest struct {
	FirstName       *string `json:"first_name,omitempty" example:"Іван"`
	LastName        *string `json:"last_name,omitempty" example:"Франко"`
	Phone           *string `json:"phone,omitempty" example:"+380991112233"`
	WantsNewsletter *bool   `json:"wants_newsletter,omitempty" example:"false"`
}

// ------ Address DTOs ------

// UserAddressResponse відповідь з даними збереженої адреси
type UserAddressResponse struct {
	ID            string `json:"id" example:"ceb4c031-8a0b-40ec-b661-3023080e247d"`
	AddressName   string `json:"address_name" example:"Дім"`
	Provider      string `json:"provider" example:"NOVA_POSHTA"`
	DeliveryType  string `json:"delivery_type" example:"BRANCH"`
	FullAddress   string `json:"full_address" example:"м. Львів, Відділення №18..."`
	CityRef       string `json:"city_ref" example:"db5c88f5-391c-11dd-90d9-001a92567626"`
	CityName      string `json:"city_name" example:"Львів"`
	AreaRef       string `json:"area_ref" example:"71508134-9b87-11de-822f-000c2965ae0e"`
	AreaName      string `json:"area_name" example:"Львівська"`
	WarehouseRef  string `json:"warehouse_ref" example:"0d545edf-e1c2-11e3-8c4a-0050568002cf"`
	WarehouseName string `json:"warehouse_name" example:"Відділення №5"`
	IsDefault     bool   `json:"is_default" example:"true"`
}

// CreateAddressRequest запит на створення нової адреси
type CreateAddressRequest struct {
	AddressName   string `json:"address_name,omitempty" example:"Дім"`
	Provider      string `json:"provider" binding:"required" example:"NOVA_POSHTA"`
	DeliveryType  string `json:"delivery_type" binding:"required" example:"BRANCH"`
	CityRef       string `json:"city_ref" binding:"required" example:"8d5a980d-391c-11dd-90d9-001a92567626"`
	CityName      string `json:"city_name" binding:"required" example:"Київ"`
	AreaRef       string `json:"area_ref" example:"71000000-0000-0000-0000-000000000000"`
	AreaName      string `json:"area_name" example:"Київська"`
	WarehouseRef  string `json:"warehouse_ref" binding:"required" example:"1ec09d88-e1c2-11e3-8c4a-0050568002cf"`
	WarehouseName string `json:"warehouse_name" binding:"required" example:"Відділення №5"`
	IsDefault     bool   `json:"is_default" example:"true"`
}

// UpdateAddressRequest використовує вказівники для часткового оновлення адреси
type UpdateAddressRequest struct {
	AddressName   *string `json:"address_name,omitempty" example:"Робота"`
	Provider      *string `json:"provider,omitempty" example:"NOVA_POSHTA"`
	DeliveryType  *string `json:"delivery_type,omitempty" example:"BRANCH"`
	CityRef       *string `json:"city_ref,omitempty" example:"8d5a980d-391c-11dd-90d9-001a92567626"`
	CityName      *string `json:"city_name,omitempty" example:"Київ"`
	AreaRef       *string `json:"area_ref,omitempty" example:"71000000-0000-0000-0000-000000000000"`
	AreaName      *string `json:"area_name,omitempty" example:"Київська"`
	WarehouseRef  *string `json:"warehouse_ref,omitempty" example:"1ec09d88-e1c2-11e3-8c4a-0050568002cf"`
	WarehouseName *string `json:"warehouse_name,omitempty" example:"Відділення №10"`
	IsDefault     *bool   `json:"is_default,omitempty" example:"false"`
}

// SaveCheckoutAddressRequest запит на автоматичне збереження адреси під час чекауту
type SaveCheckoutAddressRequest struct {
	Provider      string `json:"provider" binding:"required" example:"NOVA_POSHTA"`
	DeliveryType  string `json:"delivery_type" binding:"required" example:"BRANCH"`
	CityRef       string `json:"city_ref" binding:"required" example:"8d5a980d-391c-11dd-90d9-001a92567626"`
	CityName      string `json:"city_name" binding:"required" example:"Київ"`
	AreaRef       string `json:"area_ref" example:"71000000-0000-0000-0000-000000000000"`
	AreaName      string `json:"area_name" example:"Київська"`
	WarehouseRef  string `json:"warehouse_ref" binding:"required" example:"1ec09d88-e1c2-11e3-8c4a-0050568002cf"`
	WarehouseName string `json:"warehouse_name" binding:"required" example:"Відділення №5"`
}

// ------ Phone Verification DTOs ------

// VerifyPhoneRequest запит на відправку SMS коду
type VerifyPhoneRequest struct {
	Phone string `json:"phone" binding:"required" example:"+380991112233"`
}

// VerifyPhoneResponse повертає код для зручності розробки/тестів
type VerifyPhoneResponse struct {
	Message string `json:"message" example:"Код відправлено"`
	Code    string `json:"code,omitempty" example:"123456"`
}

// ConfirmPhoneRequest містить код для перевірки
type ConfirmPhoneRequest struct {
	Phone string `json:"phone" binding:"required" example:"+380991112233"`
	Code  string `json:"code" binding:"required" example:"123456"`
}
