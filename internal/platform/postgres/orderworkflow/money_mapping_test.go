package orderworkflow

import "testing"

func TestMoneyFromRecordRejectsCorruptPersistedValue(t *testing.T) {
	if _, err := moneyFromRecord(-1, "EUR"); err == nil {
		t.Fatal("moneyFromRecord() accepted a negative persisted amount")
	}
}
