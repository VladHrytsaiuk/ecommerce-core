package app

import (
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
		"admin", "notifications", "consent", "checkout",
		"support", "reports", "media", "video", "abandoned_cart", "availability_notifications",
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
