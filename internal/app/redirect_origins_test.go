package app

import (
	"slices"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

func TestAProductionStoreNeverReturnsABuyerToTheirOwnMachine(t *testing.T) {
	// FRONTEND_URL defaults to http://localhost:3000; a production store that
	// never set it would have accepted that as a return address.
	cfg := &config.Config{
		Env:              "production",
		FrontendURL:      "http://localhost:3000",
		CORSAllowOrigins: []string{"https://shop.example", "http://127.0.0.1:8080", "http://[::1]:3000", "http://app.localhost"},
	}
	if got, want := redirectOriginsFor(cfg), []string{"https://shop.example"}; !slices.Equal(got, want) {
		t.Fatalf("redirectOriginsFor() = %v, want %v", got, want)
	}
}

func TestDevelopmentKeepsTheLocalStorefront(t *testing.T) {
	cfg := &config.Config{Env: "development", FrontendURL: "http://localhost:3000", CORSAllowOrigins: []string{"https://shop.example"}}
	if got, want := redirectOriginsFor(cfg), []string{"http://localhost:3000", "https://shop.example"}; !slices.Equal(got, want) {
		t.Fatalf("redirectOriginsFor() = %v, want %v", got, want)
	}
}

func TestBuildingTheOriginsDoesNotAppendToTheConfiguredSlice(t *testing.T) {
	origins := make([]string, 1, 4)
	origins[0] = "https://shop.example"
	cfg := &config.Config{Env: "production", FrontendURL: "https://front.example", CORSAllowOrigins: origins}
	redirectOriginsFor(cfg)
	if len(cfg.CORSAllowOrigins) != 1 || cfg.CORSAllowOrigins[:2][1] != "" {
		t.Fatalf("CORSAllowOrigins was modified: %v", cfg.CORSAllowOrigins[:2])
	}
}
