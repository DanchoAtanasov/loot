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
	Tier     ItemTier
	Disabled bool
}

type Weapon struct {
	Item
	Damage      int
	AttackSpeed float64 // attacks per second
}

type weaponStats struct {
	MinDamage   int
	MaxDamage   int
	AttackSpeed float64
}

var Stats = map[Weapon]weaponStats{
	// Normal
	Sword:  {MinDamage: 1, MaxDamage: 2, AttackSpeed: 1.5},
	Bow:    {MinDamage: 1, MaxDamage: 2, AttackSpeed: 1.0},
	Spear:  {MinDamage: 1, MaxDamage: 2, AttackSpeed: 1.2},
	Dagger: {MinDamage: 1, MaxDamage: 2, AttackSpeed: 2.0},
	// Magic
	EnchantedSword: {MinDamage: 2, MaxDamage: 4, AttackSpeed: 1.5},
	FlamingBow:     {MinDamage: 2, MaxDamage: 4, AttackSpeed: 1.0},
	// Rare
	CursedSword: {MinDamage: 3, MaxDamage: 6, AttackSpeed: 1.5},
	// Unique
	Excalibur: {MinDamage: 10, MaxDamage: 15, AttackSpeed: 1.7},
	Trident:   {MinDamage: 10, MaxDamage: 13, AttackSpeed: 1.2},
}

// Instances
var Sword = Weapon{
	Name: "Sword",
	Tier: NormalTier,
}

var Bow = Weapon{
	Name: "Bow",
	Tier: NormalTier,
}

var Spear = Weapon{
	Name: "Spear",
	Tier: NormalTier,
}

var Dagger = Weapon{
	Name: "Dagger",
	Tier: NormalTier,
}

var NormalPool = []Weapon{
	Sword, Bow, Spear, Dagger,
}

// Magic
var EnchantedSword = Weapon{
	Name: "EnchantedSword",
	Tier: MagicTier,
}

var FlamingBow = Weapon{
	Name: "FlamingBow",
	Tier: MagicTier,
}

var MagicPool = []Weapon{
	EnchantedSword, FlamingBow,
}

// Rare
var CursedSword = Weapon{
	Name: "CursedSword",
	Tier: RareTier,
}

var RarePool = []Weapon{
	CursedSword,
}

// Unique
var Excalibur = Weapon{
	Name: "Excalibur",
	Tier: UniqueTier,
}

var Trident = Weapon{
	Name: "Trident",
	Tier: UniqueTier,
}

var UniquePool = []Weapon{
	Excalibur, Trident,
}
