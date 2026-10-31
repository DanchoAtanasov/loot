package models

type EnemyRarity int

const (
	NormalEnemyRarity EnemyRarity = iota
	MagicEnemyRarity
	UniqueEnemyRarity
)

type Enemy struct {
	Health           int
	Damage           int
	ExperienceOnKill int
	Rarity           EnemyRarity
}

var DefaultEnemy = Enemy{
	Health:           1,
	Damage:           1,
	ExperienceOnKill: 10,
	Rarity:           NormalEnemyRarity,
}

var BruteEnemy = Enemy{
	Health:           2,
	Damage:           2,
	ExperienceOnKill: 15,
	Rarity:           MagicEnemyRarity,
}

var TankEnemy = Enemy{
	Health:           10,
	Damage:           1,
	ExperienceOnKill: 150,
	Rarity:           UniqueEnemyRarity,
}

func (e Enemy) DropChance() int {
	switch e.Rarity {
	case MagicEnemyRarity:
		return 50
	case UniqueEnemyRarity:
		return 100
	default:
		return 30
	}
}
