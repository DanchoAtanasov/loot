package models

type Enemy struct {
	Health           int
	Damage           int
	ExperienceOnKill int
}

var DefaultEnemy = Enemy{
	Health:           1,
	Damage:           1,
	ExperienceOnKill: 10,
}
