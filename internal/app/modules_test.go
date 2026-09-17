package app

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestModuleSetNormalizesRawValues(t *testing.T) {
	modules := NewModuleSet([]string{"  Admin ", "REPORTS", "", "   "})

	if !modules.Has(ModuleAdmin) || !modules.Has(ModuleReports) {
		t.Fatalf("modules = %v, want admin and reports normalized", modules)
	}
	if len(modules) != 2 {
		t.Fatalf("modules = %d entries, want blanks discarded", len(modules))
	}
}

func TestModuleSetReportsEveryUnmetDependencyAtOnce(t *testing.T) {
	// These rules used to be checked inline wherever each module happened to be
	// constructed, so an operator fixed one, rebooted, and found the next.
	modules := NewModuleSet([]string{"support", "reports", "media", "abandoned_cart"})

	err := modules.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want the unmet dependencies reported")
	}
	for _, expected := range []string{"support requires", "reports requires admin", "media requires admin", "abandoned_cart requires"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("Validate() error = %q, want it to mention %q", err, expected)
		}
	}
}

func TestModuleSetExplainsWhyADependencyExists(t *testing.T) {
	err := NewModuleSet([]string{"video"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "permission-gated uploads") {
		t.Fatalf("Validate() error = %v, want the reason for the admin requirement", err)
	}
}

func TestModuleSetAcceptsASatisfiedConfiguration(t *testing.T) {
	modules := NewModuleSet(withRequiredModules(
		"admin", "notifications", "consent", "checkout", "orders",
		"support", "reports", "media", "video", "abandoned_cart",
		"availability_notifications", "returns",
	))
	if err := modules.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want a fully satisfied set to pass", err)
	}
}

func TestModuleSetIgnoresDependenciesOfDisabledModules(t *testing.T) {
	// admin alone must not drag in the modules that depend on it.
	if err := NewModuleSet(withRequiredModules("admin")).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want no requirement from a disabled dependent", err)
	}
}

func TestModuleSetReportsEveryMissingMandatoryModule(t *testing.T) {
	// Both are reported together, and each says why it is not optional. An
	// operator who left them both out used to be told about one.
	err := NewModuleSet([]string{"admin"}).Validate()
	if err == nil {
		t.Fatal("Validate() accepted a store with no catalog and no inventory")
	}
	for _, expected := range []string{
		"catalog is required because every product read preloads product_media",
		"inventory is required because checkout reservations require it",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("Validate() error = %q, want it to mention %q", err, expected)
		}
	}
}

// withRequiredModules prepends the modules no store may omit, so a test about
// dependency rules is not also a test about the mandatory list.
func withRequiredModules(modules ...string) []string {
	required := make([]string, 0, len(requiredModules)+len(modules))
	for module := range requiredModules {
		required = append(required, string(module))
	}
	slices.Sort(required)
	return append(required, modules...)
}

func TestModuleSetCoversEveryDependencyThatUsedToBeInline(t *testing.T) {
	// These rules were previously scattered across NewStoreConfig, Validate and
	// Bootstrap. Pinning them here is what stops one from being dropped during
	// a later move without anything noticing.
	for module, required := range map[string][]string{
		"availability_notifications": {"inventory", "notifications"},
		"abandoned_cart":             {"checkout", "notifications", "consent"},
		"support":                    {"notifications", "admin"},
		"returns":                    {"orders", "admin"},
		"reports":                    {"admin"},
		"media":                      {"admin"},
		"video":                      {"admin"},
	} {
		t.Run(module, func(t *testing.T) {
			for _, dependency := range required {
				err := NewModuleSet([]string{module}).Validate()
				if err == nil || !strings.Contains(err.Error(), dependency) {
					t.Fatalf("Validate() error = %v, want %s to require %s", err, module, dependency)
				}
			}
		})
	}
}

func TestModuleSetRejectsAnUnknownModule(t *testing.T) {
	// The typed vocabulary exists to make a misspelling fail at startup.
	// Validate previously only walked moduleRequirements, so a name absent
	// from that map was never examined: ENABLED_MODULES=serach booted a
	// service with search silently switched off.
	err := NewModuleSet([]string{"admin", "serach"}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want a misspelled module to fail at startup")
	}
	if !strings.Contains(err.Error(), "serach") {
		t.Fatalf("Validate() error = %v, want the misspelled name reported", err)
	}
	// An operator who mistyped needs the spelling that would have worked.
	if !strings.Contains(err.Error(), "search") {
		t.Fatalf("Validate() error = %v, want the valid module names listed", err)
	}
}

func TestModuleSetAcceptsEveryModuleItImplements(t *testing.T) {
	// Guards the other direction: the unknown-name check must not reject a
	// module the core actually has. A dependency error here is fine — this
	// asserts only that no name is reported as unrecognized.
	for _, module := range allModules {
		if err := NewModuleSet([]string{string(module)}).Validate(); err != nil && strings.Contains(err.Error(), "unknown module") {
			t.Fatalf("Validate() rejected %q as unknown", module)
		}
	}
}

func TestAllModulesListsEveryConstant(t *testing.T) {
	// allModules is maintained by hand next to the constants. If the two drift,
	// a valid configuration starts failing at startup, so the drift is caught
	// here rather than in a deployment.
	source, err := os.ReadFile("modules.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`(?m)^\tModule\w+\s+Module\s*=\s*"([^"]+)"`).FindAllStringSubmatch(string(source), -1)
	if len(declared) == 0 {
		t.Fatal("no Module constants found; the pattern no longer matches the declarations")
	}
	for _, match := range declared {
		if !Module(match[1]).known() {
			t.Fatalf("module constant %q is missing from allModules", match[1])
		}
	}
	if len(declared) != len(allModules) {
		t.Fatalf("allModules has %d entries for %d constants", len(allModules), len(declared))
	}
}

// Three places decide whether a module is enabled: this package's ModuleSet,
// the config package's guards, and the CLI. They disagreed — ModuleSet and the
// CLI folded case, config did not — so ENABLED_MODULES=Notifications built the
// module while the two guards that validate its configuration silently skipped.
//
// There is one definition now, config.NormalizeModules, and this calls it
// rather than restating its rules: a test that reimplements the thing it checks
// passes while the real code regresses.
func TestTheConfigAndModuleViewsAgreeOnWhatIsEnabled(t *testing.T) {
	for _, written := range [][]string{
		{"notifications", "abandoned_cart"},
		{"Notifications", "Abandoned_Cart"},
		{"NOTIFICATIONS", " ABANDONED_CART"},
		{"  notifications ", "  abandoned_cart  "},
	} {
		t.Run(strings.Join(written, ","), func(t *testing.T) {
			canonical := config.NormalizeModules(written)
			modules := NewModuleSet(written)

			for _, module := range []Module{ModuleNotifications, ModuleAbandonedCart} {
				if !modules.Has(module) {
					t.Fatalf("%q did not enable %s", written, module)
				}
				// The config guards compare canonical names exactly, so the
				// module being enabled has to imply its guards will run.
				if !slices.Contains(canonical, string(module)) {
					t.Fatalf("%q enabled %s but its configuration guards would not run", written, module)
				}
			}
		})
	}
}

// modulesWithoutSchema are the modules that deliberately own no tables. It is
// the only thing that may explain a module with no migrations directory, and it
// is short on purpose: search projects into Meilisearch, and checkout's tables
// arrived later under its own name.
var modulesWithoutSchema = map[Module]string{
	ModuleSearch: "projects into Meilisearch; owns no SQL",
}

func TestEveryModuleMigrationDirectoryIsAModuleNameEnabledModulesAccepts(t *testing.T) {
	// This is the invariant that was missing. migrations/modules/catalog
	// existed while "catalog" was not a module name at all, so ENABLED_MODULES
	// rejected it, cmd/migrate could never plan it, and the four tables it
	// creates — product_media among them — were created by no valid
	// configuration. Every product read then failed on a missing table.
	for _, entry := range moduleMigrationDirectories(t) {
		if !Module(entry).known() {
			t.Fatalf("migrations/modules/%s has no module name in ENABLED_MODULES, so nothing can ever apply it", entry)
		}
	}
}

func TestEveryModuleEitherOwnsSchemaOrSaysWhyItDoesNot(t *testing.T) {
	// The other direction. cmd/migrate skips a module with no migrations
	// directory, which is right for search and would silently hide the
	// accidental deletion of any other module's schema.
	directories := make(map[string]struct{})
	for _, entry := range moduleMigrationDirectories(t) {
		directories[entry] = struct{}{}
	}
	for _, module := range allModules {
		_, hasSchema := directories[string(module)]
		_, exempt := modulesWithoutSchema[module]
		if hasSchema && exempt {
			t.Fatalf("%s is listed as owning no schema but migrations/modules/%s exists", module, module)
		}
		if !hasSchema && !exempt {
			t.Fatalf("%s has no migrations directory and no entry in modulesWithoutSchema; cmd/migrate would skip it and the store would boot without its tables", module)
		}
	}
}

func moduleMigrationDirectories(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "migrations", "modules"))
	if err != nil {
		t.Fatalf("read module migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		t.Fatal("no module migration directories found; this test would assert nothing")
	}
	return names
}
