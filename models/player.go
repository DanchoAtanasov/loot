package models

type Player struct {
	Health        int
	MovementSpeed int
}

var player = Player{
	Health:        10,
	MovementSpeed: 1,
}
