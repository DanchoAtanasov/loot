package models

type Player struct {
	Health        int
	MaxHealth     int
	Damage        int
	Level         int
	MovementSpeed int
	Experience    int
	Armour        int
	AttackSpeed   float64 // bonus attacks per second gained from progression
}

type StatGrowth struct {
	MaxHealth   int
	Damage      int
	AttackSpeed float64
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

// ExperienceForLevel returns the cumulative experience needed to reach level.
// The quadratic curve preserves the first three thresholds and continues
// beyond the old fixed progression table.
func ExperienceForLevel(level int) int {
	if level <= 1 {
		return 50
	}
	maxInt := int(^uint(0) >> 1)
	n := int64(level)
	if n > int64(maxInt-50)/25/(n-1) {
		return maxInt
	}
	return int(25*n*(n-1) + 50)
}

var PlayerStatGrowth StatGrowth = StatGrowth{
	MaxHealth:   2,
	Damage:      1,
	AttackSpeed: 0.1,
}
