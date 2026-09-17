package money

import "testing"

func TestAllocateLargestRemainderIsExactAndStable(t *testing.T) {
	allocated, err := AllocateLargestRemainder(100, []int64{101, 100, 99})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{34, 33, 33}
	var total int64
	for index := range want {
		if allocated[index] != want[index] {
			t.Fatalf("allocation = %v, want %v", allocated, want)
		}
		total += allocated[index]
	}
	if total != 100 {
		t.Fatalf("allocation total = %d, want 100", total)
	}
}

func TestAllocateLargestRemainderRejectsOverAllocation(t *testing.T) {
	if _, err := AllocateLargestRemainder(4, []int64{1, 2}); err == nil {
		t.Fatal("AllocateLargestRemainder() error = nil, want error")
	}
}
