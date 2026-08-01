// Package storage надає інтерфейси для роботи зі сховищами файлів (хмарними або локальними).
package storage

import (
	"context"
)

// Storage — універсальний інтерфейс для завантаження та керування медіа-файлами.
type Storage interface {
	// Upload завантажує файл у вказану папку з заданим ім'ям.
	// Повертає публічний URL файлу або помилку.
	Upload(ctx context.Context, file interface{}, folder, filename string) (string, error)

	// Delete видаляє файл за його публічним ідентифікатором.
	Delete(ctx context.Context, publicID string) error
}
