package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"

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
	plans, err := modulePlans(storeConfig.EnabledModules)
	if err != nil {
		log.Fatal(err)
	}
	for _, plan := range plans {
		if err := run(plan.dir, cfg.DBURL, plan.table, direction); err != nil {
			log.Fatal(err)
		}
	}
}

type modulePlan struct {
	dir   string
	table string
}

// modulePlans gives every enabled module an isolated migration history and a
// stable execution order independent of the order used in .env.
func modulePlans(enabledModules []string) ([]modulePlan, error) {
	modules := append([]string(nil), enabledModules...)
	sort.Strings(modules)
	plans := make([]modulePlan, 0, len(modules))
	for index, module := range modules {
		if !moduleName.MatchString(module) {
			return nil, fmt.Errorf("invalid enabled module name %q", module)
		}
		if index > 0 && modules[index-1] == module {
			return nil, fmt.Errorf("duplicate enabled module %q", module)
		}
		plans = append(plans, modulePlan{
			dir:   filepath.Join("migrations", "modules", module),
			table: "schema_migrations_module_" + module,
		})
	}
	return plans, nil
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
