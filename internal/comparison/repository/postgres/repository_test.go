package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewestUniqueItemsKeepsNewestAcrossGuestAndUserLists(t *testing.T) {
	variantA, variantB, variantC := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	items := []itemRecord{
		{ID: uuid.New(), ProductVariantID: variantA, CreatedAt: now.Add(-4 * time.Minute)}, // old user copy
		{ID: uuid.New(), ProductVariantID: variantA, CreatedAt: now.Add(-1 * time.Minute)}, // newer guest copy
		{ID: uuid.New(), ProductVariantID: variantB, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: uuid.New(), ProductVariantID: variantC, CreatedAt: now.Add(-3 * time.Minute)},
	}

	kept := newestUniqueItems(items, 2)
	if len(kept) != 2 {
		t.Fatalf("kept items = %d, want 2", len(kept))
	}
	if kept[0].ProductVariantID != variantA || kept[1].ProductVariantID != variantB {
		t.Fatalf("kept variants = [%s %s], want newest unique A then B", kept[0].ProductVariantID, kept[1].ProductVariantID)
	}
	if !kept[0].CreatedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("duplicate variant kept at %s, want newest %s", kept[0].CreatedAt, now.Add(-time.Minute))
	}
}
