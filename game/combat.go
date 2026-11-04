package game

import (
	"math"
	"time"

	"DanchoAtanasov/loot/items"
)

func resolveBattle(hero *player, foes []enemy, now time.Time) (int, []int) {
	if hero.Health <= 0 || now.Sub(hero.lastHit) < attackInterval(hero.weapon, hero.AttackSpeed) {
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

// dashDirection returns the world-space movement vector for the current facing.
func dashDirection(facing byte) (int, int) {
	dx, dy := 0, 0
	switch facing {
	case 'w':
		dy = -1
	case 's':
		dy = 1
	case 'a':
		dx = -1
	case 'd':
		dx = 1
	case 'q':
		dx, dy = -1, -1
	case 'e':
		dx, dy = 1, -1
	case 'z':
		dx, dy = -1, 1
	case 'c':
		dx, dy = 1, 1
	default:
		dy = 1
	}
	if dx != 0 {
		dx *= horizontalMovementScale()
	}
	return dx, dy
}

func advanceDash(hero *player, foes []enemy, width, height int) (int, int, int, []int) {
	x, y := hero.x, hero.y
	// Cover three world cells per movement tick, checking every cell on the
	// segment so enemies along the dash path are hit even at high speed.
	for step := 0; step < 3 && hero.dashRemaining > 0; step++ {
		nextX, nextY := keepPlayerInChamber(x+hero.dashDX, y+hero.dashDY, width, height)
		if nextX == x && nextY == y {
			hero.dashRemaining = 0
			break
		}
		x, y = nextX, nextY
		hero.dashRemaining--
		for i := range foes {
			if !foes[i].dashTagged && foes[i].Health > 0 && enemyTouchesPlayer(foes[i], x, y) {
				foes[i].dashTagged = true
				foes[i].Health -= effectiveDamage(*hero)
				if foes[i].Health <= 0 {
					hero.Experience += foes[i].ExperienceOnKill
				}
			}
		}
	}
	defeated := make([]int, 0)
	for i := range foes {
		if foes[i].dashTagged && foes[i].Health <= 0 {
			defeated = append(defeated, i)
		}
	}
	return x, y, hero.dashRemaining, defeated
}

func advanceFlyingSlashes(slashes []flyingSlash, foes []enemy, width, height int, hero *player) ([]flyingSlash, []int) {
	defeated := make([]int, 0)
	active := slashes[:0]
	for i := range slashes {
		slash := slashes[i]
		for step := 0; step < 3 && slash.remaining > 0; step++ {
			slash.distance++
			slash.frame++
			wave := [...]int{0, 1, 0, -1}[slash.frame%4]
			perpX, perpY := -sign(slash.dy), sign(slash.dx)
			nextX := slash.startX + sign(slash.dx)*slash.distance + perpX*wave
			nextY := slash.startY + sign(slash.dy)*slash.distance + perpY*wave
			nextX, nextY = keepPlayerInChamber(nextX, nextY, width, height)
			if nextX == slash.x && nextY == slash.y {
				slash.remaining = 0
				break
			}
			slash.x, slash.y = nextX, nextY
			slash.remaining--
			for j := range foes {
				if !foes[j].dashTagged && foes[j].Health > 0 && enemyTouchesPlayer(foes[j], slash.x, slash.y) {
					foes[j].dashTagged = true
					foes[j].Health -= effectiveDamage(*hero)
					if foes[j].Health <= 0 {
						hero.Experience += foes[j].ExperienceOnKill
					}
				}
			}
		}
		if slash.remaining > 0 {
			active = append(active, slash)
		}
	}
	for i := range foes {
		if foes[i].dashTagged && foes[i].Health <= 0 {
			defeated = append(defeated, i)
			foes[i].dashTagged = false
		}
	}
	return active, defeated
}

func hookTarget(hero player, foes []enemy, maxRange int) int {
	dx, dy := dashDirection(hero.facing)
	faceX, faceY := sign(dx), sign(dy)
	best, bestDistance := -1, maxRange+1
	for i, foe := range foes {
		if foe.Health <= 0 || foe.hooked {
			continue
		}
		x, y := foe.x, foe.y
		if foe.tank {
			x++
			y++
		}
		offsetX, offsetY := x-hero.x, y-hero.y
		distance := offsetX*faceX + offsetY*faceY
		if distance <= 0 || distance > maxRange {
			continue
		}
		if abs(offsetX*faceY-offsetY*faceX) > 2 {
			continue
		}
		if distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	return best
}

func advanceHook(foes []enemy, hero player, width, height int) {
	for i := range foes {
		foe := &foes[i]
		if !foe.hooked {
			continue
		}
		if foe.Health <= 0 || enemyTouchesPlayer(*foe, hero.x, hero.y) {
			foe.hooked = false
			continue
		}
		for step := 0; step < 2; step++ {
			dx, dy := sign(hero.x-foe.x), sign(hero.y-foe.y)
			if dx != 0 {
				dx *= horizontalMovementScale()
			}
			nextX, nextY := keepEnemyInChamber(foe.x+dx, foe.y+dy, width, height, foe.tank)
			if nextX == foe.x && nextY == foe.y {
				foe.hooked = false
				break
			}
			foe.x, foe.y = nextX, nextY
			if enemyTouchesPlayer(*foe, hero.x, hero.y) {
				foe.hooked = false
				break
			}
		}
	}
}

func attackInterval(weapon items.Weapon, attackSpeedBonus float64) time.Duration {
	attacksPerSecond := weapon.AttackSpeed
	if attacksPerSecond <= 0 {
		attacksPerSecond = 2 // unarmed cadence matches the original 500 ms interval
	}
	attacksPerSecond += attackSpeedBonus
	if attacksPerSecond <= 0 {
		attacksPerSecond = 0.1
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
