package main

import "testing"

func TestModulePlansAreSortedAndUseSeparateTables(t *testing.T) {
	plans, err := modulePlans([]string{"sync", "inventory"})
	if err != nil {
		t.Fatalf("modulePlans() error = %v", err)
	}
	if len(plans) != 2 || plans[0].dir != "migrations/modules/inventory" || plans[1].table != "schema_migrations_module_sync" {
		t.Fatalf("modulePlans() = %+v, want deterministic module plans", plans)
	}
}

func TestModulePlansRejectDuplicateModule(t *testing.T) {
	if _, err := modulePlans([]string{"inventory", "inventory"}); err == nil {
		t.Fatal("modulePlans() error = nil, want duplicate module error")
	}
}
