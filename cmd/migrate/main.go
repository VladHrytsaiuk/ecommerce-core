package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"regexp"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

var moduleName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func main() {
	flag.Parse()
	if flag.NArg() != 1 || (flag.Arg(0) != "up" && flag.Arg(0) != "down") {
		log.Fatal("usage: go run ./cmd/migrate [up|down]")
	}
	cfg := config.Load()
	storeConfig, err := app.NewStoreConfig(cfg)
	if err != nil {
		log.Fatalf("invalid store configuration: %v", err)
	}

	direction := flag.Arg(0)
	if err := run("migrations/core", cfg.DBURL, "schema_migrations", direction); err != nil {
		log.Fatal(err)
	}
	for _, module := range storeConfig.EnabledModules {
		if !moduleName.MatchString(module) {
			log.Fatalf("invalid enabled module name %q", module)
		}
		path := filepath.Join("migrations", "modules", module)
		if err := run(path, cfg.DBURL, "schema_migrations_module_"+module, direction); err != nil {
			log.Fatal(err)
		}
	}
}

func run(dir, databaseURL, table, direction string) error {
	m, err := migrate.New("file://"+dir, migrationURL(databaseURL, table))
	if err != nil {
		return fmt.Errorf("create migration runner for %s: %w", dir, err)
	}
	defer func() { _, _ = m.Close() }()
	if direction == "up" {
		err = m.Up()
	} else {
		err = m.Down()
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply %s migrations in %s: %w", direction, dir, err)
	}
	return nil
}

func migrationURL(databaseURL, table string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		log.Fatalf("invalid DB_URL: %v", err)
	}
	query := u.Query()
	query.Set("x-migrations-table", table)
	u.RawQuery = query.Encode()
	return u.String()
}
