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
	if err := runMigrations(".", cfg.DBURL, storeConfig.EnabledModules, direction); err != nil {
		log.Fatal(err)
	}
}

type modulePlan struct {
	dir   string
	table string
}

type migrationStep struct {
	dir       string
	table     string
	direction string
}

// runMigrations preserves foreign-key ownership: startup applies core before
// extensions, while rollback removes extensions before their core references.
func runMigrations(root, databaseURL string, enabledModules []string, direction string) error {
	steps, err := migrationSteps(root, enabledModules, direction)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := run(step.dir, databaseURL, step.table, step.direction); err != nil {
			return err
		}
	}
	return nil
}

func migrationSteps(root string, enabledModules []string, direction string) ([]migrationStep, error) {
	plans, err := modulePlans(enabledModules)
	if err != nil {
		return nil, err
	}
	core := migrationStep{dir: filepath.Join(root, "migrations", "core"), table: "schema_migrations", direction: direction}
	steps := make([]migrationStep, 0, len(plans)+1)
	if direction == "up" {
		steps = append(steps, core)
		for _, plan := range plans {
			steps = append(steps, migrationStep{dir: filepath.Join(root, plan.dir), table: plan.table, direction: direction})
		}
		return steps, nil
	}
	for index := len(plans) - 1; index >= 0; index-- {
		plan := plans[index]
		steps = append(steps, migrationStep{dir: filepath.Join(root, plan.dir), table: plan.table, direction: direction})
	}
	return append(steps, core), nil
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
