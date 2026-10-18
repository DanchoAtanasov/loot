package models

type Player struct {
	Health        int
	Level         int
	MovementSpeed int
	Experience    int
}

type StatGrowth struct {
	Health int
	Damage int
}

var DefaultPlayer = Player{
	Health:        10,
	Level:         1,
	MovementSpeed: 1,
	Experience:    0,
}
