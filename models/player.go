package models

type Player struct {
	Health        int
	MaxHealth     int
	Damage        int
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
	MaxHealth:     10,
	Damage:        1,
	Level:         1,
	MovementSpeed: 1,
	Experience:    0,
}

var LevelThreshold []int = []int{
	0, 20, 40, 300, 400, 500,
}
