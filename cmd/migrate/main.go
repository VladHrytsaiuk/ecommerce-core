package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	var dir string
	flag.StringVar(&dir, "dir", "migrations/core", "Directory with migration files")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("Please specify a command: 'up' or 'down'")
	}
	command := args[0]

	// Завантажуємо змінні оточення
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DB_URL")
	}
	if dbURL == "" {
		log.Fatal("DATABASE_URL or DB_URL must be set")
	}

	m, err := migrate.New(fmt.Sprintf("file://%s", dir), dbURL)
	if err != nil {
		log.Fatalf("could not create instance of migrate: %v", err)
	}

	switch command {
	case "up":
		steps := 0
		if len(args) > 1 {
			steps, err = strconv.Atoi(args[1])
			if err != nil {
				log.Fatalf("Invalid number of steps for 'up': %v", err)
			}
		}

		if steps > 0 {
			log.Printf("Applying %d migrations UP...\n", steps)
			err = m.Steps(steps)
		} else {
			log.Println("Applying ALL migrations UP...")
			err = m.Up()
		}

		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Migration UP failed: %v", err)
		}
		log.Println("Migrations UP completed successfully!")

	case "down":
		steps := 1 // Безпечне значення за замовчуванням (1 крок)
		isAll := false

		if len(args) > 1 {
			if args[1] == "all" {
				isAll = true
			} else {
				steps, err = strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("Invalid number of steps for 'down': %v", err)
				}
			}
		}

		if isAll {
			log.Println("Rolling back ALL migrations (m.Down)... WARNING!")
			err = m.Down()
		} else {
			log.Printf("Rolling back %d migrations DOWN...\n", steps)
			err = m.Steps(-steps) // Передаємо від'ємне значення для відкату
		}

		// Обробляємо специфічні помилки відсутності змін
		if err != nil && !errors.Is(err, migrate.ErrNoChange) && err.Error() != "no change" {
			log.Fatalf("Migration DOWN failed: %v", err)
		}
		log.Println("Rollback completed successfully!")

	case "force":
		if len(args) < 2 {
			log.Fatal("Please specify version for 'force' command")
		}
		version, err := strconv.Atoi(args[1])
		if err != nil {
			log.Fatalf("Invalid version for 'force': %v", err)
		}
		log.Printf("Forcing database version to %d...\n", version)
		err = m.Force(version)
		if err != nil {
			log.Fatalf("Force command failed: %v", err)
		}
		log.Println("Force command completed successfully!")

	default:
		log.Fatalf("Unknown command: %s. Expected 'up' or 'down'", command)
	}
}
