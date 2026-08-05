// Command cli contains maintenance commands that must not be exposed over HTTP.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("load .env: %v", err)
	}
	if len(os.Args) < 2 || os.Args[1] != "create-owner" {
		log.Fatal("usage: go run ./cmd/cli create-owner -email owner@example.com [-password 'strong password']")
	}

	flags := flag.NewFlagSet("create-owner", flag.ExitOnError)
	emailFlag := flags.String("email", os.Getenv("OWNER_EMAIL"), "owner email (or OWNER_EMAIL)")
	passwordFlag := flags.String("password", os.Getenv("OWNER_PASSWORD"), "owner password (or OWNER_PASSWORD)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	databaseURL := strings.TrimSpace(os.Getenv("DB_URL"))
	email := strings.ToLower(strings.TrimSpace(*emailFlag))
	plainPassword := *passwordFlag
	if databaseURL == "" {
		log.Fatal("DB_URL is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		log.Fatalf("invalid owner email: %v", err)
	}
	if len(plainPassword) < 12 || len(plainPassword) > 72 {
		log.Fatal("owner password must contain 12 to 72 bytes")
	}

	hash, err := password.HashPassword(plainPassword)
	if err != nil {
		log.Fatalf("hash owner password: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: databaseURL, PreferSimpleProtocol: true}), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect PostgreSQL: %v", err)
	}

	owner := ownerRecord{ID: uuid.New(), Email: email, PasswordHash: hash, Role: "owner", Status: "active"}
	result := db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "email"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("email IS NOT NULL")}},
		DoNothing:   true,
	}).Create(&owner)
	if result.Error != nil {
		log.Fatalf("create owner: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		log.Fatalf("owner %q already exists; refusing to overwrite credentials", email)
	}
	fmt.Printf("owner created: %s (%s)\n", owner.Email, owner.ID)
}

type ownerRecord struct {
	ID           uuid.UUID `gorm:"column:id"`
	Email        string    `gorm:"column:email"`
	PasswordHash string    `gorm:"column:password_hash"`
	Role         string    `gorm:"column:role"`
	Status       string    `gorm:"column:status"`
}

func (ownerRecord) TableName() string { return "users" }
