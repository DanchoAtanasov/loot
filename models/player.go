package models

type Player struct {
	Health        int
	Level         int
	MovementSpeed int
	Experience    int
}

type StatGrowth struct {
	Health  int
	Damange int
}

var player = Player{
	Health:        10,
	Level:         1,
	MovementSpeed: 1,
	Experience:    0,
}
