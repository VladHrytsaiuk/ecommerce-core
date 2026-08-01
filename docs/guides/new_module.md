# Як додати новий модуль (Clean Architecture)

Цей проєкт побудовано за принципами **Clean Architecture**. Кожен доменний модуль (наприклад, `order`, `product`, `user`) є ізольованим і має чітку структуру.

У цьому гайді описано покроковий процес створення нового модуля на прикладі гіпотетичного модуля `blog` (блог магазину).

---

## 🏗 Крок 1. Створення структури папок

Створіть нову папку в директорії `internal/`:

```bash
mkdir -p internal/blog/domain
mkdir -p internal/blog/repository/postgres
mkdir -p internal/blog/service
mkdir -p internal/blog/delivery/http
```

---

## 📦 Крок 2. Доменний шар (`domain`)

Доменний шар містить бізнес-моделі (структури бази даних/домену) та контракти (інтерфейси) для Репозиторіїв та Сервісів. Цей шар **нічого не знає** про GORM, Gin чи інші зовнішні бібліотеки.

Створіть файл `internal/blog/domain/post.go`:

```go
package domain

import (
	"context"
	"time"
	"github.com/google/uuid"
)

// Post - наша бізнес-модель
type Post struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PostRepository описує контракт роботи з базою даних
type PostRepository interface {
	Create(ctx context.Context, post *Post) error
	GetByID(ctx context.Context, id uuid.UUID) (*Post, error)
	List(ctx context.Context) ([]Post, error)
}

// PostService описує контракт бізнес-логіки
type PostService interface {
	CreatePost(ctx context.Context, title, content string) (*Post, error)
	GetPost(ctx context.Context, id uuid.UUID) (*Post, error)
	GetAllPosts(ctx context.Context) ([]Post, error)
}
```

---

## 🗄 Крок 3. Репозиторій (`repository/postgres`)

Тут ми імплементуємо `PostRepository` використовуючи `gorm`. Цей шар знає **тільки** про базу даних і доменні моделі.

Створіть файл `internal/blog/repository/postgres/post_repository.go`:

```go
package postgres

import (
	"context"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/blog/domain"
	"gorm.io/gorm"
	"github.com/google/uuid"
)

type postRepository struct {
	db *gorm.DB
}

func NewPostRepository(db *gorm.DB) domain.PostRepository {
	return &postRepository{db: db}
}

func (r *postRepository) Create(ctx context.Context, post *domain.Post) error {
	return r.db.WithContext(ctx).Create(post).Error
}

func (r *postRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Post, error) {
	var post domain.Post
	if err := r.db.WithContext(ctx).First(&post, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *postRepository) List(ctx context.Context) ([]domain.Post, error) {
	var posts []domain.Post
	if err := r.db.WithContext(ctx).Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}
```

> **Увага:** Не забудьте додати нову таблицю у файли міграцій (`docs/guides/database_migrate.md`)!

---

## ⚙️ Крок 4. Сервісний шар (`service`)

Тут лежить бізнес-логіка. Сервіс викликає методи репозиторію. Він не знає про Gin (HTTP) чи базу даних напряму.

Створіть файл `internal/blog/service/post_service.go`:

```go
package service

import (
	"context"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/blog/domain"
	"github.com/google/uuid"
	"time"
)

type postService struct {
	repo domain.PostRepository
}

func NewPostService(repo domain.PostRepository) domain.PostService {
	return &postService{repo: repo}
}

func (s *postService) CreatePost(ctx context.Context, title, content string) (*domain.Post, error) {
	post := &domain.Post{
		ID:        uuid.New(),
		Title:     title,
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.Create(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *postService) GetPost(ctx context.Context, id uuid.UUID) (*domain.Post, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *postService) GetAllPosts(ctx context.Context) ([]domain.Post, error) {
	return s.repo.List(ctx)
}
```

---

## 🌐 Крок 5. HTTP Delivery (Хендлери)

Тут ми підключаємо `gin`, обробляємо вхідний JSON, валідуємо його та викликаємо `service`.

Створіть файл `internal/blog/delivery/http/post_handler.go`:

```go
package http

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/blog/domain"
)

type PostHandler struct {
	service domain.PostService
}

func NewPostHandler(service domain.PostService) *PostHandler {
	return &PostHandler{service: service}
}

// CreateRequest DTO для вхідних даних
type CreateRequest struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// RegisterRoutes підключає роути до Gin RouterGroup
func (h *PostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	posts := rg.Group("/posts")
	{
		posts.POST("", h.Create)
		posts.GET("/:id", h.Get)
		posts.GET("", h.List)
	}
}

func (h *PostHandler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	post, err := h.service.CreatePost(c.Request.Context(), req.Title, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, post)
}

func (h *PostHandler) Get(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid uuid"})
		return
	}

	post, err := h.service.GetPost(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) List(c *gin.Context) {
	posts, err := h.service.GetAllPosts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, posts)
}
```

---

## 🔌 Крок 6. Ініціалізація модуля (Dependency Injection)

Нарешті, нам потрібно зібрати всі ці шари разом під час старту застосунку. Усі модулі ініціалізуються у файлі `internal/http/router.go`.

Відкрийте `internal/http/router.go` та додайте наступне:

```go
// 1. Додайте імпорти
import (
    bloghttp "github.com/VladHrytsaiuk/ecommerce-core/internal/blog/delivery/http"
    blogrepo "github.com/VladHrytsaiuk/ecommerce-core/internal/blog/repository/postgres"
    blogservice "github.com/VladHrytsaiuk/ecommerce-core/internal/blog/service"
)

// 2. У функції InitRouter зберіть залежності
func InitRouter(ctx context.Context, cfg *config.Config, db *gorm.DB, tokenMaker token.Maker) *gin.Engine {
    // ... існуючий код ...

    // --- BLOG MODULE ---
    postRepo := blogrepo.NewPostRepository(db)
    postService := blogservice.NewPostService(postRepo)
    postHandler := bloghttp.NewPostHandler(postService)

    // 3. Зареєструйте роути в загальній групі /api
    apiGroup := r.Group("/api")
    {
        // ... існуючі групи ...
        postHandler.RegisterRoutes(apiGroup)
    }

    return r
}
```

✅ **Готово!** Ви створили новий модуль з дотриманням усіх архітектурних стандартів проєкту. Тепер він легко тестується, підтримується і може розширюватися.
