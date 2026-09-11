package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryGuardedPermissionIsSeeded closes a defect this repository has now
// shipped three times: a route guarded by a permission constant whose row no
// permissions migration ever inserts.
//
// Nothing catches it otherwise. The code compiles, the route registers, the
// authorizer runs — and denies, always, for every role including the super
// admin, because a permission with no row cannot be in anyone's granted set.
// The video admin API was unreachable for a whole phase this way, and the
// privacy and support admin surfaces for longer.
func TestEveryGuardedPermissionIsSeeded(t *testing.T) {
	root := repositoryRoot(t)
	declared := declaredPermissionCodes(t, filepath.Join(root, "internal"))
	if len(declared) == 0 {
		t.Fatal("no permission constants found; the pattern no longer matches how they are declared")
	}
	seeded := seededPermissionCodes(t, filepath.Join(root, "migrations"))

	for code, file := range declared {
		if _, present := seeded[code]; !present {
			t.Errorf("permission %q is checked by %s but no migration inserts it, so every check of it denies", code, file)
		}
	}
}

// declaredPermissionCodes finds the constants routes guard on. They are spread
// across facades and module delivery packages rather than gathered in one
// place, which is part of why the omissions were easy to miss.
func declaredPermissionCodes(t *testing.T, dir string) map[string]string {
	t.Helper()
	pattern := regexp.MustCompile(`Permission[A-Za-z]*\s*=\s*"([a-z_]+:[a-z_]+)"`)
	codes := make(map[string]string)
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
			codes[match[1]] = filepath.Base(path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return codes
}

// seededPermissionCodes reads only the INSERT INTO permissions statements.
//
// Scanning whole files would count a code named in a role_permissions grant as
// seeded, which is exactly the state that cannot work: granting a role a
// permission row that does not exist inserts nothing and denies everything.
func seededPermissionCodes(t *testing.T, dir string) map[string]struct{} {
	t.Helper()
	insert := regexp.MustCompile(`(?is)INSERT\s+INTO\s+permissions\b.*?;`)
	code := regexp.MustCompile(`'([a-z_]+:[a-z_]+)'`)
	codes := make(map[string]struct{})
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, statement := range insert.FindAllString(string(source), -1) {
			for _, match := range code.FindAllStringSubmatch(statement, -1) {
				codes[match[1]] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return codes
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// This package sits at internal/app.
	return filepath.Join(working, "..", "..")
}
