package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	slugLib "github.com/gosimple/slug"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
)

type categoryService struct {
	repo           domain.CategoryRepository
	productChecker domain.ProductChecker
	storage        storage.Storage
	redirect       redirectDomain.RedirectService
	l              logger.Logger
}

// NewCategoryService створює новий інстанс сервісу
func NewCategoryService(repo domain.CategoryRepository, productChecker domain.ProductChecker, st storage.Storage, redirect redirectDomain.RedirectService, l logger.Logger) domain.CategoryService {
	return &categoryService{
		repo:           repo,
		productChecker: productChecker,
		storage:        st,
		redirect:       redirect,
		l:              l,
	}
}

// GetList повертає плаский список категорій з перекладами на вказаній мові, відсортований ієрархічно
func (s *categoryService) GetList(ctx context.Context, lang string, showAll bool) ([]domain.Category, error) {
	categories, err := s.repo.FindAll(ctx, lang)
	if err != nil {
		return nil, err
	}

	if !showAll {
		activeCatIDs, err := s.productChecker.GetActiveCategoryIDs(ctx)
		if err != nil {
			return nil, err
		}
		categories = filterEmptyCategories(categories, activeCatIDs)
	}

	return sortCategoriesHierarchically(categories), nil
}

// filterEmptyCategories залишає лише ті категорії, які мають активні товари,
// або які мають підкатегорії з активними товарами.
func filterEmptyCategories(categories []domain.Category, activeCatIDs map[uuid.UUID]bool) []domain.Category {
	byParent := make(map[uuid.UUID][]domain.Category)
	catMap := make(map[uuid.UUID]domain.Category)
	var roots []domain.Category

	for _, cat := range categories {
		catMap[cat.ID] = cat
		if cat.ParentID == nil {
			roots = append(roots, cat)
		} else {
			byParent[*cat.ParentID] = append(byParent[*cat.ParentID], cat)
		}
	}

	keep := make(map[uuid.UUID]bool)

	// Функція обходу знизу вгору (або скоріше рекурсивно зверху вниз з поверненням результату)
	var checkKeep func(catID uuid.UUID) bool
	checkKeep = func(catID uuid.UUID) bool {
		shouldKeep := activeCatIDs[catID]
		for _, child := range byParent[catID] {
			if checkKeep(child.ID) {
				shouldKeep = true
			}
		}
		if shouldKeep {
			keep[catID] = true
		}
		return shouldKeep
	}

	for _, root := range roots {
		checkKeep(root.ID)
	}

	// Також може бути ситуація з 'сиротами'
	for _, cat := range categories {
		if _, visited := keep[cat.ID]; !visited {
			if activeCatIDs[cat.ID] {
				keep[cat.ID] = true
			}
		}
	}

	var filtered []domain.Category
	for _, cat := range categories {
		if keep[cat.ID] {
			filtered = append(filtered, cat)
		}
	}

	return filtered
}

// sortCategoriesHierarchically впорядковує плаский список категорій ієрархічно:
// спочатку кореневі (parent_id == nil), а під ними їхні підкатегорії.
// Відносний порядок (sort_order) зберігається з бази даних.
func sortCategoriesHierarchically(categories []domain.Category) []domain.Category {
	byParent := make(map[uuid.UUID][]domain.Category)
	var roots []domain.Category

	for _, cat := range categories {
		if cat.ParentID == nil {
			roots = append(roots, cat)
		} else {
			byParent[*cat.ParentID] = append(byParent[*cat.ParentID], cat)
		}
	}

	result := make([]domain.Category, 0, len(categories))
	var build func(parentID uuid.UUID)
	build = func(parentID uuid.UUID) {
		children := byParent[parentID]
		for _, child := range children {
			result = append(result, child)
			build(child.ID)
		}
	}

	for _, root := range roots {
		result = append(result, root)
		build(root.ID)
	}

	// Захист від втрати сиріт або елементів у циклі
	if len(result) < len(categories) {
		visited := make(map[uuid.UUID]bool)
		for _, cat := range result {
			visited[cat.ID] = true
		}
		for _, cat := range categories {
			if !visited[cat.ID] {
				result = append(result, cat)
			}
		}
	}

	return result
}

func (s *categoryService) GetByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Category, error) {
	return s.repo.FindByID(ctx, id, lang)
}

func (s *categoryService) GetBySlug(ctx context.Context, slug string, lang string) (*domain.Category, error) {
	return s.repo.FindBySlug(ctx, slug, lang)
}

func (s *categoryService) GenerateSlug(ctx context.Context, name string, excludeID uuid.UUID) (string, error) {
	baseSlug := slugLib.Make(name)
	if baseSlug == "" {
		baseSlug = "category"
	}

	candidate := baseSlug
	for attempts := 1; attempts <= 100; attempts++ {
		exists, err := s.repo.SlugExists(ctx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		
		// Колізія — додаємо порядковий номер
		candidate = fmt.Sprintf("%s-%d", baseSlug, attempts)
	}

	return candidate, nil
}

func (s *categoryService) Create(ctx context.Context, category *domain.Category) error {
	if category.ID == uuid.Nil {
		category.ID = uuid.New()
	}
	for i := range category.Translations {
		t := &category.Translations[i]
		if t.Slug == "" && t.Name != "" {
			slug, err := s.GenerateSlug(ctx, t.Name, category.ID)
			if err != nil {
				return fmt.Errorf("failed to generate category slug: %w", err)
			}
			t.Slug = slug
		}
	}
	s.applyTranslationFallback(category)
	return s.repo.Create(ctx, category)
}

func (s *categoryService) Update(ctx context.Context, category *domain.Category) error {
	// Отримуємо стару категорію, щоб перевірити чи змінилася іконка і слюг
	oldCategory, err := s.repo.FindByID(ctx, category.ID, "uk")
	if err == nil && oldCategory != nil {
		if oldCategory.IconURL != nil && (category.IconURL == nil || *oldCategory.IconURL != *category.IconURL) {
			s.deleteImageAsync(*oldCategory.IconURL)
		}
		
		// Перевірка на зміну імені або слюга
		for i := range category.Translations {
			t := &category.Translations[i]
			var oldT *domain.CategoryTranslation
			for j := range oldCategory.Translations {
				if oldCategory.Translations[j].LanguageCode == t.LanguageCode {
					oldT = &oldCategory.Translations[j]
					break
				}
			}

			if oldT != nil {
				oldName := oldT.Name
				oldSlug := oldT.Slug

				// Якщо імя змінилося, а слюг явно не задали новим, перегенеруємо його
				if t.Name != "" && oldName != t.Name && t.Slug == oldSlug {
					newSlug, err := s.GenerateSlug(ctx, t.Name, category.ID)
					if err != nil {
						return fmt.Errorf("failed to generate category slug: %w", err)
					}
					t.Slug = newSlug
				}

				// Якщо слюг порожній (для fallback), згенеруємо його
				if t.Slug == "" && t.Name != "" {
					slug, err := s.GenerateSlug(ctx, t.Name, category.ID)
					if err != nil {
						return fmt.Errorf("failed to generate category slug: %w", err)
					}
					t.Slug = slug
				}

				// Записуємо історію, якщо слюг змінився
				if oldSlug != "" && oldSlug != t.Slug {
					_ = s.redirect.RecordSlugChange(ctx, "category", category.ID, oldSlug)
				}
			} else {
				if t.Slug == "" && t.Name != "" {
					slug, err := s.GenerateSlug(ctx, t.Name, category.ID)
					if err != nil {
						return fmt.Errorf("failed to generate category slug: %w", err)
					}
					t.Slug = slug
				}
			}
		}
	} else {
		// Fallback якщо стару категорію не знайдено (наприклад для нових мов)
		for i := range category.Translations {
			t := &category.Translations[i]
			if t.Slug == "" && t.Name != "" {
				slug, err := s.GenerateSlug(ctx, t.Name, category.ID)
				if err != nil {
					return fmt.Errorf("failed to generate category slug: %w", err)
				}
				t.Slug = slug
			}
		}
	}

	s.applyTranslationFallback(category)
	return s.repo.Update(ctx, category)
}

func (s *categoryService) Delete(ctx context.Context, id uuid.UUID) error {
	count, err := s.productChecker.CountByCategory(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return domain.ErrCategoryInUse
	}

	// Видаляємо іконку перед видаленням категорії
	if oldCategory, err := s.repo.FindByID(ctx, id, "uk"); err == nil && oldCategory != nil && oldCategory.IconURL != nil {
		s.deleteImageAsync(*oldCategory.IconURL)
	}

	return s.repo.Delete(ctx, id)
}

func (s *categoryService) UpdateOrder(ctx context.Context, ids []uuid.UUID) error {
	return s.repo.UpdateOrder(ctx, ids)
}

func (s *categoryService) applyTranslationFallback(category *domain.Category) {
	var ukT, enT *domain.CategoryTranslation
	for i := range category.Translations {
		t := &category.Translations[i]
		if t.LanguageCode == "uk" {
			ukT = t
		}
		if t.LanguageCode == "en" {
			enT = t
		}
	}

	if ukT != nil && enT != nil {
		if enT.Name == "" {
			enT.Name = ukT.Name
		}
	}
}

func (s *categoryService) deleteImageAsync(url string) {
	if s.storage == nil {
		return
	}
	publicID := extractCategoryPublicID(url)
	if publicID == "" {
		return
	}
	// Асинхронне видалення, щоб не блокувати запит
	go func() {
		ctx := context.Background()
		if err := s.storage.Delete(ctx, publicID); err != nil {
			s.l.Errorw("failed to delete category icon", "publicID", publicID, "error", err)
		}
	}()
}

func extractCategoryPublicID(url string) string {
	// Шукаємо шлях до Cloudinary, зазвичай це /categories/ або /general/
	idx := strings.Index(url, "/categories/")
	if idx == -1 {
		idx = strings.Index(url, "/general/")
		if idx == -1 {
			return ""
		}
	}
	path := url[idx+1:]
	extIdx := strings.LastIndex(path, ".")
	if extIdx != -1 {
		path = path[:extIdx]
	}
	return path
}
