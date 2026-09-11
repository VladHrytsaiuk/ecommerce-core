package app

import (
	"os"
	"regexp"
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
	modules := NewModuleSet([]string{
		"admin", "notifications", "consent", "checkout", "inventory", "orders",
		"support", "reports", "media", "video", "abandoned_cart",
		"availability_notifications", "returns",
	})
	if err := modules.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want a fully satisfied set to pass", err)
	}
}

func TestModuleSetIgnoresDependenciesOfDisabledModules(t *testing.T) {
	// admin alone must not drag in the modules that depend on it.
	if err := NewModuleSet([]string{"admin"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want no requirement from a disabled dependent", err)
	}
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
