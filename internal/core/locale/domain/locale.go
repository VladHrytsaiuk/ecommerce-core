// Package domain defines locale state owned by the stable core schema.
package domain

import "context"

type Locale struct {
	Code      string `gorm:"primaryKey;type:varchar(10)" json:"code"`
	Name      string `gorm:"type:varchar(100);not null" json:"name"`
	IsDefault bool   `gorm:"not null" json:"is_default"`
	IsActive  bool   `gorm:"not null" json:"is_active"`
}

func (Locale) TableName() string { return "locales" }

// Repository is the persistence port for the configured catalog locale set.
type Repository interface {
	Synchronize(context.Context, []Locale) error
}
