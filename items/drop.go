package items

import (
	"fmt"
	"math/rand/v2"
)

var tierOrder = []ItemTier{
	UniqueTier,
	RareTier,
	MagicTier,
	NormalTier,
}

func rollTier(distributions map[ItemTier]int) ItemTier {
	// Use cascading rolls
	var roll int
	for _, tier := range tierOrder {
		if tier == NormalTier {
			return NormalTier
		}

		distribution := distributions[tier]
		// fmt.Printf("Rolling %d with %d\n", tier, distribution)

		roll = rand.IntN(100)
		if roll < distribution {
			return tier
		}
	}
	panic("We shouldn't be here")
}

func choose(pool []Weapon) Weapon {
	// Use best score random selection algorithm
	bestScore := -1.0
	var choice Weapon
	for _, item := range pool {
		if item.Disabled {
			continue
		}
		score := rand.Float64()
		if score > bestScore {
			bestScore = score
			choice = item
		}
	}
	return choice
}

func DropItem() Weapon {
	// total := 100
	roll := rollTier(Distributions)
	// fmt.Printf("Rolled '%d' with chance %.3f\n", roll, float64(distributions[roll])/float64(total))

	var pool []Weapon
	switch roll {
	case NormalTier:
		pool = NormalPool
	case MagicTier:
		pool = MagicPool
	case RareTier:
		pool = RarePool
	case UniqueTier:
		pool = UniquePool
	}

	item := choose(pool)

	stat, exist := Stats[item]
	if exist {
		item.Damage = rand.IntN(stat.MaxDamage-stat.MinDamage+1) + stat.MinDamage
	}

	return item

}

var Distributions = map[ItemTier]int{
	UniqueTier: 2,
	RareTier:   8,
	MagicTier:  20,
	NormalTier: 70,
}

func init() {
	fmt.Println("Checking ")
	total := 0
	for _, count := range Distributions {
		total += count
	}
	if total != 100 {
		panic("Total isn't 100")
	}
}
