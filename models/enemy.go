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

var BruteEnemy = Enemy{
	Health:           2,
	Damage:           2,
	ExperienceOnKill: 15,
}

var TankEnemy = Enemy{
	Health:           10,
	Damage:           1,
	ExperienceOnKill: 150,
}
