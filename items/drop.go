package items

import (
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
	roll := rollTier(Distributions)
	item := choose(NormalPool)

	stat, exist := Stats[item]
	if exist {
		item.Damage = rand.IntN(stat.MaxDamage-stat.MinDamage+1) + stat.MinDamage
		item.AttackSpeed = stat.AttackSpeed
	}
	for range bonusCount(roll) {
		if rand.IntN(2) == 0 {
			item.Damage++
		} else {
			item.AttackSpeed += 0.2
		}
	}
	item.Tier = roll
	item.Name = tierPrefix(roll) + item.Name
	return item

}

func DropArmour() Armour {
	tier := rollTier(Distributions)
	pool := []Armour{LeatherVest, Chainmail, LeatherCap, IronHelm, TravelBelt, WarBelt, WoodenShield, IronShield}
	armour := pool[rand.IntN(len(pool))]
	if stats, exists := ArmourStats[armour]; exists {
		armour.Defence = stats.Defence
	}
	for range bonusCount(tier) {
		if rand.IntN(2) == 0 {
			armour.Defence++
		} else {
			armour.Health += 2
		}
	}
	armour.Tier = tier
	armour.Name = tierPrefix(tier) + armour.Name
	return armour
}

func bonusCount(tier ItemTier) int { return int(tier) }

func tierPrefix(tier ItemTier) string {
	switch tier {
	case MagicTier:
		return "Magic "
	case RareTier:
		return "Rare "
	case UniqueTier:
		return "Unique "
	default:
		return ""
	}
}

var Distributions = map[ItemTier]int{
	UniqueTier: 2,
	RareTier:   8,
	MagicTier:  20,
	NormalTier: 70,
}

func init() {
	total := 0
	for _, count := range Distributions {
		total += count
	}
	if total != 100 {
		panic("Total isn't 100")
	}
}
