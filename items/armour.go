package items

type Armour struct {
	Item
	Defence int
}

type Helmet struct {
	Armour
}

type Chest struct {
	Armour
}

type Belt struct {
	Armour
}

// Shield, Another weapon, etc
type OffHand struct {
	Armour
}

type armourStats struct {
	Defence int
}

var ArmourStats = map[Armour]armourStats{
	// Normal
	LeatherVest: {Defence: 1},
	Chainmail:   {Defence: 2},
}

// Instances
var LeatherVest = Armour{
	Name: "LeatherVest",
	Tier: NormalTier,
}

var Chainmail = Armour{
	Name: "Chainmail",
	Tier: NormalTier,
}
