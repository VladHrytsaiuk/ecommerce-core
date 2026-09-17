package money

import (
	"fmt"
	"math/big"
	"sort"
)

// AllocateLargestRemainder distributes a non-negative minor-unit amount over
// non-negative line weights. It uses exact integer arithmetic and stable input
// ordering as the tie breaker, so the allocated sum is always exactly total.
func AllocateLargestRemainder(total int64, weights []int64) ([]int64, error) {
	if total < 0 || len(weights) == 0 {
		return nil, fmt.Errorf("invalid allocation")
	}
	allocations := make([]int64, len(weights))
	if total == 0 {
		return allocations, nil
	}

	var denominator int64
	for _, weight := range weights {
		if weight < 0 || denominator > maxInt64-weight {
			return nil, fmt.Errorf("invalid allocation weights")
		}
		denominator += weight
	}
	if denominator == 0 || total > denominator {
		return nil, fmt.Errorf("allocation exceeds weights")
	}

	totalBig := big.NewInt(total)
	denominatorBig := big.NewInt(denominator)
	type remainder struct {
		index int
		value *big.Int
	}
	remainders := make([]remainder, 0, len(weights))
	var allocated int64
	for index, weight := range weights {
		product := new(big.Int).Mul(big.NewInt(weight), totalBig)
		quotient, rest := new(big.Int), new(big.Int)
		quotient.QuoRem(product, denominatorBig, rest)
		if !quotient.IsInt64() || quotient.Int64() < 0 || allocated > maxInt64-quotient.Int64() {
			return nil, fmt.Errorf("allocation overflow")
		}
		allocations[index] = quotient.Int64()
		allocated += quotient.Int64()
		remainders = append(remainders, remainder{index: index, value: rest})
	}

	sort.SliceStable(remainders, func(left, right int) bool {
		return remainders[left].value.Cmp(remainders[right].value) > 0
	})
	for index := int64(0); index < total-allocated; index++ {
		allocations[remainders[index].index]++
	}
	return allocations, nil
}

const maxInt64 = int64(^uint64(0) >> 1)
