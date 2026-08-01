package main

import (
	"fmt"
	"log"

	"os"

	"github.com/gosimple/slug"
	"github.com/joho/godotenv"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DB_URL")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	slug.CustomSub = map[string]string{
		"а": "a", "б": "b", "в": "v", "г": "h", "ґ": "g",
		"д": "d", "е": "e", "є": "ie", "ж": "zh", "з": "z",
		"и": "y", "і": "i", "ї": "i", "й": "i", "к": "k",
		"л": "l", "м": "m", "н": "n", "о": "o", "п": "p",
		"р": "r", "с": "s", "т": "t", "у": "u", "ф": "f",
		"х": "kh", "ц": "ts", "ч": "ch", "ш": "sh", "щ": "shch",
		"ь": "", "ю": "iu", "я": "ia",
	}

	// 1. Clean Category Translations
	var catTranslations []domain.CategoryTranslation
	if err := db.Find(&catTranslations).Error; err != nil {
		log.Fatal(err)
	}

	for _, ct := range catTranslations {
		baseSlug := slug.Make(ct.Name)
		newSlug := baseSlug
		
		for attempt := 0; attempt < 100; attempt++ {
			if attempt > 0 {
				newSlug = fmt.Sprintf("%s-%d", baseSlug, attempt)
			}
			var count int64
			db.Model(&domain.CategoryTranslation{}).Where("slug = ? AND category_id != ?", newSlug, ct.CategoryID).Count(&count)
			if count == 0 {
				break
			}
		}

		if newSlug != ct.Slug {
			if err := db.Model(&ct).Where("category_id = ? AND language_code = ?", ct.CategoryID, ct.LanguageCode).UpdateColumn("slug", newSlug).Error; err != nil {
				log.Printf("Failed to update category translation %v %v: %v", ct.CategoryID, ct.LanguageCode, err)
			} else {
				log.Printf("Updated category translation %v %v: %s -> %s", ct.CategoryID, ct.LanguageCode, ct.Slug, newSlug)
			}
		}
	}

	// 2. Clean Product Translations
	var prodTranslations []productDomain.ProductTranslation
	if err := db.Find(&prodTranslations).Error; err != nil {
		log.Fatal(err)
	}

	for _, pt := range prodTranslations {
		baseSlug := slug.Make(pt.Name)
		newSlug := baseSlug
		
		for attempt := 0; attempt < 100; attempt++ {
			if attempt > 0 {
				newSlug = fmt.Sprintf("%s-%d", baseSlug, attempt)
			}
			var count int64
			db.Model(&productDomain.ProductTranslation{}).Where("slug = ? AND product_id != ?", newSlug, pt.ProductID).Count(&count)
			if count == 0 {
				break
			}
		}

		if newSlug != pt.Slug {
			if err := db.Model(&pt).Where("product_id = ? AND language_code = ?", pt.ProductID, pt.LanguageCode).UpdateColumn("slug", newSlug).Error; err != nil {
				log.Printf("Failed to update product translation %v %v: %v", pt.ProductID, pt.LanguageCode, err)
			} else {
				log.Printf("Updated product translation %v %v: %s -> %s", pt.ProductID, pt.LanguageCode, pt.Slug, newSlug)
			}
		}
	}

	log.Println("Done cleaning slugs!")
}
