package main

import (
	"fmt"
	"math/rand/v2"
)

type ItemTier int

const (
	NormalTier ItemTier = iota
	MagicTier
	RareTier
	UniqueTier
)

type Item struct {
	name   string
	damage int
	tier   ItemTier
}

func Populate(items []Item, distributions map[ItemTier]int) []Item {
	i := 0
	for tier, number := range distributions {
		for j := 0; j < number; j++ {
			items = append(items, Item{name: fmt.Sprintf("%d name", i), damage: 1, tier: tier})
		}
		i++
	}
	return items
}

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
		fmt.Printf("Rolling %d with %d\n", tier, distribution)

		roll = rand.IntN(100)
		if roll < distribution {
			return tier
		}
	}
	panic("We shouldn't be here")
}

func main() {
	fmt.Println("Main")

	distributions := map[ItemTier]int{
		UniqueTier: 2,
		RareTier:   8,
		MagicTier:  20,
		NormalTier: 70,
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
	// drop := Drop(items)
	// fmt.Println(drop)
	// fmt.Printf("chance %.3f\n", float64(distributions[drop.tier])/float64(total))
	return
}
