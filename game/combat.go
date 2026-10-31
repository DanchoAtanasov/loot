package game

import (
	"math"
	"time"

	"DanchoAtanasov/loot/items"
)

func resolveBattle(hero *player, foes []enemy, now time.Time) (int, []int) {
	if hero.Health <= 0 || now.Sub(hero.lastHit) < attackInterval(hero.weapon) {
		return -1, nil
	}
	attackTarget := -1
	nearestTargetDistance := int(^uint(0) >> 1)
	hitAny := false
	incomingDamage := 0
	defeated := make([]int, 0)
	for i := range foes {
		foe := &foes[i]
		if foe.Health <= 0 || !heroCanHitEnemyAOE(*foe, hero.x, hero.y) {
			continue
		}
		hitAny = true
		foeX, foeY := foe.x, foe.y
		if foe.tank {
			foeX++
			foeY++
		}
		distance := abs(foeX-hero.x)/2 + abs(foeY-hero.y)
		if distance < nearestTargetDistance {
			attackTarget, nearestTargetDistance = i, distance
		}
		foeEffectiveDamage := int(math.Ceil(
			float64(foe.Damage) * (10.0 / (10.0 + float64(hero.Armour))),
		))
		incomingDamage += max(foeEffectiveDamage, 0)
		foe.Health -= effectiveDamage(*hero)
		if foe.Health <= 0 {
			hero.Experience += foe.ExperienceOnKill
			defeated = append(defeated, i)
		}
	}
	if !hitAny {
		return -1, nil
	}
	hero.lastHit = now
	hero.Health = max(hero.Health-incomingDamage, 0)
	return attackTarget, defeated
}

func attackInterval(weapon items.Weapon) time.Duration {
	attacksPerSecond := weapon.AttackSpeed
	if attacksPerSecond <= 0 {
		attacksPerSecond = 2 // unarmed cadence matches the original 500 ms interval
	}
	return time.Duration(float64(time.Second) / attacksPerSecond)
}

func enemyTouchesPlayer(foe enemy, playerX, playerY int) bool {
	maxX, maxY := foe.x, foe.y
	if foe.tank {
		maxX++
		maxY++
	}
	return playerX >= foe.x-2 && playerX <= maxX+2 && playerY >= foe.y-1 && playerY <= maxY+1
}

func heroCanHitEnemy(foe enemy, playerX, playerY int) bool {
	maxX, maxY := foe.x, foe.y
	if foe.tank {
		maxX++
		maxY++
	}
	return playerX >= foe.x-3 && playerX <= maxX+3 && playerY >= foe.y-2 && playerY <= maxY+2
}

func heroCanHitEnemyAOE(foe enemy, playerX, playerY int) bool {
	maxX, maxY := foe.x, foe.y
	if foe.tank {
		maxX++
		maxY++
	}
	return playerX >= foe.x-heroAttackAOEHorizontalRange && playerX <= maxX+heroAttackAOEHorizontalRange &&
		playerY >= foe.y-heroAttackAOEVerticalRange && playerY <= maxY+heroAttackAOEVerticalRange
}

func facingEnemy(hero player, foe enemy) byte {
	enemyX, enemyY := foe.x, foe.y
	if foe.tank {
		enemyX++
		enemyY++
	}
	dx, dy := enemyX-hero.x, enemyY-hero.y
	switch {
	case dx < 0 && dy < 0:
		return 'q'
	case dx > 0 && dy < 0:
		return 'e'
	case dx < 0 && dy > 0:
		return 'z'
	case dx > 0 && dy > 0:
		return 'c'
	case dx < 0:
		return 'a'
	case dx > 0:
		return 'd'
	case dy < 0:
		return 'w'
	case dy > 0:
		return 's'
	default:
		return hero.facing
	}
}
