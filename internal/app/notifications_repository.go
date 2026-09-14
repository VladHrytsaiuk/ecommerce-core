package app

import (
	"fmt"

	"gorm.io/gorm"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
)

// newNotificationsRepository builds the one shape every caller needs: the
// store's locale, and the encryption a scheduled job needs so its recipient and
// rendered body are never a column anyone can read.
//
// It exists because there is no safe half-configured version. A repository
// without the cipher cannot write a scheduled job at all — ScheduleEmail
// refuses rather than falling back to plaintext — so building it in one place
// is what keeps that refusal from being something a caller can trip over.
func newNotificationsRepository(cfg *config.Config, storeConfig StoreConfig, db *gorm.DB) (*notificationsPostgres.Repository, error) {
	cipher, err := encryption.NewAESGCM(cfg.NotificationEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("configure notifications encryption: %w", err)
	}
	// Derived from the same key rather than a second secret: a leak of one must
	// not be a forgery of the other, and one key is one thing to rotate.
	dedupe, err := notificationsDomain.NewDedupeKeyer(cfg.NotificationEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("configure notifications dedupe key: %w", err)
	}
	return notificationsPostgres.NewRepository(db).
		WithDefaultLocale(storeConfig.DefaultLocale).
		WithEncryption(cipher, dedupe), nil
}
