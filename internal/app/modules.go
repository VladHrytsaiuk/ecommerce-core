package app

import (
	"fmt"
	"sort"
	"strings"
)

// Module is an optional capability selected through ENABLED_MODULES. Using a
// named type keeps the vocabulary in one place: the flag was previously a bare
// string compared at more than twenty call sites, where a typo silently
// disabled a module instead of failing.
type Module string

const (
	ModuleAbandonedCart Module = "abandoned_cart"
	ModuleAdmin         Module = "admin"
	ModuleAvailability  Module = "availability_notifications"
	ModuleBadges        Module = "badges"
	ModuleCheckout      Module = "checkout"
	ModuleComparison    Module = "comparison"
	ModuleConsent       Module = "consent"
	ModuleCustomers     Module = "customers"
	ModuleInventory     Module = "inventory"
	ModuleMedia         Module = "media"
	ModuleNotifications Module = "notifications"
	ModuleOrders        Module = "orders"
	ModulePromos        Module = "promos"
	ModuleReports       Module = "reports"
	ModuleReturns       Module = "returns"
	ModuleReviews       Module = "reviews"
	ModuleSearch        Module = "search"
	ModuleSEO           Module = "seo"
	ModuleSupport       Module = "support"
	ModuleSync          Module = "sync"
	ModuleUserProfiles  Module = "user_profiles"
	ModuleVideo         Module = "video"
	ModuleWishlist      Module = "wishlist"
)

// moduleRequirements is the single declarative source of truth for
// module-to-module dependencies. These rules previously lived inline at the
// point each module happened to be constructed, so a misconfiguration was only
// reported once execution reached that spot — and only the first one found.
var moduleRequirements = map[Module][]Module{
	ModuleAvailability:  {ModuleNotifications},
	ModuleAbandonedCart: {ModuleCheckout, ModuleNotifications, ModuleConsent},
	ModuleSupport:       {ModuleNotifications, ModuleAdmin},
	ModuleReports:       {ModuleAdmin},
	ModuleMedia:         {ModuleAdmin},
	ModuleVideo:         {ModuleAdmin},
}

// requirementReasons explains why a dependency exists, so a failed boot tells
// an operator what to do rather than only what is missing.
var requirementReasons = map[Module]string{
	ModuleReports: "permission-gated dashboard routes",
	ModuleMedia:   "permission-gated uploads",
	ModuleVideo:   "permission-gated uploads",
}

// ModuleSet is the normalized set of enabled modules.
type ModuleSet map[Module]struct{}

// NewModuleSet normalizes raw ENABLED_MODULES values into a set.
func NewModuleSet(values []string) ModuleSet {
	set := make(ModuleSet, len(values))
	for _, value := range values {
		if normalized := Module(strings.ToLower(strings.TrimSpace(value))); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}

// Has reports whether a module is enabled.
func (m ModuleSet) Has(module Module) bool {
	_, enabled := m[module]
	return enabled
}

// Validate reports every unmet module dependency at once. Failing on the first
// one made fixing a multi-module misconfiguration an iterative guessing game.
func (m ModuleSet) Validate() error {
	dependents := make([]Module, 0, len(moduleRequirements))
	for module := range moduleRequirements {
		if m.Has(module) {
			dependents = append(dependents, module)
		}
	}
	sort.Slice(dependents, func(i, j int) bool { return dependents[i] < dependents[j] })

	problems := make([]string, 0, len(dependents))
	for _, module := range dependents {
		missing := make([]string, 0, len(moduleRequirements[module]))
		for _, required := range moduleRequirements[module] {
			if !m.Has(required) {
				missing = append(missing, string(required))
			}
		}
		if len(missing) == 0 {
			continue
		}
		problem := fmt.Sprintf("%s requires %s", module, strings.Join(missing, ", "))
		if reason := requirementReasons[module]; reason != "" {
			problem += " for " + reason
		}
		problems = append(problems, problem)
	}
	if len(problems) > 0 {
		return fmt.Errorf("ENABLED_MODULES is inconsistent: %s", strings.Join(problems, "; "))
	}
	return nil
}
