package postgres

import (
	"encoding/json"
	"testing"
)

func TestJSONBValueAndScanPreserveJSONObject(t *testing.T) {
	original := jsonb(`{"birth_date":"1990-01-31","subscribed":true}`)
	value, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}
	var scanned jsonb
	if err := scanned.Scan(value); err != nil {
		t.Fatal(err)
	}
	var attributes map[string]json.RawMessage
	if err := json.Unmarshal(scanned, &attributes); err != nil {
		t.Fatal(err)
	}
	if string(attributes["birth_date"]) != `"1990-01-31"` || string(attributes["subscribed"]) != "true" {
		t.Fatalf("attributes = %s", scanned)
	}
}

func TestJSONBRejectsInvalidValue(t *testing.T) {
	if _, err := jsonb(`not-json`).Value(); err == nil {
		t.Fatal("Value() error = nil, want invalid JSON")
	}
}
