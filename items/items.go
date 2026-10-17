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

// Instances
var Sword = Item{
	Name:   "Sword",
	Damage: 1,
	Tier:   NormalTier,
}

var Bow = Item{
	Name:   "Bow",
	Damage: 1,
	Tier:   NormalTier,
}

var Spear = Item{
	Name:   "Spear",
	Damage: 1,
	Tier:   NormalTier,
}

var Dagger = Item{
	Name:   "Dagger",
	Damage: 1,
	Tier:   NormalTier,
}

var NormalPool = []Item{
	Sword, Bow, Spear, Dagger,
}

// Magic
var EnchantedSword = Item{
	Name:   "EnchantedSword",
	Damage: 3,
	Tier:   MagicTier,
}

var FlamingBow = Item{
	Name:   "FlamingBow",
	Damage: 2,
	Tier:   MagicTier,
}

var MagicPool = []Item{
	EnchantedSword, FlamingBow,
}

// Rare
var CursedSword = Item{
	Name:   "CursedSword",
	Damage: 6,
	Tier:   RareTier,
}

var RarePool = []Item{
	CursedSword,
}

// Unique
var Excalibur = Item{
	Name:   "Excalibur",
	Damage: 10,
	Tier:   UniqueTier,
}

var Trident = Item{
	Name:   "Trident",
	Damage: 10,
	Tier:   UniqueTier,
}

var UniquePool = []Item{
	Excalibur, Trident,
}
