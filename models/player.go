package models

type Player struct {
	Health        int
	MaxHealth     int
	Damage        int
	Level         int
	MovementSpeed int
	Experience    int
	Armour        int
}

type StatGrowth struct {
	MaxHealth int
	Damage    int
}

var DefaultPlayer = Player{
	Health:        10,
	MaxHealth:     10,
	Damage:        1,
	Level:         1,
	MovementSpeed: 2,
	Experience:    0,
	Armour:        0,
}

var LevelThreshold []int = []int{
	0, 50, 100, 200, 300, 400, 500, 700, 900, 1200,
}

var PlayerStatGrowth StatGrowth = StatGrowth{
	MaxHealth: 2,
	Damage:    1,
}
