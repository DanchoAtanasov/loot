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

func Drop(items []Item) Item {
	return items[rand.IntN(len(items))]
}

func main() {
	fmt.Println("Main")

	distributions := map[ItemTier]int{
		NormalTier: 10,
		MagicTier:  5,
		RareTier:   2,
		UniqueTier: 1,
	}
	total := 0
	for _, count := range distributions {
		total += count
	}

	var items []Item = make([]Item, 0)
	items = Populate(items, distributions)
	fmt.Println(len(items))
	drop := Drop(items)
	fmt.Println(drop)
	fmt.Printf("chance %.3f\n", float64(distributions[drop.tier])/float64(total))
	return
}
