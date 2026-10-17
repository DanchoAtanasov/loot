package items

type ItemTier int

const (
	NormalTier ItemTier = iota
	MagicTier
	RareTier
	UniqueTier
)

type Item struct {
	Name     string
	Damage   int
	Tier     ItemTier
	Disabled bool
}

type weaponStats struct {
	MinDamage int
	MaxDamage int
}

var Stats = map[Item]weaponStats{
	// Normal
	Sword:  {MinDamage: 1, MaxDamage: 2},
	Bow:    {MinDamage: 1, MaxDamage: 2},
	Spear:  {MinDamage: 1, MaxDamage: 2},
	Dagger: {MinDamage: 1, MaxDamage: 2},
	// Magic
	EnchantedSword: {MinDamage: 2, MaxDamage: 4},
	FlamingBow:     {MinDamage: 2, MaxDamage: 4},
	// Rare
	CursedSword: {MinDamage: 3, MaxDamage: 6},
	// Unique
	Excalibur: {MinDamage: 10, MaxDamage: 15},
	Trident:   {MinDamage: 10, MaxDamage: 13},
}

// Instances
var Sword = Item{
	Name: "Sword",
	Tier: NormalTier,
}

var Bow = Item{
	Name: "Bow",
	Tier: NormalTier,
}

var Spear = Item{
	Name: "Spear",
	Tier: NormalTier,
}

var Dagger = Item{
	Name: "Dagger",
	Tier: NormalTier,
}

var NormalPool = []Item{
	Sword, Bow, Spear, Dagger,
}

// Magic
var EnchantedSword = Item{
	Name: "EnchantedSword",
	Tier: MagicTier,
}

var FlamingBow = Item{
	Name: "FlamingBow",
	Tier: MagicTier,
}

var MagicPool = []Item{
	EnchantedSword, FlamingBow,
}

// Rare
var CursedSword = Item{
	Name: "CursedSword",
	Tier: RareTier,
}

var RarePool = []Item{
	CursedSword,
}

// Unique
var Excalibur = Item{
	Name: "Excalibur",
	Tier: UniqueTier,
}

var Trident = Item{
	Name: "Trident",
	Tier: UniqueTier,
}

var UniquePool = []Item{
	Excalibur, Trident,
}
