package game

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
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
	x, y       int
	tank       bool
	brute      bool
	finalBoss  bool
	nextMoveAt time.Time
}

type player struct {
	models.Player
	weapon         items.Weapon
	armour         items.Armour
	potions        int
	x, y           int
	facing         byte
	lastMove       time.Time
	lastHit        time.Time
	attackStarted  time.Time
	levelUpStarted time.Time
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
	foes := spawnEnemyPacks(worldWidth, height, initialEnemyPackCount)
	_, worldRight, _, _ := chamberBounds(worldWidth, height)
	tank := newEnemy(true, false)
	tank.x, tank.y = worldRight-3, playerY
	tank.finalBoss = true
	foes = append(foes, tank)
	deathEffects := make([]enemyDeathEffect, 0)
	pickups := make([]healthPickup, 0)
	kills := 0
	gearDrops := make([]gearDrop, 0)
	showEquipment := false
	paused := false
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
		if !paused {
			for i := range foes {
				foes[i].x, foes[i].y = keepEnemyInChamber(foes[i].x, foes[i].y, worldWidth, height, foes[i].tank)
			}
			attackTarget, defeated := resolveBattle(&hero, foes, now)
			if attackTarget >= 0 {
				hero.facing = facingEnemy(hero, foes[attackTarget])
				hero.attackStarted = now
			}
			for _, index := range defeated {
				foe := foes[index]
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
			pickups = collectHealthPickups(&hero, pickups)

			// Check if character should level up
			previousLevel := hero.Level
			levelUp(&hero)
			if hero.Level > previousLevel {
				hero.levelUpStarted = now
			}
		}

		// Switch the warrior's pose while idle.
		idleTime := time.Since(hero.lastMove)
		idleFrame := idleTime > 400*time.Millisecond && int(idleTime/(500*time.Millisecond))%2 == 1
		runningFrame := -1
		if idleTime <= 200*time.Millisecond {
			if runAnimationStarted.IsZero() {
				runAnimationStarted = time.Now()
			}
			runningFrame = int(time.Since(runAnimationStarted)/(100*time.Millisecond)) % 2
		} else {
			runAnimationStarted = time.Time{}
		}
		swingFrame := -1
		if !hero.attackStarted.IsZero() && time.Since(hero.attackStarted) < 240*time.Millisecond {
			swingFrame = int(time.Since(hero.attackStarted) / (60 * time.Millisecond))
		}
		fmt.Print(renderDungeon(hero, width, height, worldWidth, idleFrame, runningFrame, swingFrame, foes, pickups, gearDrops, deathEffects, bushes, showEquipment))

		// Handle shutdown, player input, frame timing, or enemy movement.
		select {
		case <-ctx.Done():
			return
		case event := <-keys:
			if event.key == 3 && event.pressed {
				stop()
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
			if paused {
				continue
			}
			if event.key == ' ' && event.pressed {
				gearDrops = pickupGearDrop(&hero, gearDrops)
				continue
			}
			if event.key == '1' && event.pressed {
				usePotion(&hero)
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
			if !paused && hero.Health > 0 && now.Sub(lastStep) >= 40*time.Millisecond {
				// Advance held movement in single-cell increments. Rendering and the
				// run cycle continue independently at 60 fps between movement ticks.
				nextX, nextY, nextFacing := movePlayerHeld(heldKeys, hero.x, hero.y, hero.facing, 1, worldWidth, height, foes, deathEffects)
				if nextX != hero.x || nextY != hero.y {
					hero.x, hero.y, hero.facing = nextX, nextY, nextFacing
					hero.lastMove = now
				}
				lastStep = now
			}
		case <-enemyTicker.C:
			if !paused {
				stepEnemyPacks(foes, hero.x, hero.y, worldWidth, height, time.Now())
			}
		case <-pickupTicker.C:
			if !paused {
				pickups = append(pickups, spawnHealthPickup(worldWidth, height))
			}
		}
	}
}

func renderDungeon(hero player, width, height, worldWidth int, idleFrame bool, runningFrame, swingFrame int, foes []enemy, pickups []healthPickup, gearDrops []gearDrop, deathEffects []enemyDeathEffect, bushes []bush, showEquipment bool) string {
	grid := make([][]tile, height)
	for y := range grid {
		grid[y] = make([]tile, width)
		for x := range grid[y] {
			grid[y][x] = tile{' ', ""}
		}
	}

	worldLeft, worldRight, top, bottom := chamberBounds(worldWidth, height)
	viewLeft := min(max(hero.x-width/2, worldLeft), max(worldLeft, worldRight-width+1))
	left := max(worldLeft-viewLeft, 0)
	right := min(worldRight-viewLeft, width-1)
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			border := (viewLeft+x == worldLeft || viewLeft+x == worldRight) || y == top || y == bottom
			if border {
				wallColor := "\x1b[38;2;174;181;192m"
				if viewLeft+x == worldRight {
					wallColor = "\x1b[38;2;255;190;92m"
				}
				grid[y][x] = tile{'█', wallColor}
			} else {
				grid[y][x] = tile{'·', "\x1b[38;2;66;71;78m"}
			}
		}
	}
	for _, plant := range bushes {
		x := plant.x - viewLeft
		if x >= left && x <= right && plant.y > top && plant.y < bottom {
			grid[plant.y][x] = tile{'♣', "\x1b[38;2;93;147;83m"}
			if x+1 <= right {
				grid[plant.y][x+1] = tile{'♧', "\x1b[38;2;115;164;92m"}
			}
			if plant.y+1 < bottom && x+1 <= right {
				grid[plant.y+1][x+1] = tile{'♣', "\x1b[38;2;77;128;75m"}
			}
		}
	}

	if right-left >= 12 && bottom-top >= 8 {
		midX, midY := (worldLeft+worldRight)/2-viewLeft, (top+bottom)/2
		for _, p := range [][2]int{{worldLeft + 4 - viewLeft, top + 3}, {worldRight - 4 - viewLeft, top + 3}, {worldLeft + 4 - viewLeft, bottom - 3}, {worldRight - 4 - viewLeft, bottom - 3}} {
			if p[0] > left && p[0] < right {
				grid[p[1]][p[0]] = tile{'╬', "\x1b[38;2;145;132;103m"}
			}
		}
		// An old flagstone circle marks the center of the room.
		if midX >= left && midX <= right {
			for _, p := range [][2]int{{midX, midY}, {midX - 1, midY}, {midX + 1, midY}, {midX, midY - 1}, {midX, midY + 1}} {
				if p[0] >= left && p[0] <= right {
					grid[p[1]][p[0]] = tile{'◈', "\x1b[38;2;136;113;78m"}
				}
			}
		}
	}
	drawHUD(grid, hero, foes, gearDrops)
	visibleEffects := make([]enemyDeathEffect, 0, len(deathEffects))
	for _, effect := range deathEffects {
		effect.x -= viewLeft
		visibleEffects = append(visibleEffects, effect)
	}
	drawEnemyDeathEffects(grid, visibleEffects, left, right, top, bottom, time.Now())
	for _, drop := range gearDrops {
		drop.x -= viewLeft
		drawGearDrop(grid, drop, left, right, top, bottom)
	}
	drawLevelUpEffect(grid, hero.x-viewLeft, hero.y, hero.levelUpStarted, time.Now(), left, right, top, bottom)
	// The player is rendered as a Kitty graphics placement after the text frame.
	uiOverlayOpen := showEquipment
	if drop, ok := nearestGearDrop(hero, gearDrops); ok && gearDropInRange(hero, drop) {
		drawHero, drawDrop := hero, drop
		drawHero.x -= viewLeft
		drawDrop.x -= viewLeft
		drawGearOverlay(grid, drawHero, drawDrop)
		uiOverlayOpen = true
	}
	if showEquipment {
		drawEquipmentPanel(grid, hero)
	}

	var frame bytes.Buffer
	frame.WriteString("\x1b[?2026h")
	for imageID := 1; imageID <= heroRunRightMirroredImageID; imageID++ {
		fmt.Fprintf(&frame, "\x1b_Ga=d,d=i,i=%d,p=1,q=2\x1b\\", imageID)
	}
	for imageID := enemySmallImageID; imageID <= enemyTankImageID; imageID++ {
		fmt.Fprintf(&frame, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", imageID)
	}
	fmt.Fprintf(&frame, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", healthImageID)
	frame.WriteString("\x1b[H")
	for y, row := range grid {
		lastColor := ""
		for _, cell := range row {
			if cell.color != lastColor {
				frame.WriteString("\x1b[0m")
				frame.WriteString(cell.color)
				lastColor = cell.color
			}
			frame.WriteRune(cell.glyph)
		}
		if y != len(grid)-1 {
			frame.WriteString("\x1b[K\r\n")
		}
	}
	frame.WriteString("\x1b[0m\x1b[K\x1b[?2026l")
	if !uiOverlayOpen {
		for index, pickup := range pickups {
			x := pickup.x - viewLeft
			if x+healthImageCols/2 < 0 || x-healthImageCols/2 >= width {
				continue
			}
			row := max(pickup.y-healthImageRows/2, 3)
			col := max(x-healthImageCols/2, 1)
			frame.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,c=%d,r=%d,q=2\x1b\\", row, col, healthImageID, index+1, healthImageCols, healthImageRows))
		}
	}
	for index, foe := range foes {
		if foe.Health <= 0 {
			continue
		}
		x := foe.x - viewLeft
		if x+heroImageCols/2 < 0 || x-heroImageCols/2 >= width {
			continue
		}
		imageID, cols, rows := enemySpriteSize(foe)
		y := foe.y
		if foe.tank {
			x++
			y++
		}
		row := max(y-rows/2, 3) // Keep enemy art below the two HUD rows.
		frame.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,c=%d,r=%d,q=2\x1b\\", row, max(x-cols/2, 1), imageID, index+1, cols, rows))
	}
	// Overlay the pixel art after the text frame so the terminal image is not
	// overwritten by subsequent character drawing.
	imageID := 1
	if swingFrame >= 0 {
		imageID = 2 + swingFrame
	} else if runningFrame >= 0 {
		imageID = heroRunLeftMirroredImageID + runningFrame
	}
	if hero.facing == 'd' || hero.facing == 'e' || hero.facing == 'c' {
		if runningFrame >= 0 && swingFrame < 0 {
			imageID = heroRunLeftImageID + runningFrame
		} else {
			imageID += 5
		}
	}
	heroScreenX := hero.x - viewLeft
	frame.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b_Ga=p,i=%d,p=1,c=%d,r=%d,q=2\x1b\\", max(hero.y-heroImageRows/2, 1), max(heroScreenX-heroImageCols/2, 1), imageID, heroImageCols, heroImageRows))
	return frame.String()
}

func enemySpriteSize(foe enemy) (imageID, cols, rows int) {
	switch {
	case foe.tank:
		return enemyTankImageID, heroImageCols, heroImageRows
	case foe.brute:
		return enemyMediumImageID, 18, 5
	default:
		return enemySmallImageID, 12, 4
	}
}

func drawLevelUpEffect(grid [][]tile, x, y int, startedAt, now time.Time, left, right, top, bottom int) {
	const duration = 1100 * time.Millisecond
	if startedAt.IsZero() {
		return
	}
	elapsed := now.Sub(startedAt)
	if elapsed < 0 || elapsed >= duration {
		return
	}
	progress := float64(elapsed) / float64(duration)
	radiusX := 6 + int(math.Round(progress*4))
	radiusY := 2 + int(math.Round(progress*2))
	brightness := 1 - progress*0.55
	color := fmt.Sprintf("\x1b[38;2;%d;%d;%dm",
		int(255*brightness), int(224*brightness), int(126*brightness))
	glyphs := []rune{'✦', '✧', '⋆', '+'}
	phase := int(elapsed / (90 * time.Millisecond))
	for i := 0; i < 16; i++ {
		angle := 2*math.Pi*float64(i)/16 + progress*math.Pi/3
		px := x + int(math.Round(math.Cos(angle)*float64(radiusX)))
		py := y + int(math.Round(math.Sin(angle)*float64(radiusY)))
		if px <= left || px >= right || py <= top || py >= bottom || py < 0 || py >= len(grid) || px < 0 || px >= len(grid[py]) {
			continue
		}
		glyph := glyphs[(i+phase)%len(glyphs)]
		grid[py][px] = tile{glyph: glyph, color: color}
	}
}

func loadKittyHero(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	source, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	// The supplied JPEG uses a flat charcoal background. Make the connected
	// background pixels transparent, preserving the dark outlines in the art.
	bounds := source.Bounds()
	img := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, source.At(x, y))
		}
	}
	background := color.NRGBAModel.Convert(source.At(bounds.Min.X, bounds.Min.Y)).(color.NRGBA)
	visited := make([]bool, bounds.Dx()*bounds.Dy())
	queue := []image.Point{{X: bounds.Min.X, Y: bounds.Min.Y}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		index := (p.Y-bounds.Min.Y)*bounds.Dx() + p.X - bounds.Min.X
		if visited[index] {
			continue
		}
		visited[index] = true
		c := color.NRGBAModel.Convert(source.At(p.X, p.Y)).(color.NRGBA)
		if abs(int(c.R)-int(background.R))+abs(int(c.G)-int(background.G))+abs(int(c.B)-int(background.B)) > 24 {
			continue
		}
		img.SetNRGBA(p.X, p.Y, color.NRGBA{})
		for _, next := range []image.Point{{p.X - 1, p.Y}, {p.X + 1, p.Y}, {p.X, p.Y - 1}, {p.X, p.Y + 1}} {
			if next.In(bounds) {
				queue = append(queue, next)
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func flipKittyHero(pngData []byte) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	flipped := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			flipped.Set(x, y, source.At(bounds.Max.X-1-x, bounds.Min.Y+y))
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, flipped); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func kittyTransmitHero(pngData []byte, imageID int) string {
	encoded := base64.StdEncoding.EncodeToString(pngData)
	var out strings.Builder
	for len(encoded) > 0 {
		chunkSize := min(len(encoded), 4096)
		chunk := encoded[:chunkSize]
		encoded = encoded[chunkSize:]
		more := 0
		if len(encoded) > 0 {
			more = 1
		}
		fmt.Fprintf(&out, "\x1b_Ga=t,f=100,i=%d,q=2,m=%d;%s\x1b\\", imageID, more, chunk)
	}
	return out.String()
}

func drawHUD(grid [][]tile, hero player, foes []enemy, gearDrops []gearDrop) {
	segments := []hudSegment{
		{fmt.Sprintf("HP %d/%d", hero.Health, hero.MaxHealth), "\x1b[38;2;232;103;103m"},
		{"   ", ""},
		{fmt.Sprintf("DMG %d", effectiveDamage(hero)), "\x1b[38;2;222;222;210m"},
		{"   ", ""},
		{"WPN ", "\x1b[38;2;184;204;221m"},
		{equippedWeaponName(hero.weapon), weaponTierColor(hero.weapon.Tier)},
		{"   ARM ", "\x1b[38;2;184;204;221m"},
		{equippedArmourName(hero.armour), weaponTierColor(hero.armour.Tier)},
		{fmt.Sprintf(" DEF %d", hero.Armour), "\x1b[38;2;222;222;210m"},
		{"   ", ""},
		{fmt.Sprintf("XP %d", hero.Experience), "\x1b[38;2;232;193;104m"},
		{"   ", ""},
		{fmt.Sprintf("LEVEL %d", hero.Level), "\x1b[38;2;232;193;104m"},
		{"   ", ""},
		{fmt.Sprintf("ENEMIES %d", len(foes)), "\x1b[38;2;220;120;103m"},
	}
	drawHUDLine(grid, 0, segments)
	if len(grid) < 2 {
		return
	}
	itemSegments := []hudSegment{
		{text: "DROP: none", color: "\x1b[38;2;184;204;221m"},
		{text: "   [I: ITEMS / PAUSE]", color: "\x1b[38;2;232;193;104m"},
	}
	if drop, ok := nearestGearDrop(hero, gearDrops); ok && gearDropInRange(hero, drop) {
		if drop.isArmour {
			itemSegments = []hudSegment{
				{text: "DROP: ", color: "\x1b[38;2;184;204;221m"},
				{text: drop.armour.Name, color: weaponTierColor(drop.armour.Tier)},
				{text: fmt.Sprintf(" %s DEF %d  [SPACE nearby]", weaponTierName(drop.armour.Tier), drop.armour.Defence), color: "\x1b[38;2;184;204;221m"},
			}
		} else {
			itemSegments = []hudSegment{
				{text: "DROP: ", color: "\x1b[38;2;184;204;221m"},
				{text: drop.weapon.Name, color: weaponTierColor(drop.weapon.Tier)},
				{text: fmt.Sprintf(" %s DMG %d  [SPACE nearby]", weaponTierName(drop.weapon.Tier), drop.weapon.Damage), color: "\x1b[38;2;184;204;221m"},
			}
		}
	}
	if len(itemSegments) > 1 && itemSegments[0].text == "DROP: " {
		itemSegments = append(itemSegments, hudSegment{text: "   [I: ITEMS / PAUSE]", color: "\x1b[38;2;232;193;104m"})
	}
	itemSegments = append([]hudSegment{{text: fmt.Sprintf("POTIONS %d [1: USE]   ", hero.potions), color: "\x1b[38;2;115;207;174m"}}, itemSegments...)
	drawHUDLine(grid, 1, itemSegments)
}

func drawHUDLine(grid [][]tile, row int, segments []hudSegment) {
	x := 1
	for _, segment := range segments {
		for _, glyph := range segment.text {
			if x >= len(grid[row]) {
				return
			}
			grid[row][x] = tile{glyph, segment.color}
			x++
		}
	}
}

func drawGearOverlay(grid [][]tile, hero player, drop gearDrop) {
	if len(grid) < 15 || len(grid[0]) < 14 {
		return
	}
	panelWidth := min(34, len(grid[0])-2)
	left := len(grid[0]) - panelWidth - 1
	top := 3
	if top+12 >= len(grid) {
		top = max(2, len(grid)-13)
	}
	borderColor := "\x1b[38;2;120;126;138m"
	drawWeaponPanelBorder(grid, top, left, panelWidth, borderColor)
	line := func(row int, segments ...hudSegment) {
		drawPanelLine(grid, row, left, panelWidth, borderColor, segments)
	}
	if drop.isArmour {
		line(top+1, hudSegment{text: " ARMOUR COMPARISON", color: "\x1b[38;2;230;221;190m"})
		line(top+2, hudSegment{text: " Found: ", color: "\x1b[38;2;184;204;221m"}, hudSegment{text: drop.armour.Name, color: weaponTierColor(drop.armour.Tier)})
		line(top+3, hudSegment{text: " Rarity: " + weaponTierName(drop.armour.Tier), color: weaponTierColor(drop.armour.Tier)})
		line(top+4, hudSegment{text: fmt.Sprintf(" Defence: %d", drop.armour.Defence), color: "\x1b[38;2;222;222;210m"})
		line(top+5, hudSegment{text: " Equipped: " + equippedArmourName(hero.armour), color: weaponTierColor(hero.armour.Tier)})
		line(top+6, hudSegment{text: " Rarity: " + weaponTierName(hero.armour.Tier), color: weaponTierColor(hero.armour.Tier)})
		line(top+7, hudSegment{text: fmt.Sprintf(" Defence: %d", hero.Armour), color: "\x1b[38;2;222;222;210m"})
		line(top+8, hudSegment{text: fmt.Sprintf(" Defence change: %+d", drop.armour.Defence-hero.Armour), color: "\x1b[38;2;232;193;104m"})
		line(top+9, hudSegment{text: " Press SPACE to equip", color: "\x1b[38;2;184;204;221m"})
		drawWeaponPanelBorder(grid, top+10, left, panelWidth, borderColor)
		return
	}
	line(top+1, hudSegment{text: " WEAPON COMPARISON", color: "\x1b[38;2;230;221;190m"})
	line(top+2, hudSegment{text: " Found: ", color: "\x1b[38;2;184;204;221m"}, hudSegment{text: drop.weapon.Name, color: weaponTierColor(drop.weapon.Tier)})
	line(top+3, hudSegment{text: " Rarity: " + weaponTierName(drop.weapon.Tier), color: weaponTierColor(drop.weapon.Tier)})
	line(top+4, hudSegment{text: fmt.Sprintf(" Weapon DMG: %d", drop.weapon.Damage), color: "\x1b[38;2;222;222;210m"})
	line(top+5, hudSegment{text: fmt.Sprintf(" Total DMG: %d", hero.Damage+drop.weapon.Damage), color: "\x1b[38;2;232;193;104m"})
	line(top+6, hudSegment{text: " Equipped: " + equippedWeaponName(hero.weapon), color: weaponTierColor(hero.weapon.Tier)})
	line(top+7, hudSegment{text: " Rarity: " + weaponTierName(hero.weapon.Tier), color: weaponTierColor(hero.weapon.Tier)})
	line(top+8, hudSegment{text: fmt.Sprintf(" Weapon DMG: %d", hero.weapon.Damage), color: "\x1b[38;2;222;222;210m"})
	line(top+9, hudSegment{text: fmt.Sprintf(" Total DMG: %d", effectiveDamage(hero)), color: "\x1b[38;2;232;193;104m"})
	difference := hero.Damage + drop.weapon.Damage - effectiveDamage(hero)
	line(top+10, hudSegment{text: fmt.Sprintf(" Damage change: %+d", difference), color: "\x1b[38;2;232;193;104m"})
	line(top+11, hudSegment{text: " Press SPACE to pick up", color: "\x1b[38;2;184;204;221m"})
	drawWeaponPanelBorder(grid, top+12, left, panelWidth, borderColor)
}

func drawEquipmentPanel(grid [][]tile, hero player) {
	if len(grid) < 19 || len(grid[0]) < 56 {
		return
	}
	panelWidth := min(70, len(grid[0])-2)
	left := (len(grid[0]) - panelWidth) / 2
	top := (len(grid) - 18) / 2
	borderColor := "\x1b[38;2;120;126;138m"
	drawWeaponPanelBorder(grid, top, left, panelWidth, borderColor)
	line := func(offset int, segments ...hudSegment) {
		drawPanelLine(grid, top+offset, left, panelWidth, borderColor, segments)
	}
	line(1, hudSegment{text: " EQUIPPED ITEMS — PAUSED", color: "\x1b[38;2;230;221;190m"})
	weaponColor := weaponTierColor(hero.weapon.Tier)
	armourColor := weaponTierColor(hero.armour.Tier)
	centerSlot := left + (panelWidth-20)/2
	drawEquipmentSlot(grid, centerSlot, top+2, 20, "HELMET", "Not equipped", "", "\x1b[38;2;184;204;221m", []string{"^^", "||"})
	drawEquipmentSlot(grid, left+2, top+6, 20, "WEAPON", equippedWeaponName(hero.weapon), fmt.Sprintf("DMG %d", hero.weapon.Damage), weaponColor, []string{" /", "/|"})
	drawEquipmentSlot(grid, centerSlot, top+6, 20, "CHEST", equippedArmourName(hero.armour), fmt.Sprintf("DEF %d", hero.Armour), armourColor, []string{"[#", "##"})
	drawEquipmentSlot(grid, left+panelWidth-22, top+6, 20, "OFF-HAND", "Not equipped", "", "\x1b[38;2;184;204;221m", []string{"[ ]", "|#|"})
	drawEquipmentSlot(grid, centerSlot, top+10, 20, "BELT", "Not equipped", "", "\x1b[38;2;184;204;221m", []string{"===", "|||"})
	line(15, hudSegment{text: fmt.Sprintf(" TOTAL DAMAGE %d", effectiveDamage(hero)), color: "\x1b[38;2;232;193;104m"}, hudSegment{text: fmt.Sprintf("     TOTAL DEFENCE %d", hero.Armour), color: "\x1b[38;2;222;222;210m"})
	line(16, hudSegment{text: " Press I to close and resume", color: "\x1b[38;2;184;204;221m"})
	drawWeaponPanelBorder(grid, top+17, left, panelWidth, borderColor)
}

func drawEquipmentSlot(grid [][]tile, x, y, width int, title, name, stats, color string, art []string) {
	borderColor := "\x1b[38;2;120;126;138m"
	drawWeaponPanelBorder(grid, y, x, width, borderColor)
	drawPanelLine(grid, y+1, x, width, borderColor, []hudSegment{{text: "     " + title, color: borderColor}})
	drawPanelLine(grid, y+2, x, width, borderColor, []hudSegment{{text: "     " + name, color: color}})
	if stats != "" {
		drawPanelLine(grid, y+3, x, width, borderColor, []hudSegment{{text: "     " + stats, color: color}})
	} else {
		drawPanelLine(grid, y+3, x, width, borderColor, nil)
	}
	drawWeaponPanelBorder(grid, y+4, x, width, borderColor)
	for row, artLine := range art {
		for column, glyph := range artLine {
			if row+1 >= 4 || column >= 3 || glyph == ' ' || y+row+1 >= len(grid) || x+column+1 >= len(grid[y+row+1]) {
				continue
			}
			grid[y+row+1][x+column+1] = tile{glyph, color}
		}
	}
}

func drawPanelLine(grid [][]tile, row, left, width int, borderColor string, segments []hudSegment) {
	if row < 0 || row >= len(grid) || left < 0 || left+width > len(grid[row]) {
		return
	}
	grid[row][left] = tile{'|', borderColor}
	grid[row][left+width-1] = tile{'|', borderColor}
	for offset := 1; offset < width-1; offset++ {
		grid[row][left+offset] = tile{' ', ""}
	}
	column := left + 1
	limit := left + width - 1
	for _, segment := range segments {
		for _, glyph := range segment.text {
			if column >= limit {
				return
			}
			grid[row][column] = tile{glyph, segment.color}
			column++
		}
	}
}

func drawWeaponPanelBorder(grid [][]tile, row, left, width int, color string) {
	if row < 0 || row >= len(grid) || left < 0 || left+width > len(grid[row]) {
		return
	}
	for offset := 0; offset < width; offset++ {
		glyph := '-'
		if offset == 0 || offset == width-1 {
			glyph = '+'
		}
		grid[row][left+offset] = tile{glyph, color}
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
		hero.armour = drop.armour
		hero.Armour = drop.armour.Defence
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

func effectiveDamage(hero player) int {
	return max(hero.Damage, 0) + max(hero.weapon.Damage, 0)
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

func drawGearDrop(grid [][]tile, drop gearDrop, left, right, top, bottom int) {
	if drop.x <= left || drop.x >= right || drop.y <= top || drop.y >= bottom {
		return
	}
	glyph, tier := 'w', drop.weapon.Tier
	if drop.isArmour {
		glyph, tier = 'a', drop.armour.Tier
	}
	grid[drop.y][drop.x] = tile{glyph, weaponTierColor(tier)}
}

func spawnEnemyPacks(width, height, count int) []enemy {
	foes := make([]enemy, 0, count*3)
	for i := 0; i < count; i++ {
		foes = append(foes, spawnEnemyPack(width, height)...)
	}
	return foes
}

func spawnEnemyPack(width, height int) []enemy {
	packSize := 2 + rand.IntN(2)
	pack := make([]enemy, 0, packSize)
	for i := 0; i < packSize; i++ {
		tank := false
		brute := rand.IntN(4) == 0
		pack = append(pack, newEnemy(tank, brute))
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
	foe := enemy{Enemy: models.DefaultEnemy, tank: tank, brute: brute}
	if tank {
		foe.Enemy = models.TankEnemy
	} else if brute {
		foe.Enemy = models.BruteEnemy
	}
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
		if foe.Health <= 0 || now.Before(foe.nextMoveAt) {
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

func drawEnemy(grid [][]tile, foe enemy, left, right, top, bottom int) {
	if foe.Health <= 0 {
		return
	}
	if foe.tank {
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				x, y := foe.x+dx, foe.y+dy
				if x > left && x < right && y > top && y < bottom {
					grid[y][x] = tile{'t', "\x1b[38;2;220;92;83m"}
				}
			}
		}
		return
	}
	if foe.x > left && foe.x < right && foe.y > top && foe.y < bottom {
		glyph, color := 'g', "\x1b[38;2;220;92;83m"
		if foe.brute {
			glyph, color = 'B', "\x1b[38;2;227;153;74m"
		}
		grid[foe.y][foe.x] = tile{glyph, color}
	}
}

func drawEnemyDeathEffects(grid [][]tile, effects []enemyDeathEffect, left, right, top, bottom int, now time.Time) {
	for _, effect := range effects {
		phase := int(now.Sub(effect.startedAt) / (enemyDeathEffectDuration / 3))
		glyph, color := rune('X'), "\x1b[38;2;255;173;92m"
		positions := [][2]int{{effect.x, effect.y}}
		if effect.tank {
			positions = [][2]int{{effect.x, effect.y}, {effect.x + 1, effect.y}, {effect.x, effect.y + 1}, {effect.x + 1, effect.y + 1}}
		}
		switch phase {
		case 1:
			glyph, color = '*', "\x1b[38;2;255;220;130m"
			centerX, centerY := effect.x, effect.y
			if effect.tank {
				centerX++
				centerY++
			}
			positions = [][2]int{{centerX, centerY}, {centerX - 1, centerY}, {centerX + 1, centerY}, {centerX, centerY - 1}, {centerX, centerY + 1}}
		case 2:
			glyph, color = '.', "\x1b[38;2;170;153;129m"
		}
		for _, position := range positions {
			x, y := position[0], position[1]
			if x > left && x < right && y > top && y < bottom {
				grid[y][x] = tile{glyph, color}
			}
		}
	}
}

func levelUp(hero *player) {
	for hero.Level < len(models.LevelThreshold) && hero.Experience >= models.LevelThreshold[hero.Level] {
		hero.Level++
		hero.MaxHealth += models.PlayerStatGrowth.MaxHealth
		hero.Damage += models.PlayerStatGrowth.Damage
	}
}

func resolveBattle(hero *player, foes []enemy, now time.Time) (int, []int) {
	if hero.Health <= 0 || now.Sub(hero.lastHit) < 500*time.Millisecond {
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

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
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

func drawWarrior(grid [][]tile, x, y, left, right, top, bottom int, facing byte, weapon items.Weapon, idleFrame bool, swingFrame int) {
	// Helmet, armored torso with weapon, and boots. The player position is the sprite center.
	chest := '#'
	head := '^'
	leftArm, rightArm := '<', '>'
	swordColumn := 3
	legs := " / \\ "
	switch facing {
	case 's':
		head = 'v'
	case 'a':
		head, swordColumn = '<', 1
	case 'd':
		head = '>'
	case 'w':
		head, leftArm, rightArm, legs = '^', '/', '\\', "/   \\"
	case 'q':
		head, swordColumn = '↖', 1
	case 'e':
		head = '↗'
	case 'z':
		head, swordColumn = '↙', 1
	case 'c':
		head = '↘'
	}
	if idleFrame {
		chest = '+'
		if facing == 'a' || facing == 'w' || facing == 'q' || facing == 'z' {
			rightArm = '\\'
		} else {
			leftArm = '\\'
		}
		if facing == 'w' || facing == 'q' || facing == 'e' {
			legs = " / \\ "
		} else {
			legs = "/   \\"
		}
	}
	torso := []rune{leftArm, '|', chest, '|', rightArm}
	torso[swordColumn] = weaponGlyph(weapon)
	sprite := []string{
		string([]rune{' ', '/', head, '\\', ' '}),
		string(torso),
		legs,
	}
	colors := []string{
		"\x1b[38;2;220;183;105m",
		"\x1b[38;2;226;220;195m",
		"\x1b[38;2;160;166;174m",
	}
	for row, line := range sprite {
		gridY := y - 1 + row
		if gridY <= top || gridY >= bottom {
			continue
		}
		for col, glyph := range line {
			gridX := x - 2 + col
			if gridX <= left || gridX >= right {
				continue
			}
			color := colors[row]
			if col == swordColumn {
				color = weaponTierColor(weapon.Tier)
			}
			grid[gridY][gridX] = tile{glyph, color}
		}
	}
	if swingFrame >= 0 {
		slashX, slashY, slashGlyph := x, y, rune('*')
		switch facing {
		case 'a':
			slashX, slashGlyph = x-3, '-'
		case 'd':
			slashX, slashGlyph = x+3, '-'
		case 'w':
			slashY, slashGlyph = y-2, '|'
		case 's':
			slashY, slashGlyph = y+2, '|'
		case 'q':
			slashX, slashY, slashGlyph = x-3, y-1, '\\'
		case 'e':
			slashX, slashY, slashGlyph = x+3, y-1, '/'
		case 'z':
			slashX, slashY, slashGlyph = x-3, y+1, '/'
		case 'c':
			slashX, slashY, slashGlyph = x+3, y+1, '\\'
		}
		if swingFrame == 0 {
			slashGlyph = '.'
		} else if swingFrame == 2 {
			slashGlyph = '*'
		}
		if slashX > left && slashX < right && slashY > top && slashY < bottom {
			grid[slashY][slashX] = tile{slashGlyph, "\x1b[38;2;255;227;139m"}
		}
	}
}

func weaponGlyph(weapon items.Weapon) rune {
	switch weapon.Name {
	case "Bow":
		return ')'
	case "Spear":
		return '>'
	case "Dagger":
		return '!'
	case "EnchantedSword":
		return '*'
	case "FlamingBow":
		return '}'
	case "CursedSword":
		return '\u2021'
	case "Excalibur":
		return '+'
	case "Trident":
		return 'Y'
	default:
		return '\u2020'
	}
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
	for _, effect := range effects {
		if !deathEffectTouchesPlayer(effect, x, y) {
			continue
		}
		if !deathEffectTouchesPlayer(effect, previousX, previousY) || deathEffectCenterDistance(effect, x, y) <= deathEffectCenterDistance(effect, previousX, previousY) {
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
