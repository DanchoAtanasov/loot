package items

type Armour struct {
	Item
	Defence int
	Slot    ArmourSlot
}

type ArmourSlot int

const (
	ChestSlot ArmourSlot = iota
	HelmetSlot
	BeltSlot
	OffHandSlot
)

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
	LeatherVest:  {Defence: 1},
	Chainmail:    {Defence: 2},
	LeatherCap:   {Defence: 1},
	IronHelm:     {Defence: 2},
	TravelBelt:   {Defence: 1},
	WarBelt:      {Defence: 2},
	WoodenShield: {Defence: 1},
	IronShield:   {Defence: 2},
}

// Instances
var LeatherVest = Armour{
	Name: "LeatherVest",
	Tier: NormalTier,
}

var Chainmail = Armour{
	Name: "Chainmail",
	Tier: NormalTier,
	Slot: ChestSlot,
}

var LeatherCap = Armour{Item: Item{Name: "LeatherCap", Tier: NormalTier}, Slot: HelmetSlot}
var IronHelm = Armour{Item: Item{Name: "IronHelm", Tier: NormalTier}, Slot: HelmetSlot}
var TravelBelt = Armour{Item: Item{Name: "TravelBelt", Tier: NormalTier}, Slot: BeltSlot}
var WarBelt = Armour{Item: Item{Name: "WarBelt", Tier: NormalTier}, Slot: BeltSlot}
var WoodenShield = Armour{Item: Item{Name: "WoodenShield", Tier: NormalTier}, Slot: OffHandSlot}
var IronShield = Armour{Item: Item{Name: "IronShield", Tier: NormalTier}, Slot: OffHandSlot}
