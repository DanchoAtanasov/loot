package main

import (
	"DanchoAtanasov/loot/items"
	"fmt"
	"math/rand/v2"
	"sort"
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
		// fmt.Printf("Rolling %d with %d\n", tier, distribution)

		roll = rand.IntN(100)
		if roll < distribution {
			return tier
		}
	}
	panic("We shouldn't be here")
}

func choose(pool []items.Weapon) items.Weapon {
	// Use best score random selection algorithm
	bestScore := -1.0
	var choice items.Weapon
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

func dropItem(distributions map[items.ItemTier]int) items.Weapon {
	// total := 100
	roll := rollTier(distributions)
	// fmt.Printf("Rolled '%d' with chance %.3f\n", roll, float64(distributions[roll])/float64(total))

	var pool []items.Weapon
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

	item := choose(pool)

	stat, exist := items.Stats[item]
	if exist {
		item.Damage = rand.IntN(stat.MaxDamage-stat.MinDamage+1) + stat.MinDamage
	}

	return item

}

func main() {
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

	// simulations := 1_000_000
	simulations := 100
	results := make(map[items.Weapon]int)
	for i := 0; i < simulations; i++ {
		item := dropItem(distributions)
		fmt.Println(item)
		results[item]++
	}
	// fmt.Println(results)

	// for k, v := range results {
	// 	fmt.Printf("%s -> %d, (%d%%)\n", k.Name, v, int((float64(v)/float64(simulations))*100))
	// }

	keys := make([]items.Weapon, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name < keys[j].Name
	})

	for _, k := range keys {
		v := results[k]
		fmt.Printf("%s -> %d, (%.1f%%)\n",
			k.Name, v, (float64(v)/float64(simulations))*100)
	}

	return
}
