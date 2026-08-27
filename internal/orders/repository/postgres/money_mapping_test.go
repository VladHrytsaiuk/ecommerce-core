package postgres

import "testing"

func TestMoneyFromRecordRejectsCorruptPersistedValue(t *testing.T) {
	if _, err := moneyFromRecord(100, "EU "); err == nil {
		t.Fatal("moneyFromRecord() accepted an invalid persisted currency")
	}
}
