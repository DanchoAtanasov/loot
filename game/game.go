package game

import (
	"bufio"
	"bytes"
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
	framesPerSecond = 60
	windowSizeIOCTL = 0x40087468 // TIOCGWINSZ on macOS
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
	x, y int
	dx   int
	dy   int
}

type player struct {
	models.Player
	weapon   items.Weapon
	x, y     int
	facing   byte
	lastMove time.Time
	lastHit  time.Time
}

type healthPickup struct {
	x int
	y int
}

type weaponDrop struct {
	weapon items.Weapon
	x, y   int
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

	width, height := readTerminalSize()
	playerX, playerY := chamberCenter(width, height)
	hero := player{
		Player:   models.DefaultPlayer,
		weapon:   items.Weapon{},
		x:        playerX,
		y:        playerY,
		facing:   's',
		lastMove: time.Now(),
	}
	foes := []enemy{newEnemy(width, height)}
	pickups := make([]healthPickup, 0)
	weaponDrops := make([]weaponDrop, 0)
	keys := make(chan keyEvent, 32)
	heldKeys := make(map[rune]bool)
	lastStep := time.Time{}
	go readKeys(keys)

	frameDuration := time.Second / framesPerSecond
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()
	enemyTicker := time.NewTicker(180 * time.Millisecond)
	defer enemyTicker.Stop()
	enemySpawnTicker := time.NewTicker(2 * time.Second)
	defer enemySpawnTicker.Stop()
	pickupTicker := time.NewTicker(10 * time.Second)
	defer pickupTicker.Stop()

	for {
		// Refresh the bounds and keep the characters inside the chamber.
		width, height = readTerminalSize()
		hero.x, hero.y = keepPlayerInChamber(hero.x, hero.y, width, height)

		// Resolve close-range hits before drawing the next frame.
		now := time.Now()
		for i := range foes {
			foes[i].x, foes[i].y = keepEnemyInChamber(foes[i].x, foes[i].y, width, height)
			wasAlive := foes[i].Health > 0
			resolveBattle(&hero, &foes[i], now)
			if wasAlive && foes[i].Health <= 0 && rand.IntN(100) < 30 {
				dropX, dropY := weaponDropPosition(foes[i], hero, width, height)
				weaponDrops = append(weaponDrops, weaponDrop{weapon: items.DropItem(), x: dropX, y: dropY})
			}
		}
		alive := foes[:0]
		for _, foe := range foes {
			if foe.Health > 0 {
				alive = append(alive, foe)
			}
		}
		foes = alive
		pickups = collectHealthPickups(&hero, pickups)

		// Check if character should level up
		levelUp(&hero)

		// Switch the warrior's pose while idle.
		idleTime := time.Since(hero.lastMove)
		idleFrame := idleTime > 400*time.Millisecond && int(idleTime/(500*time.Millisecond))%2 == 1
		fmt.Print(renderDungeon(hero, width, height, idleFrame, foes, pickups, weaponDrops))

		// Handle shutdown, player input, frame timing, or enemy movement.
		select {
		case <-ctx.Done():
			return
		case event := <-keys:
			if event.key == 3 && event.pressed {
				stop()
				continue
			}
			if event.key == ' ' && event.pressed {
				weaponDrops = pickupWeaponDrop(&hero, weaponDrops)
				continue
			}
			if event.tracked {
				heldKeys[event.key] = event.pressed
				continue
			}
			if hero.Health > 0 && event.pressed {
				hero.x, hero.y, hero.facing = movePlayer(byte(event.key), hero.x, hero.y, hero.facing, hero.MovementSpeed, width, height)
				hero.lastMove = time.Now()
			}
		case now := <-ticker.C:
			if hero.Health > 0 && now.Sub(lastStep) >= 120*time.Millisecond {
				nextX, nextY, nextFacing := movePlayerHeld(heldKeys, hero.x, hero.y, hero.facing, hero.MovementSpeed, width, height)
				if nextX != hero.x || nextY != hero.y {
					hero.x, hero.y, hero.facing = nextX, nextY, nextFacing
					hero.lastMove = now
				}
				lastStep = now
			}
		case <-enemyTicker.C:
			for i := range foes {
				foes[i].step(width, height)
			}
		case <-enemySpawnTicker.C:
			foes = append(foes, newEnemy(width, height))
		case <-pickupTicker.C:
			pickups = append(pickups, spawnHealthPickup(width, height))
		}
	}
}

func renderDungeon(hero player, width, height int, idleFrame bool, foes []enemy, pickups []healthPickup, weaponDrops []weaponDrop) string {
	grid := make([][]tile, height)
	for y := range grid {
		grid[y] = make([]tile, width)
		for x := range grid[y] {
			grid[y][x] = tile{' ', ""}
		}
	}

	// Carve a broad stone chamber with a short passage at each cardinal wall.
	left, right := 2, width-3
	top, bottom := 1, height-2
	if right-left < 8 || bottom-top < 6 {
		left, right, top, bottom = 0, width-1, 0, height-1
	}
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			border := x == left || x == right || y == top || y == bottom
			if border {
				grid[y][x] = tile{'▓', "\x1b[38;2;105;111;119m"}
			} else {
				grid[y][x] = tile{'·', "\x1b[38;2;66;71;78m"}
			}
		}
	}

	if right-left >= 12 && bottom-top >= 8 {
		midX, midY := (left+right)/2, (top+bottom)/2
		for _, p := range [][2]int{{left, midY}, {right, midY}, {midX, top}, {midX, bottom}} {
			grid[p[1]][p[0]] = tile{' ', ""}
		}
		for _, p := range [][2]int{{left + 4, top + 3}, {right - 4, top + 3}, {left + 4, bottom - 3}, {right - 4, bottom - 3}} {
			grid[p[1]][p[0]] = tile{'╬', "\x1b[38;2;145;132;103m"}
		}
		// An old flagstone circle marks the center of the room.
		for _, p := range [][2]int{{midX, midY}, {midX - 1, midY}, {midX + 1, midY}, {midX, midY - 1}, {midX, midY + 1}} {
			grid[p[1]][p[0]] = tile{'◈', "\x1b[38;2;136;113;78m"}
		}
	}
	drawHUD(grid, hero, foes, weaponDrops)
	for _, pickup := range pickups {
		drawHealthPickup(grid, pickup, left, right, top, bottom)
	}
	for _, foe := range foes {
		drawEnemy(grid, foe, left, right, top, bottom)
	}
	for _, drop := range weaponDrops {
		drawWeaponDrop(grid, drop, left, right, top, bottom)
	}
	drawWarrior(grid, hero.x, hero.y, left, right, top, bottom, hero.facing, idleFrame)

	var frame bytes.Buffer
	frame.WriteString("\x1b[?2026h\x1b[H")
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
	return frame.String()
}

func drawHUD(grid [][]tile, hero player, foes []enemy, weaponDrops []weaponDrop) {
	segments := []hudSegment{
		{fmt.Sprintf("HP %d/%d", hero.Health, hero.MaxHealth), "\x1b[38;2;232;103;103m"},
		{"   ", ""},
		{fmt.Sprintf("DMG %d", hero.Damage), "\x1b[38;2;222;222;210m"},
		{"   ", ""},
		{"WPN ", "\x1b[38;2;184;204;221m"},
		{equippedWeaponName(hero.weapon), weaponTierColor(hero.weapon.Tier)},
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
	itemSegments := []hudSegment{{text: "DROP: none", color: "\x1b[38;2;184;204;221m"}}
	if drop, ok := nearestWeaponDrop(hero, weaponDrops); ok && weaponDropInRange(hero, drop) {
		itemSegments = []hudSegment{
			{text: "DROP: ", color: "\x1b[38;2;184;204;221m"},
			{text: drop.weapon.Name, color: weaponTierColor(drop.weapon.Tier)},
			{text: fmt.Sprintf(" %s DMG %d  [SPACE nearby]", weaponTierName(drop.weapon.Tier), drop.weapon.Damage), color: "\x1b[38;2;184;204;221m"},
		}
	}
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
		if touching && hero.Health < hero.MaxHealth {
			hero.Health = min(hero.Health+10, hero.MaxHealth)
			continue
		}
		remaining = append(remaining, pickup)
	}
	return remaining
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
		if abs(x-hero.x) <= 2 && abs(y-hero.y) <= 1 {
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

func pickupWeaponDrop(hero *player, drops []weaponDrop) []weaponDrop {
	for i, drop := range drops {
		if abs(hero.x-drop.x) <= 2 && abs(hero.y-drop.y) <= 1 {
			hero.weapon = drop.weapon
			hero.Damage = drop.weapon.Damage
			return append(drops[:i], drops[i+1:]...)
		}
	}
	return drops
}

func nearestWeaponDrop(hero player, drops []weaponDrop) (weaponDrop, bool) {
	if len(drops) == 0 {
		return weaponDrop{}, false
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

func weaponDropInRange(hero player, drop weaponDrop) bool {
	return abs(hero.x-drop.x) <= 2 && abs(hero.y-drop.y) <= 1
}

func equippedWeaponName(weapon items.Weapon) string {
	if weapon.Name == "" {
		return "None"
	}
	return weapon.Name
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

func drawWeaponDrop(grid [][]tile, drop weaponDrop, left, right, top, bottom int) {
	if drop.x <= left || drop.x >= right || drop.y <= top || drop.y >= bottom {
		return
	}
	grid[drop.y][drop.x] = tile{'w', weaponTierColor(drop.weapon.Tier)}
}

func drawHealthPickup(grid [][]tile, pickup healthPickup, left, right, top, bottom int) {
	if pickup.x > left && pickup.x < right && pickup.y > top && pickup.y < bottom {
		grid[pickup.y][pickup.x] = tile{'+', "\x1b[38;2;103;222;130m"}
	}
}

func newEnemy(width, height int) enemy {
	left, right, top, bottom := chamberBounds(width, height)
	minX, maxX := left+1, right-1
	minY, maxY := top+1, bottom-1
	if minX > maxX {
		minX, maxX = left, right
	}
	if minY > maxY {
		minY, maxY = top, bottom
	}

	foe := enemy{Enemy: models.DefaultEnemy}
	switch rand.IntN(4) {
	case 0: // Enter from the top.
		foe.x, foe.y = randomBetween(minX, maxX), minY
		foe.dx, foe.dy = randomDirection(), 1
	case 1: // Enter from the bottom.
		foe.x, foe.y = randomBetween(minX, maxX), maxY
		foe.dx, foe.dy = randomDirection(), -1
	case 2: // Enter from the left.
		foe.x, foe.y = minX, randomBetween(minY, maxY)
		foe.dx, foe.dy = 1, randomDirection()
	case 3: // Enter from the right.
		foe.x, foe.y = maxX, randomBetween(minY, maxY)
		foe.dx, foe.dy = -1, randomDirection()
	}
	return foe
}

func randomBetween(minimum, maximum int) int {
	return minimum + rand.IntN(maximum-minimum+1)
}

func randomDirection() int {
	if rand.IntN(2) == 0 {
		return -1
	}
	return 1
}

func (e *enemy) step(width, height int) {
	if e.Health <= 0 {
		return
	}
	left, right, top, bottom := chamberBounds(width, height)
	minX, maxX := left+1, right-1
	minY, maxY := top+1, bottom-1
	if e.x+2*e.dx < minX || e.x+2*e.dx > maxX {
		e.dx *= -1
	}
	if e.y+e.dy < minY || e.y+e.dy > maxY {
		e.dy *= -1
	}
	e.x += 2 * e.dx
	e.y += e.dy
	e.x, e.y = keepEnemyInChamber(e.x, e.y, width, height)
}

func keepEnemyInChamber(x, y, width, height int) (int, int) {
	left, right, top, bottom := chamberBounds(width, height)
	if right-left < 2 || bottom-top < 2 {
		return chamberCenter(width, height)
	}
	return min(max(x, left+1), right-1), min(max(y, top+1), bottom-1)
}

func drawEnemy(grid [][]tile, foe enemy, left, right, top, bottom int) {
	if foe.Health > 0 && foe.x > left && foe.x < right && foe.y > top && foe.y < bottom {
		grid[foe.y][foe.x] = tile{'g', "\x1b[38;2;220;92;83m"}
	}
}

func levelUp(hero *player) {
	for hero.Level < len(models.LevelThreshold) && hero.Experience >= models.LevelThreshold[hero.Level] {
		hero.Level++
	}
}

func resolveBattle(hero *player, foe *enemy, now time.Time) bool {
	if hero.Health <= 0 || foe.Health <= 0 || now.Sub(hero.lastHit) < 500*time.Millisecond {
		return false
	}
	if abs(hero.x-foe.x) > 2 || abs(hero.y-foe.y) > 1 {
		return false
	}

	foe.Health -= max(hero.Damage, 0)
	hero.Health = max(hero.Health-max(foe.Damage, 0), 0)
	hero.lastHit = now
	if foe.Health <= 0 {
		hero.Experience += foe.ExperienceOnKill
	}
	return true
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func drawWarrior(grid [][]tile, x, y, left, right, top, bottom int, facing byte, idleFrame bool) {
	// Helmet, armored torso with sword, and boots. The player position is the sprite center.
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
	torso[swordColumn] = '†'
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
			if glyph == '†' {
				color = "\x1b[38;2;184;204;221m"
			}
			grid[gridY][gridX] = tile{glyph, color}
		}
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

func movePlayer(key byte, x, y int, facing byte, movementSpeed, width, height int) (int, int, byte) {
	step := max(movementSpeed, 1)
	horizontalStep := horizontalMovementScale() * step
	switch key {
	case 'w', 'W':
		y -= step
		facing = 'w'
	case 's', 'S':
		y += step
		facing = 's'
	case 'a', 'A':
		x -= horizontalStep
		facing = 'a'
	case 'd', 'D':
		x += horizontalStep
		facing = 'd'
	}
	x, y = keepPlayerInChamber(x, y, width, height)
	return x, y, facing
}

func movePlayerHeld(held map[rune]bool, x, y int, facing byte, movementSpeed, width, height int) (int, int, byte) {
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
	x += dx * horizontalMovementScale() * step
	y += dy * step
	x, y = keepPlayerInChamber(x, y, width, height)
	return x, y, facing
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
	// Accept both plain CSI arrows (for example, ESC [ A) and the
	// modifier-bearing form used by some terminals (ESC [ 1 ; 1 A).
	if terminator == 'A' {
		return keyEvent{key: 'w', pressed: true}, true
	}
	if terminator == 'B' {
		return keyEvent{key: 's', pressed: true}, true
	}
	if terminator == 'C' {
		return keyEvent{key: 'd', pressed: true}, true
	}
	if terminator == 'D' {
		return keyEvent{key: 'a', pressed: true}, true
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
	if code >= 'a' && code <= 'z' {
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
