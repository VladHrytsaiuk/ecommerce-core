package app

import (
	"fmt"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
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

// allModules is every name ENABLED_MODULES accepts. It exists because
// moduleRequirements only lists modules that depend on another one, so it
// cannot answer "is this a real module?" — and a name absent from it was
// previously not examined at all.
//
// It must list every Module constant above; TestAllModulesListsEveryConstant
// reads this file and fails if one is missing, so adding a constant without
// adding it here cannot silently start rejecting a valid configuration.
var allModules = []Module{
	ModuleAbandonedCart, ModuleAdmin, ModuleAvailability, ModuleBadges,
	ModuleCheckout, ModuleComparison, ModuleConsent, ModuleCustomers,
	ModuleInventory, ModuleMedia, ModuleNotifications, ModuleOrders,
	ModulePromos, ModuleReports, ModuleReturns, ModuleReviews, ModuleSearch,
	ModuleSEO, ModuleSupport, ModuleSync, ModuleUserProfiles, ModuleVideo,
	ModuleWishlist,
}

// moduleRequirements is the single declarative source of truth for
// module-to-module dependencies. These rules previously lived inline at the
// point each module happened to be constructed, so a misconfiguration was only
// reported once execution reached that spot — and only the first one found.
var moduleRequirements = map[Module][]Module{
	ModuleAvailability:  {ModuleInventory, ModuleNotifications},
	ModuleAbandonedCart: {ModuleCheckout, ModuleNotifications, ModuleConsent},
	ModuleSupport:       {ModuleNotifications, ModuleAdmin},
	ModuleReturns:       {ModuleOrders, ModuleAdmin},
	ModuleReports:       {ModuleAdmin},
	ModuleMedia:         {ModuleAdmin},
	ModuleVideo:         {ModuleAdmin},
}

// requirementReasons explains why a dependency exists, so a failed boot tells
// an operator what to do rather than only what is missing.
var requirementReasons = map[Module]string{
	ModuleReturns: "audited refund authorization",
	ModuleReports: "permission-gated dashboard routes",
	ModuleMedia:   "permission-gated uploads",
	ModuleVideo:   "permission-gated uploads",
}

// ModuleSet is the normalized set of enabled modules.
type ModuleSet map[Module]struct{}

// NewModuleSet normalizes raw ENABLED_MODULES values into a set.
func NewModuleSet(values []string) ModuleSet {
	// config.NormalizeModules owns the canonical form. Restating it here is
	// what let this package and the config guards disagree.
	normalized := config.NormalizeModules(values)
	set := make(ModuleSet, len(normalized))
	for _, value := range normalized {
		set[Module(value)] = struct{}{}
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
	// An unrecognized name is checked first and on its own. ENABLED_MODULES=serach
	// used to pass validation and boot a service with search quietly switched
	// off, which is the exact failure the typed vocabulary was introduced to
	// prevent. Reporting it alongside dependency problems would be misleading:
	// a typo makes every rule that mentions that module unanswerable.
	unknown := make([]string, 0)
	for module := range m {
		if !module.known() {
			unknown = append(unknown, string(module))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("ENABLED_MODULES contains unknown module(s): %s; valid modules are %s",
			strings.Join(unknown, ", "), strings.Join(moduleNames(), ", "))
	}

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

// known reports whether this is a module the core implements.
func (m Module) known() bool {
	for _, module := range allModules {
		if module == m {
			return true
		}
	}
	return false
}

// moduleNames lists the valid modules for an error message. An operator who
// mistyped one needs to see the spelling that would have worked.
func moduleNames() []string {
	names := make([]string, 0, len(allModules))
	for _, module := range allModules {
		names = append(names, string(module))
	}
	sort.Strings(names)
	return names
}
