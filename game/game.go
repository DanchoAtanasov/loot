package game

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"DanchoAtanasov/loot/items"
	"DanchoAtanasov/loot/models"
)

const (
	framesPerSecond              = 60
	initialEnemyPackCount        = 20
	heroImageCols                = 26
	heroImageRows                = 7
	healthImageCols              = 12
	healthImageRows              = 4
	wizardImageID                = 19
	wizardImageCols              = 18
	wizardImageRows              = 6
	warriorImageID               = 20 // Clear any previously cached placement without loading the asset.
	enemySmallImageID            = 11
	enemyMediumImageID           = 12
	enemyTankImageID             = 13
	healthImageID                = 14
	heroRunLeftImageID           = 15
	heroRunRightImageID          = 16
	heroRunLeftMirroredImageID   = 17
	heroRunRightMirroredImageID  = 18
	enemyAggroHorizontalRange    = 20
	enemyAggroVerticalRange      = 9
	heroAttackAOEHorizontalRange = 8
	heroAttackAOEVerticalRange   = 4
	windowSizeIOCTL              = 0x40087468 // TIOCGWINSZ on macOS
	weaponPickupHorizontalRange  = 4
	weaponPickupVerticalRange    = 2
	enemyDeathEffectDuration     = 600 * time.Millisecond
)

type terminalSize struct {
	rows uint16
	cols uint16
	x    uint16
	y    uint16
}

type tile struct {
	glyph rune
	color string
}

type hudSegment struct {
	text  string
	color string
}

type enemy struct {
	models.Enemy
	x, y           int
	tank           bool
	brute          bool
	finalBoss      bool
	dashTagged     bool
	hooked         bool
	ultimateTarget bool
	nextMoveAt     time.Time
}

type player struct {
	models.Player
	weapon                items.Weapon
	armour                items.Armour // chest slot
	helmet                items.Armour
	belt                  items.Armour
	offHand               items.Armour
	potions               int
	x, y                  int
	facing                byte
	lastMove              time.Time
	lastHit               time.Time
	attackStarted         time.Time
	pendingLevelChoices   int
	dashDX, dashDY        int
	dashRemaining         int
	dashCooldownUntil     time.Time
	slashCooldownUntil    time.Time
	hookCooldownUntil     time.Time
	ultimateStarted       time.Time
	ultimateUntil         time.Time
	ultimateCooldownUntil time.Time
	levelUpStarted        time.Time
}

type healthPickup struct {
	x int
	y int
}

type bush struct {
	x int
	y int
}

type enemyDeathEffect struct {
	x, y      int
	tank      bool
	startedAt time.Time
}

type flyingSlash struct {
	x, y           int
	startX, startY int
	dx, dy         int
	remaining      int
	frame          int
	distance       int
}

type gearDrop struct {
	weapon   items.Weapon
	armour   items.Armour
	isArmour bool
	x, y     int
}

type keyEvent struct {
	key     rune
	pressed bool
	tracked bool
}

func wizardPosition(worldWidth, width, height int) (int, int) {
	worldLeft, _, top, bottom := chamberBounds(worldWidth, height)
	return worldLeft + width/2 - 12, (top + bottom) / 2
}

func isNearWizard(hero player, worldWidth, width, height int) bool {
	wizardX, wizardY := wizardPosition(worldWidth, width, height)
	return abs(hero.x-wizardX) <= 8 && abs(hero.y-wizardY) <= 3
}

func Run() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	originalTerminal, err := enableRawMode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not enable keyboard input: %v\n", err)
		return
	}
	defer restoreTerminal(originalTerminal)

	fmt.Print("\x1b[?1049h\x1b[>10u\x1b[?25l\x1b[2J") // alternate screen, key events, hide cursor
	defer fmt.Print("\x1b[?2026l\x1b[0m\x1b[?25h\x1b[<u\x1b[?1049l")
	loadingWidth, loadingHeight := readTerminalSize()
	fmt.Print(renderLoadingScreen(loadingWidth, loadingHeight))
	for imageID, path := range []string{
		"assets/hero.jpeg",
		"assets/hero_prep.jpeg",
		"assets/hero_slash.jpeg",
		"assets/hero_followthrough.jpeg",
		"assets/hero_pullback.jpeg",
	} {
		heroImage, err := loadKittyHero(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not load hero image %q: %v\n", path, err)
			return
		}
		fmt.Print(kittyTransmitHero(heroImage, imageID+1))
		mirroredHero, err := flipKittyHero(heroImage)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not mirror hero image %q: %v\n", path, err)
			return
		}
		fmt.Print(kittyTransmitHero(mirroredHero, imageID+6))
	}
	for imageID, path := range []string{"assets/hero_run_left.png", "assets/hero_run_right.png"} {
		runImage, err := loadKittyHero(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not load hero run image %q: %v\n", path, err)
			return
		}
		fmt.Print(kittyTransmitHero(runImage, heroRunLeftImageID+imageID))
		mirroredRunImage, err := flipKittyHero(runImage)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not mirror hero run image %q: %v\n", path, err)
			return
		}
		fmt.Print(kittyTransmitHero(mirroredRunImage, heroRunLeftMirroredImageID+imageID))
	}
	for imageID, path := range []string{
		"assets/enemy_small.jpeg",
		"assets/enemy_medium.jpeg",
		"assets/enemy_tank.jpeg",
	} {
		enemyImage, err := loadKittyHero(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not load enemy image %q: %v\n", path, err)
			return
		}
		fmt.Print(kittyTransmitHero(enemyImage, enemySmallImageID+imageID))
	}
	healthImage, err := loadKittyHero("assets/health.jpeg")
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not load health image: %v\n", err)
		return
	}
	fmt.Print(kittyTransmitHero(healthImage, healthImageID))
	wizardImage, err := loadKittyHero("assets/pixel_wizard.png")
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not load wizard image: %v\n", err)
		return
	}
	fmt.Print(kittyTransmitHero(wizardImage, wizardImageID))
	fmt.Printf("\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", warriorImageID)

	width, height := readTerminalSize()
	worldWidth := width * 4
	left, _, top, bottom := chamberBounds(worldWidth, height)
	playerX, playerY := left+2, (top+bottom)/2
	bushes := scatterBushes(worldWidth, height)
	hero := player{
		Player:   models.DefaultPlayer,
		weapon:   items.Weapon{},
		potions:  1,
		x:        playerX,
		y:        playerY,
		facing:   's',
		lastMove: time.Now(),
	}
	stage := 1
	foes := spawnStageEnemies(worldWidth, height, stage, playerY)
	stageTransitionUntil := time.Time{}
	deathEffects := make([]enemyDeathEffect, 0)
	flyingSlashes := make([]flyingSlash, 0, 1)
	pickups := make([]healthPickup, 0)
	kills := 0
	gearDrops := make([]gearDrop, 0)
	showEquipment := false
	paused := false
	heroHiUntil := time.Time{}
	wizardHiUntil := time.Time{}
	wizardQuestUntil := time.Time{}
	questActive := false
	keys := make(chan keyEvent, 32)
	heldKeys := make(map[rune]bool)
	lastStep := time.Time{}
	runAnimationStarted := time.Time{}
	go readKeys(keys)

	frameDuration := time.Second / framesPerSecond
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()
	enemyTicker := time.NewTicker(180 * time.Millisecond)
	defer enemyTicker.Stop()
	pickupTicker := time.NewTicker(10 * time.Second)
	defer pickupTicker.Stop()

	for {
		// Refresh the bounds and keep the characters inside the chamber.
		width, height = readTerminalSize()
		hero.x, hero.y = keepPlayerInChamber(hero.x, hero.y, worldWidth, height)

		// Keep combat and world timers frozen while the game is paused.
		now := time.Now()
		stagePending := !stageTransitionUntil.IsZero()
		if stagePending && !now.Before(stageTransitionUntil) {
			stage++
			left, _, top, bottom = chamberBounds(worldWidth, height)
			hero.x, hero.y = keepPlayerInChamber(left+2, (top+bottom)/2, worldWidth, height)
			hero.attackStarted = time.Time{}
			bushes = scatterBushes(worldWidth, height)
			pickups = nil
			gearDrops = nil
			deathEffects = nil
			flyingSlashes = nil
			foes = spawnStageEnemies(worldWidth, height, stage, hero.y)
			stageTransitionUntil = time.Time{}
			stagePending = false
			hero.lastMove = now
			lastStep = now
		}
		if !hero.ultimateUntil.IsZero() && (!now.Before(hero.ultimateUntil) || hero.Health <= 0) {
			hero.ultimateUntil = time.Time{}
			for i := range foes {
				foes[i].ultimateTarget = false
			}
		}
		if !paused && !stagePending && hero.pendingLevelChoices == 0 {
			for i := range foes {
				foes[i].x, foes[i].y = keepEnemyInChamber(foes[i].x, foes[i].y, worldWidth, height, foes[i].tank)
			}
			attackTarget, defeated := resolveBattle(&hero, foes, now)
			if attackTarget >= 0 {
				hero.facing = facingEnemy(hero, foes[attackTarget])
				hero.attackStarted = now
			}
			bossDefeated := false
			for _, index := range defeated {
				foe := foes[index]
				bossDefeated = bossDefeated || foe.finalBoss
				kills++
				if kills%5 == 0 {
					pickups = append(pickups, healthPickup{x: foe.x, y: foe.y})
				}
				if rand.IntN(100) < foe.Enemy.DropChance() {
					dropX, dropY := weaponDropPosition(foe, hero, worldWidth, height)
					drop := gearDrop{x: dropX, y: dropY}
					if rand.IntN(2) == 0 {
						drop.weapon = items.DropItem()
					} else {
						drop.armour = items.DropArmour()
						drop.isArmour = true
					}
					gearDrops = append(gearDrops, drop)
				}
				deathEffects = append(deathEffects, enemyDeathEffect{x: foe.x, y: foe.y, tank: foe.tank, startedAt: now})
			}
			activeEffects := deathEffects[:0]
			for _, effect := range deathEffects {
				if now.Sub(effect.startedAt) < enemyDeathEffectDuration {
					activeEffects = append(activeEffects, effect)
				}
			}
			deathEffects = activeEffects
			alive := foes[:0]
			for _, foe := range foes {
				if foe.Health > 0 {
					alive = append(alive, foe)
				}
			}
			foes = alive
			if bossDefeated {
				clear(heldKeys)
				hero.dashRemaining = 0
				stageTransitionUntil = now.Add(3 * time.Second)
			}
			pickups = collectHealthPickups(&hero, pickups)

			// Check if character should level up
			previousLevel := hero.Level
			levelUp(&hero)
			if hero.Level > previousLevel {
				hero.levelUpStarted = now
				clear(heldKeys)
				hero.dashRemaining = 0
				hero.lastMove = time.Time{}
				lastStep = now
			}
		}

		runningFrame := -1
		idleTime := time.Since(hero.lastMove)
		if idleTime <= 200*time.Millisecond {
			if runAnimationStarted.IsZero() {
				runAnimationStarted = time.Now()
			}
			runningFrame = int(time.Since(runAnimationStarted)/(100*time.Millisecond)) % 2
		} else {
			runAnimationStarted = time.Time{}
		}
		swingFrame := -1
		if now.Before(hero.ultimateUntil) {
			swingFrame = int(time.Since(hero.ultimateStarted)/(60*time.Millisecond)) % 4
		} else if !hero.attackStarted.IsZero() && time.Since(hero.attackStarted) < 240*time.Millisecond {
			swingFrame = int(time.Since(hero.attackStarted) / (60 * time.Millisecond))
		}
		transitionSeconds := 0
		if !stageTransitionUntil.IsZero() {
			transitionSeconds = int(math.Ceil(time.Until(stageTransitionUntil).Seconds()))
		}
		nearWizard := isNearWizard(hero, worldWidth, width, height)
		now = time.Now()
		showHeroHi := now.Before(heroHiUntil)
		showWizardHi := !showHeroHi && now.Before(wizardHiUntil)
		showWizardQuest := !showHeroHi && !showWizardHi && now.Before(wizardQuestUntil) && !wizardQuestUntil.IsZero()
		if !wizardQuestUntil.IsZero() && !now.Before(wizardQuestUntil) {
			questActive = true
		}
		fmt.Print(renderDungeon(hero, width, height, worldWidth, stage, runningFrame, swingFrame, transitionSeconds, foes, pickups, gearDrops, deathEffects, flyingSlashes, bushes, showEquipment, nearWizard, showHeroHi, showWizardHi, showWizardQuest, questActive))

		// Handle shutdown, player input, frame timing, or enemy movement.
		select {
		case <-ctx.Done():
			return
		case event := <-keys:
			if event.key == 3 && event.pressed {
				stop()
				continue
			}
			if event.key == '1' && event.pressed && hero.pendingLevelChoices == 0 && !paused && stageTransitionUntil.IsZero() {
				usePotion(&hero)
				continue
			}
			if hero.pendingLevelChoices > 0 {
				if event.pressed && event.key >= '1' && event.key <= '3' {
					chooseLevelUpgrade(&hero, event.key)
					clear(heldKeys)
					lastStep = time.Now()
				}
				continue
			}
			if (event.key == 'i' || event.key == 'I') && event.pressed {
				showEquipment = !showEquipment
				paused = showEquipment
				clear(heldKeys)
				lastStep = time.Now()
				if !paused {
					pickupTicker.Reset(10 * time.Second)
				}
				continue
			}
			if paused || !stageTransitionUntil.IsZero() {
				continue
			}
			if event.key == ' ' && event.pressed {
				if isNearWizard(hero, worldWidth, width, height) {
					now := time.Now()
					heroHiUntil = now.Add(650 * time.Millisecond)
					wizardHiUntil = now.Add(1300 * time.Millisecond)
					wizardQuestUntil = now.Add(2600 * time.Millisecond)
					continue
				}
				gearDrops = pickupGearDrop(&hero, gearDrops)
				continue
			}
			if (event.key == 'z' || event.key == 'Z') && event.pressed && hero.Health > 0 {
				if hero.dashRemaining == 0 && !time.Now().Before(hero.dashCooldownUntil) {
					hero.dashDX, hero.dashDY = dashDirection(hero.facing)
					hero.dashRemaining = 12
					hero.dashCooldownUntil = time.Now().Add(5 * time.Second)
					for i := range foes {
						foes[i].dashTagged = false
					}
				}
				hero.attackStarted = time.Now()
				continue
			}
			if (event.key == 'x' || event.key == 'X') && event.pressed && hero.Health > 0 {
				if !time.Now().Before(hero.slashCooldownUntil) {
					dx, dy := dashDirection(hero.facing)
					dx, dy = sign(dx), sign(dy)
					hero.slashCooldownUntil = time.Now().Add(time.Second)
					for i := range foes {
						foes[i].dashTagged = false
					}
					startX, startY := hero.x+dx*2, hero.y+dy*2
					flyingSlashes = append(flyingSlashes, flyingSlash{x: startX, y: startY, startX: startX, startY: startY, dx: dx, dy: dy, remaining: 36})
				}
				continue
			}
			if (event.key == 'c' || event.key == 'C') && event.pressed && hero.Health > 0 {
				if !time.Now().Before(hero.hookCooldownUntil) {
					if target := hookTarget(hero, foes, 28); target >= 0 {
						foes[target].hooked = true
						hero.hookCooldownUntil = time.Now().Add(2 * time.Second)
					}
				}
				continue
			}
			if (event.key == 'v' || event.key == 'V') && event.pressed && hero.Health > 0 {
				if hero.ultimateUntil.IsZero() && !time.Now().Before(hero.ultimateCooldownUntil) {
					now := time.Now()
					hero.ultimateStarted = now
					hero.ultimateUntil = now.Add(4 * time.Second)
					hero.ultimateCooldownUntil = now.Add(20 * time.Second)
					hero.dashRemaining = 0
					hero.lastMove = time.Time{}
					clear(heldKeys)
					worldLeft, worldRight, _, _ := chamberBounds(worldWidth, height)
					viewLeft := min(max(hero.x-width/2, worldLeft), max(worldLeft, worldRight-width+1))
					for i := range foes {
						foes[i].ultimateTarget = enemyIsOnScreen(foes[i], viewLeft, width, height)
						if foes[i].ultimateTarget {
							foes[i].hooked = false
						}
					}
				}
				continue
			}
			if !hero.ultimateUntil.IsZero() && time.Now().Before(hero.ultimateUntil) {
				continue
			}
			if event.tracked {
				heldKeys[event.key] = event.pressed
				continue
			}
			if hero.Health > 0 && event.pressed {
				hero.x, hero.y, hero.facing = movePlayer(byte(event.key), hero.x, hero.y, hero.facing, hero.MovementSpeed, worldWidth, height, foes, deathEffects)
				hero.lastMove = time.Now()
			}
		case now := <-ticker.C:
			if !paused && stageTransitionUntil.IsZero() && hero.pendingLevelChoices == 0 {
				advanceHook(foes, hero, worldWidth, height)
				var defeated []int
				flyingSlashes, defeated = advanceFlyingSlashes(flyingSlashes, foes, worldWidth, height, &hero)
				bossDefeated := false
				for _, index := range defeated {
					foe := foes[index]
					bossDefeated = bossDefeated || foe.finalBoss
					kills++
					if kills%5 == 0 {
						pickups = append(pickups, healthPickup{x: foe.x, y: foe.y})
					}
					deathEffects = append(deathEffects, enemyDeathEffect{x: foe.x, y: foe.y, tank: foe.tank, startedAt: now})
				}
				if bossDefeated {
					clear(heldKeys)
					stageTransitionUntil = now.Add(3 * time.Second)
				}
			}
			if !paused && stageTransitionUntil.IsZero() && hero.pendingLevelChoices == 0 && hero.Health > 0 && now.Sub(lastStep) >= 40*time.Millisecond {
				if now.Before(hero.ultimateUntil) {
					advanceUltimate(foes, hero, worldWidth, height)
				} else if hero.dashRemaining > 0 {
					var defeated []int
					hero.x, hero.y, hero.dashRemaining, defeated = advanceDash(&hero, foes, worldWidth, height)
					bossDefeated := false
					for _, index := range defeated {
						foe := foes[index]
						bossDefeated = bossDefeated || foe.finalBoss
						kills++
						if kills%5 == 0 {
							pickups = append(pickups, healthPickup{x: foe.x, y: foe.y})
						}
						deathEffects = append(deathEffects, enemyDeathEffect{x: foe.x, y: foe.y, tank: foe.tank, startedAt: now})
					}
					hero.lastMove = now
					if bossDefeated {
						clear(heldKeys)
						hero.dashRemaining = 0
						stageTransitionUntil = now.Add(3 * time.Second)
					}
				} else {
					// Advance held movement in single-cell increments. Rendering and the
					// run cycle continue independently at 60 fps between movement ticks.
					nextX, nextY, nextFacing := movePlayerHeld(heldKeys, hero.x, hero.y, hero.facing, 1, worldWidth, height, foes, deathEffects)
					if nextX != hero.x || nextY != hero.y {
						hero.x, hero.y, hero.facing = nextX, nextY, nextFacing
						hero.lastMove = now
					}
				}
				lastStep = now
			}
		case <-enemyTicker.C:
			if !paused && hero.pendingLevelChoices == 0 && !time.Now().Before(hero.ultimateUntil) {
				stepEnemyPacks(foes, hero.x, hero.y, worldWidth, height, time.Now())
			}
		case <-pickupTicker.C:
			if !paused && stageTransitionUntil.IsZero() && hero.pendingLevelChoices == 0 {
				pickups = append(pickups, spawnHealthPickup(worldWidth, height))
			}
		}
	}
}

func spawnHealthPickup(width, height int) healthPickup {
	left, right, top, bottom := chamberBounds(width, height)
	minX, maxX := left+1, right-1
	minY, maxY := top+1, bottom-1
	if minX > maxX {
		minX, maxX = left, right
	}
	if minY > maxY {
		minY, maxY = top, bottom
	}
	return healthPickup{x: randomBetween(minX, maxX), y: randomBetween(minY, maxY)}
}

func collectHealthPickups(hero *player, pickups []healthPickup) []healthPickup {
	remaining := pickups[:0]
	for _, pickup := range pickups {
		touching := abs(hero.x-pickup.x) <= 2 && abs(hero.y-pickup.y) <= 1
		if touching {
			hero.potions++
			continue
		}
		remaining = append(remaining, pickup)
	}
	return remaining
}

func usePotion(hero *player) bool {
	if hero.potions <= 0 || hero.Health >= hero.MaxHealth {
		return false
	}
	hero.potions--
	hero.Health = min(hero.Health+10, hero.MaxHealth)
	return true
}

func weaponDropPosition(foe enemy, hero player, width, height int) (int, int) {
	left, right, top, bottom := chamberBounds(width, height)
	minX, maxX := left+1, right-1
	minY, maxY := top+1, bottom-1
	candidates := [][2]int{
		{foe.x + 6, foe.y},
		{foe.x - 6, foe.y},
		{foe.x, foe.y + 4},
		{foe.x, foe.y - 4},
	}
	best := candidates[0]
	bestDistance := -1
	fallbackDistance := -1
	fallback := best
	for _, candidate := range candidates {
		x := min(max(candidate[0], minX), maxX)
		y := min(max(candidate[1], minY), maxY)
		distance := abs(x-hero.x) + abs(y-hero.y)
		if distance > fallbackDistance {
			fallback = [2]int{x, y}
			fallbackDistance = distance
		}
		if abs(x-hero.x) <= weaponPickupHorizontalRange && abs(y-hero.y) <= weaponPickupVerticalRange {
			continue
		}
		if distance > bestDistance {
			best = [2]int{x, y}
			bestDistance = distance
		}
	}
	if bestDistance < 0 {
		best = fallback
	}
	return best[0], best[1]
}

func pickupGearDrop(hero *player, drops []gearDrop) []gearDrop {
	nearestIndex, nearestDistance := -1, int(^uint(0)>>1)
	for i, drop := range drops {
		if !gearDropInRange(*hero, drop) {
			continue
		}
		distance := abs(hero.x-drop.x) + abs(hero.y-drop.y)
		if distance < nearestDistance {
			nearestIndex, nearestDistance = i, distance
		}
	}
	if nearestIndex < 0 {
		return drops
	}
	drop := drops[nearestIndex]
	if drop.isArmour {
		previousHealthBonus := armourHealthBonus(*hero)
		switch drop.armour.Slot {
		case items.HelmetSlot:
			hero.helmet = drop.armour
		case items.BeltSlot:
			hero.belt = drop.armour
		case items.OffHandSlot:
			hero.offHand = drop.armour
		default:
			hero.armour = drop.armour
		}
		hero.Armour = totalArmour(*hero)
		healthBonusChange := armourHealthBonus(*hero) - previousHealthBonus
		hero.MaxHealth += healthBonusChange
		hero.Health = min(hero.Health, hero.MaxHealth)
	} else {
		hero.weapon = drop.weapon
	}
	return append(drops[:nearestIndex], drops[nearestIndex+1:]...)
}

func nearestGearDrop(hero player, drops []gearDrop) (gearDrop, bool) {
	if len(drops) == 0 {
		return gearDrop{}, false
	}
	nearest := drops[0]
	nearestDistance := abs(hero.x-nearest.x) + abs(hero.y-nearest.y)
	for _, drop := range drops[1:] {
		distance := abs(hero.x-drop.x) + abs(hero.y-drop.y)
		if distance < nearestDistance {
			nearest, nearestDistance = drop, distance
		}
	}
	return nearest, true
}

func gearDropInRange(hero player, drop gearDrop) bool {
	return abs(hero.x-drop.x) <= weaponPickupHorizontalRange && abs(hero.y-drop.y) <= weaponPickupVerticalRange
}

func equippedWeaponName(weapon items.Weapon) string {
	if weapon.Name == "" {
		return "None"
	}
	return weapon.Name
}

func equippedArmourName(armour items.Armour) string {
	if armour.Name == "" {
		return "None"
	}
	return armour.Name
}

func totalArmour(hero player) int {
	return hero.armour.Defence + hero.helmet.Defence + hero.belt.Defence + hero.offHand.Defence
}

func armourHealthBonus(hero player) int {
	return hero.armour.Health + hero.helmet.Health + hero.belt.Health + hero.offHand.Health
}

func equippedArmourForSlot(hero player, slot items.ArmourSlot) items.Armour {
	switch slot {
	case items.HelmetSlot:
		return hero.helmet
	case items.BeltSlot:
		return hero.belt
	case items.OffHandSlot:
		return hero.offHand
	default:
		return hero.armour
	}
}

func effectiveDamage(hero player) int {
	return max(hero.Damage, 0) + max(hero.weapon.Damage, 0)
}

func weaponAttackSpeed(weapon items.Weapon) float64 {
	if weapon.AttackSpeed <= 0 {
		return 2 // unarmed attacks retain the original 500 ms cadence
	}
	return weapon.AttackSpeed
}

func effectiveAttackSpeed(weapon items.Weapon, progressionBonus float64) float64 {
	return weaponAttackSpeed(weapon) + progressionBonus
}

func weaponTierName(tier items.ItemTier) string {
	switch tier {
	case items.MagicTier:
		return "Magic"
	case items.RareTier:
		return "Rare"
	case items.UniqueTier:
		return "Unique"
	default:
		return "Normal"
	}
}

func weaponTierColor(tier items.ItemTier) string {
	switch tier {
	case items.MagicTier:
		return "\x1b[38;2;113;205;232m"
	case items.RareTier:
		return "\x1b[38;2;232;195;92m"
	case items.UniqueTier:
		return "\x1b[38;2;225;132;220m"
	default:
		return "\x1b[38;2;220;220;205m"
	}
}

func spawnStageEnemies(width, height, stage, bossY int) []enemy {
	foes := spawnEnemyPacksForStage(width, height, initialEnemyPackCount, stage)
	_, worldRight, _, _ := chamberBounds(width, height)
	boss := newEnemyForStage(true, false, stage)
	boss.x, boss.y = worldRight-3, bossY
	boss.finalBoss = true
	return append(foes, boss)
}

func spawnEnemyPacks(width, height, count int) []enemy {
	return spawnEnemyPacksForStage(width, height, count, 1)
}

func spawnEnemyPacksForStage(width, height, count, stage int) []enemy {
	foes := make([]enemy, 0, count*3)
	for i := 0; i < count; i++ {
		foes = append(foes, spawnEnemyPackForStage(width, height, stage)...)
	}
	return foes
}

func spawnEnemyPack(width, height int) []enemy {
	return spawnEnemyPackForStage(width, height, 1)
}

func spawnEnemyPackForStage(width, height, stage int) []enemy {
	packSize := 2 + rand.IntN(2)
	pack := make([]enemy, 0, packSize)
	for i := 0; i < packSize; i++ {
		tank := false
		brute := rand.IntN(4) == 0
		pack = append(pack, newEnemyForStage(tank, brute, stage))
	}

	pack[0].x, pack[0].y = randomPackAnchor(width, height, pack[0].tank)
	for i := 1; i < len(pack); i++ {
		pack[i].x, pack[i].y = nearbySpawnPosition(pack[0], pack[i], pack[:i], width, height)
	}
	return pack
}

func randomPackAnchor(width, height int, tank bool) (int, int) {
	left, right, top, bottom := chamberBounds(width, height)
	minX, maxX := left+2, right-2
	minY, maxY := top+1, bottom-1
	if tank {
		maxX--
		maxY--
	}
	if minX > maxX {
		minX, maxX = left+1, right-1
	}
	if minY > maxY {
		minY, maxY = top+1, bottom-1
	}
	return randomBetween(minX, maxX), randomBetween(minY, maxY)
}

func nearbySpawnPosition(anchor, candidate enemy, pack []enemy, width, height int) (int, int) {
	positions := make([][2]int, 0, 400)
	left, right, top, bottom := chamberBounds(width, height)
	for dy := -8; dy <= 8; dy++ {
		for dx := -18; dx <= 18; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := keepEnemyInChamber(anchor.x+dx, anchor.y+dy, width, height, candidate.tank)
			if enemyHasSpawnSpace(candidate, x, y, pack) {
				positions = append(positions, [2]int{x, y})
			}
		}
	}
	if len(positions) == 0 {
		bestX, bestY, bestDistance := anchor.x, anchor.y, -1
		for y := top + 1; y < bottom; y++ {
			for x := left + 1; x < right; x++ {
				distance := nearestEnemyDistance(x, y, pack)
				if distance > bestDistance {
					bestX, bestY, bestDistance = x, y, distance
				}
			}
		}
		return bestX, bestY
	}
	position := positions[rand.IntN(len(positions))]
	return position[0], position[1]
}

func enemyHasSpawnSpace(candidate enemy, x, y int, pack []enemy) bool {
	for _, other := range pack {
		minX, minY := 7, 3
		if candidate.brute || other.brute {
			minX, minY = 12, 5
		}
		if abs(x-other.x) < minX && abs(y-other.y) < minY {
			return false
		}
	}
	return true
}

func nearestEnemyDistance(x, y int, pack []enemy) int {
	if len(pack) == 0 {
		return int(^uint(0) >> 1)
	}
	nearest := int(^uint(0) >> 1)
	for _, other := range pack {
		nearest = min(nearest, abs(x-other.x)/2+abs(y-other.y))
	}
	return nearest
}

func newEnemy(tank, brute bool) enemy {
	return newEnemyForStage(tank, brute, 1)
}

func newEnemyForStage(tank, brute bool, stage int) enemy {
	foe := enemy{Enemy: models.DefaultEnemy, tank: tank, brute: brute}
	if tank {
		foe.Enemy = models.TankEnemy
	} else if brute {
		foe.Enemy = models.BruteEnemy
	}
	stage = max(stage, 1)
	foe.Damage += (stage - 1) / 2
	foe.ExperienceOnKill += (foe.ExperienceOnKill*(stage-1) + 4) / 5
	return foe
}

func randomBetween(minimum, maximum int) int {
	return minimum + rand.IntN(maximum-minimum+1)
}

func scatterBushes(width, height int) []bush {
	left, right, top, bottom := chamberBounds(width, height)
	count := max((right-left)/12, 1)
	plants := make([]bush, 0, count)
	for len(plants) < count {
		candidate := bush{
			x: randomBetween(left+1, right-2),
			y: randomBetween(top+1, bottom-2),
		}
		tooClose := false
		for _, existing := range plants {
			if abs(existing.x-candidate.x) < 4 && abs(existing.y-candidate.y) < 3 {
				tooClose = true
				break
			}
		}
		if !tooClose {
			plants = append(plants, candidate)
		}
	}
	return plants
}

func stepEnemyPacks(foes []enemy, heroX, heroY, width, height int, now time.Time) {
	for i := range foes {
		foe := &foes[i]
		if foe.Health <= 0 || foe.hooked || foe.ultimateTarget || now.Before(foe.nextMoveAt) {
			continue
		}
		centerX, centerY := foe.x, foe.y
		if foe.tank {
			centerX++
			centerY++
		}
		if abs(heroX-centerX) > enemyAggroHorizontalRange || abs(heroY-centerY) > enemyAggroVerticalRange {
			continue
		}
		if heroCanHitEnemy(*foe, heroX, heroY) {
			continue
		}
		dx, dy := sign(heroX-centerX), sign(heroY-centerY)
		if dx != 0 {
			dx *= horizontalMovementScale()
		}
		nextX, nextY := keepEnemyInChamber(foe.x+dx, foe.y+dy, width, height, foe.tank)
		if nextX != foe.x || nextY != foe.y {
			foe.x, foe.y = nextX, nextY
		}
		foe.nextMoveAt = now.Add(650 * time.Millisecond)
	}
}

func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}

func keepEnemyInChamber(x, y, width, height int, tank bool) (int, int) {
	left, right, top, bottom := chamberBounds(width, height)
	if right-left < 2 || bottom-top < 2 {
		return chamberCenter(width, height)
	}
	maxX, maxY := right-1, bottom-1
	if tank {
		maxX = max(left+1, maxX-1)
		maxY = max(top+1, maxY-1)
	}
	return min(max(x, left+1), maxX), min(max(y, top+1), maxY)
}

func levelUp(hero *player) {
	maxInt := int(^uint(0) >> 1)
	if hero.pendingLevelChoices == 0 && hero.Level < maxInt && hero.Experience >= models.ExperienceForLevel(hero.Level) {
		hero.Level++
		hero.pendingLevelChoices = 1
		hero.MaxHealth++
	}
}

func chooseLevelUpgrade(hero *player, choice rune) bool {
	if hero.pendingLevelChoices == 0 {
		return false
	}
	switch choice {
	case '1':
		hero.Damage += models.PlayerStatGrowth.Damage
	case '2':
		hero.MaxHealth += models.PlayerStatGrowth.MaxHealth
		if hero.Health > 0 {
			hero.Health = min(hero.Health+models.PlayerStatGrowth.MaxHealth, hero.MaxHealth)
		}
	case '3':
		hero.AttackSpeed += models.PlayerStatGrowth.AttackSpeed
	default:
		return false
	}
	hero.pendingLevelChoices--
	return true
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func chamberBounds(width, height int) (left, right, top, bottom int) {
	left, right = 2, width-3
	top, bottom = 1, height-2
	if right-left < 8 || bottom-top < 6 {
		left, right, top, bottom = 0, width-1, 0, height-1
	}
	return
}

func chamberCenter(width, height int) (int, int) {
	left, right, top, bottom := chamberBounds(width, height)
	return (left + right) / 2, (top + bottom) / 2
}

func keepPlayerInChamber(x, y, width, height int) (int, int) {
	left, right, top, bottom := chamberBounds(width, height)
	if right-left < 2 || bottom-top < 2 {
		return chamberCenter(width, height)
	}
	return min(max(x, left+2), right-2), min(max(y, top+1), bottom-1)
}

func movePlayer(key byte, x, y int, facing byte, movementSpeed, width, height int, foes []enemy, effects []enemyDeathEffect) (int, int, byte) {
	step := max(movementSpeed, 1)
	horizontalStep := horizontalMovementScale() * step
	targetX, targetY := x, y
	switch key {
	case 'w', 'W':
		targetY -= step
		facing = 'w'
	case 's', 'S':
		targetY += step
		facing = 's'
	case 'a', 'A':
		targetX -= horizontalStep
		facing = 'a'
	case 'd', 'D':
		targetX += horizontalStep
		facing = 'd'
	}
	x, y = movePlayerAlongPath(x, y, targetX, targetY, width, height, foes, effects)
	return x, y, facing
}

func movePlayerHeld(held map[rune]bool, x, y int, facing byte, movementSpeed, width, height int, foes []enemy, effects []enemyDeathEffect) (int, int, byte) {
	dx, dy := 0, 0
	if held['a'] != held['d'] {
		if held['a'] {
			dx = -1
		} else {
			dx = 1
		}
	}
	if held['w'] != held['s'] {
		if held['w'] {
			dy = -1
		} else {
			dy = 1
		}
	}
	if dx == 0 && dy == 0 {
		return x, y, facing
	}

	switch {
	case dx < 0 && dy < 0:
		facing = 'q'
	case dx > 0 && dy < 0:
		facing = 'e'
	case dx < 0 && dy > 0:
		facing = 'z'
	case dx > 0 && dy > 0:
		facing = 'c'
	case dx < 0:
		facing = 'a'
	case dx > 0:
		facing = 'd'
	case dy < 0:
		facing = 'w'
	case dy > 0:
		facing = 's'
	}
	step := max(movementSpeed, 1)
	targetX := x + dx*horizontalMovementScale()*step
	targetY := y + dy*step
	x, y = movePlayerAlongPath(x, y, targetX, targetY, width, height, foes, effects)
	return x, y, facing
}

func movePlayerAlongPath(x, y, targetX, targetY, width, height int, foes []enemy, effects []enemyDeathEffect) (int, int) {
	targetX, targetY = keepPlayerInChamber(targetX, targetY, width, height)
	deltaX, deltaY := abs(targetX-x), abs(targetY-y)
	stepX, stepY := 1, 1
	if x > targetX {
		stepX = -1
	}
	if y > targetY {
		stepY = -1
	}
	error := deltaX - deltaY
	for x != targetX || y != targetY {
		previousX, previousY := x, y
		doubleError := 2 * error
		if doubleError > -deltaY {
			error -= deltaY
			x += stepX
		}
		if doubleError < deltaX {
			error += deltaX
			y += stepY
		}
		if playerBlockedByEnemy(x, y, previousX, previousY, foes, effects) {
			return previousX, previousY
		}
	}
	return x, y
}

func playerBlockedByEnemy(x, y, previousX, previousY int, foes []enemy, effects []enemyDeathEffect) bool {
	for _, foe := range foes {
		if foe.Health <= 0 || !enemyTouchesPlayer(foe, x, y) {
			continue
		}
		if !enemyTouchesPlayer(foe, previousX, previousY) || enemyCenterDistance(foe, x, y) <= enemyCenterDistance(foe, previousX, previousY) {
			return true
		}
	}
	return false
}

func enemyCenterDistance(foe enemy, x, y int) int {
	centerX, centerY := foe.x, foe.y
	if foe.tank {
		centerX++
		centerY++
	}
	return abs(x-centerX) + abs(y-centerY)
}

func deathEffectTouchesPlayer(effect enemyDeathEffect, x, y int) bool {
	maxX, maxY := effect.x, effect.y
	if effect.tank {
		maxX++
		maxY++
	}
	return x >= effect.x-2 && x <= maxX+2 && y >= effect.y-1 && y <= maxY+1
}

func deathEffectCenterDistance(effect enemyDeathEffect, x, y int) int {
	centerX, centerY := effect.x, effect.y
	if effect.tank {
		centerX++
		centerY++
	}
	return abs(x-centerX) + abs(y-centerY)
}

func horizontalMovementScale() int {
	var size terminalSize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), windowSizeIOCTL, uintptr(unsafe.Pointer(&size)))
	if errno != 0 || size.cols == 0 || size.rows == 0 || size.x == 0 || size.y == 0 {
		return 2
	}
	cellWidth := float64(size.x) / float64(size.cols)
	cellHeight := float64(size.y) / float64(size.rows)
	return max(1, int(math.Round(cellHeight/cellWidth)))
}

func readKeys(keys chan<- keyEvent) {
	reader := bufio.NewReader(os.Stdin)
	for {
		first, err := reader.ReadByte()
		if err != nil {
			return
		}
		if first != 0x1b {
			sendKey(keys, keyEvent{key: rune(first), pressed: true})
			continue
		}

		intro, err := reader.ReadByte()
		if err != nil {
			return
		}
		if intro != '[' {
			continue
		}
		var sequence strings.Builder
		for {
			part, readErr := reader.ReadByte()
			if readErr != nil {
				return
			}
			if part >= 0x40 && part <= 0x7e {
				key, ok := parseKeySequence(sequence.String(), part)
				if ok {
					sendKey(keys, key)
				}
				break
			}
			sequence.WriteByte(part)
		}
	}
}

func parseKeySequence(sequence string, terminator byte) (keyEvent, bool) {
	// Plain CSI arrows work as single steps. Kitty CSI arrows include event
	// types, which let held movement stop cleanly on key release.
	arrow := rune(0)
	switch terminator {
	case 'A':
		arrow = 'w'
	case 'B':
		arrow = 's'
	case 'C':
		arrow = 'd'
	case 'D':
		arrow = 'a'
	}
	if arrow != 0 {
		if !strings.Contains(sequence, ":") {
			return keyEvent{key: arrow, pressed: true}, true
		}
		fields := strings.Split(sequence, ";")
		eventFields := strings.Split(fields[len(fields)-1], ":")
		eventType, err := strconv.Atoi(eventFields[len(eventFields)-1])
		if err != nil {
			return keyEvent{}, false
		}
		return keyEvent{key: arrow, pressed: eventType != 3, tracked: true}, true
	}
	if terminator != 'u' {
		return keyEvent{}, false
	}

	fields := strings.Split(sequence, ";")
	if len(fields) == 0 {
		return keyEvent{}, false
	}
	code, err := strconv.Atoi(fields[0])
	if err != nil {
		return keyEvent{}, false
	}
	key := keyFromCode(code)
	if key == 0 {
		return keyEvent{}, false
	}

	modifiers, eventType := 1, 1
	if len(fields) > 1 {
		eventFields := strings.Split(fields[1], ":")
		modifiers, _ = strconv.Atoi(eventFields[0])
		if len(eventFields) > 1 {
			eventType, _ = strconv.Atoi(eventFields[1])
		}
	}
	if key == 'c' && (modifiers-1)&4 != 0 {
		key = 3 // Preserve Ctrl-C as the game's exit key.
	}
	return keyEvent{key: key, pressed: eventType != 3, tracked: true}, true
}

func keyFromCode(code int) rune {
	if code == ' ' {
		return ' '
	}
	if code >= 'a' && code <= 'z' || code >= '0' && code <= '9' {
		return rune(code)
	}
	switch code {
	case 57350: // Left arrow.
		return 'a'
	case 57351: // Right arrow.
		return 'd'
	case 57352: // Up arrow.
		return 'w'
	case 57353: // Down arrow.
		return 's'
	default:
		return 0
	}
}

func sendKey(keys chan<- keyEvent, key keyEvent) {
	select {
	case keys <- key:
	default:
	}
}

func enableRawMode() (*syscall.Termios, error) {
	var original syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TIOCGETA, uintptr(unsafe.Pointer(&original)))
	if errno != 0 {
		return nil, errno
	}
	raw := original
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TIOCSETA, uintptr(unsafe.Pointer(&raw)))
	if errno != 0 {
		return nil, errno
	}
	return &original, nil
}

func restoreTerminal(original *syscall.Termios) {
	if original != nil {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TIOCSETA, uintptr(unsafe.Pointer(original)))
	}
}

func readTerminalSize() (int, int) {
	var size terminalSize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), windowSizeIOCTL, uintptr(unsafe.Pointer(&size)))
	if errno == 0 && size.cols > 0 && size.rows > 0 {
		return int(size.cols), int(size.rows)
	}
	width, height := 80, 24
	if value := os.Getenv("COLUMNS"); value != "" {
		fmt.Sscanf(value, "%d", &width)
	}
	if value := os.Getenv("LINES"); value != "" {
		fmt.Sscanf(value, "%d", &height)
	}
	return max(width, 1), max(height, 1)
}
