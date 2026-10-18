package game

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

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

type enemy struct {
	models.Enemy
	x, y      int
	dx        int
	dy        int
	respawnAt time.Time
}

type player struct {
	models.Player
	x, y     int
	facing   byte
	lastMove time.Time
	lastHit  time.Time
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

	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J") // alternate screen, hide cursor, clear
	defer fmt.Print("\x1b[?2026l\x1b[0m\x1b[?25h\x1b[?1049l")

	width, height := readTerminalSize()
	playerX, playerY := chamberCenter(width, height)
	hero := player{
		Player:   models.DefaultPlayer,
		x:        playerX,
		y:        playerY,
		facing:   's',
		lastMove: time.Now(),
	}
	foe := newEnemy(width, height)
	keys := make(chan byte, 16)
	go readKeys(keys)

	frameDuration := time.Second / framesPerSecond
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()
	enemyTicker := time.NewTicker(180 * time.Millisecond)
	defer enemyTicker.Stop()

	for {
		// Refresh the bounds and keep both characters inside the chamber.
		width, height = readTerminalSize()
		hero.x, hero.y = keepPlayerInChamber(hero.x, hero.y, width, height)
		foe.x, foe.y = keepEnemyInChamber(foe.x, foe.y, width, height)

		// Resolve close-range hits before drawing the next frame.
		resolveBattle(&hero, &foe, time.Now())

		// Check if character should level up
		levelUp(&hero)

		// Switch the warrior's pose while idle.
		idleTime := time.Since(hero.lastMove)
		idleFrame := idleTime > 400*time.Millisecond && int(idleTime/(500*time.Millisecond))%2 == 1
		fmt.Print(renderDungeon(hero, width, height, idleFrame, foe))

		// Handle shutdown, player input, frame timing, or enemy movement.
		select {
		case <-ctx.Done():
			return
		case key := <-keys:
			if hero.Health <= 0 {
				continue
			}
			nextX, nextY, nextFacing := movePlayer(key, hero.x, hero.y, hero.facing, hero.MovementSpeed, width, height)
			if nextX != hero.x || nextY != hero.y || nextFacing != hero.facing {
				hero.lastMove = time.Now()
			}
			hero.x, hero.y, hero.facing = nextX, nextY, nextFacing
		case <-ticker.C:
		case <-enemyTicker.C:
			foe.step(width, height)
		}
	}
}

func renderDungeon(hero player, width, height int, idleFrame bool, foe enemy) string {
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
	drawHUD(grid, hero, foe)
	drawEnemy(grid, foe, left, right, top, bottom)
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

func drawHUD(grid [][]tile, hero player, foe enemy) {
	segments := []struct {
		text  string
		color string
	}{
		{fmt.Sprintf("HP %d", hero.Health), "\x1b[38;2;232;103;103m"},
		{"   ", ""},
		{fmt.Sprintf("DMG %d", hero.Damage), "\x1b[38;2;222;222;210m"},
		{"   ", ""},
		{fmt.Sprintf("XP %d", hero.Experience), "\x1b[38;2;232;193;104m"},
		{"   ", ""},
		{fmt.Sprintf("LEVEL %d", hero.Level), "\x1b[38;2;232;193;104m"},
		{"   ", ""},
		{fmt.Sprintf("FOE HP %d", max(foe.Health, 0)), "\x1b[38;2;220;120;103m"},
	}
	x := 1
	for _, segment := range segments {
		for _, glyph := range segment.text {
			if x >= len(grid[0]) {
				return
			}
			grid[0][x] = tile{glyph, segment.color}
			x++
		}
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
		if !e.respawnAt.IsZero() && !time.Now().Before(e.respawnAt) {
			*e = newEnemy(width, height)
		}
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
	threshold := models.LevelThreshold[hero.Level]
	if !(hero.Experience >= threshold) {
		return
	}
	hero.Level += 1
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
		foe.respawnAt = now.Add(2 * time.Second)
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
	}
	if idleFrame {
		chest = '+'
		if facing == 'a' || facing == 'w' {
			rightArm = '\\'
		} else {
			leftArm = '\\'
		}
		if facing == 'w' {
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
	switch key {
	case 'w', 'W':
		y -= step
		facing = 'w'
	case 's', 'S':
		y += step
		facing = 's'
	case 'a', 'A':
		x -= 2 * step
		facing = 'a'
	case 'd', 'D':
		x += 2 * step
		facing = 'd'
	}
	x, y = keepPlayerInChamber(x, y, width, height)
	return x, y, facing
}

func readKeys(keys chan<- byte) {
	reader := bufio.NewReader(os.Stdin)
	for {
		key, err := reader.ReadByte()
		if err != nil {
			return
		}
		if key == 0x1b {
			_, _ = reader.ReadByte() // '['
			key, err = reader.ReadByte()
			if err != nil {
				return
			}
			switch key {
			case 'A':
				key = 'w'
			case 'B':
				key = 's'
			case 'C':
				key = 'd'
			case 'D':
				key = 'a'
			}
		}
		select {
		case keys <- key:
		default:
		}
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
