package main

import (
	"DanchoAtanasov/loot/items"
	"fmt"
	"math/rand/v2"
)

var tierOrder = []items.ItemTier{
	items.UniqueTier,
	items.RareTier,
	items.MagicTier,
	items.NormalTier,
}

func rollTier(distributions map[items.ItemTier]int) items.ItemTier {
	// Use cascading rolls
	var roll int
	for _, tier := range tierOrder {
		if tier == items.NormalTier {
			return items.NormalTier
		}

		distribution := distributions[tier]
		fmt.Printf("Rolling %d with %d\n", tier, distribution)

		roll = rand.IntN(100)
		if roll < distribution {
			return tier
		}
	}
	panic("We shouldn't be here")
}

func choose(pool []items.Item) items.Item {
	// Use best score random selection algorithm
	bestScore := -1.0
	var choice items.Item
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

func main() {
	fmt.Println("Main")

	distributions := map[items.ItemTier]int{
		items.UniqueTier: 2,
		items.RareTier:   8,
		items.MagicTier:  20,
		items.NormalTier: 70,
	}
	total := 0
	for _, count := range distributions {
		total += count
	}
	if total != 100 {
		panic("Total isn't 100")
	}

	// var items []Item = make([]Item, 0)
	// items = Populate(items, distributions)
	// fmt.Println(len(items))
	roll := rollTier(distributions)
	fmt.Printf("Rolled '%d' with chance %.3f\n", roll, float64(distributions[roll])/float64(total))

	var item items.Item
	var pool []items.Item
	switch roll {
	case items.NormalTier:
		pool = items.NormalPool
	case items.MagicTier:
		pool = items.MagicPool
	case items.RareTier:
		pool = items.RarePool
	case items.UniqueTier:
		pool = items.UniquePool
	}

	item = choose(pool)
	// item = pool[rand.IntN(len(pool))]
	fmt.Println(item)
	// fmt.Printf("chance %.3f\n", float64(distributions[drop.tier])/float64(total))
	return
}
