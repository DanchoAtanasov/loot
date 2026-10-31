package game

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"strings"
	"time"
)

func renderLoadingScreen(width, height int) string {
	var frame strings.Builder
	frame.WriteString("\x1b[2J\x1b[H\x1b[?2026h")
	letters := map[rune][5]string{
		'S': {"█████", "█    ", "█████", "    █", "█████"},
		'I': {"█████", "  █  ", "  █  ", "  █  ", "█████"},
		'G': {"█████", "█    ", "█ ███", "█   █", "█████"},
		'K': {"█   █", "█  █ ", "███  ", "█  █ ", "█   █"},
		'L': {"█    ", "█    ", "█    ", "█    ", "█████"},
	}
	label := "SIGKILL"
	const letterWidth = 5
	const letterGap = 1
	labelWidth := len(label)*(letterWidth+letterGap) - letterGap
	labelHeight := 5
	status := "LOADING..."
	blockHeight := labelHeight + 2 + 1
	startRow := max((height-blockHeight)/2+1, 1)
	startColumn := max((width-labelWidth)/2+1, 1)
	for row := 0; row < labelHeight; row++ {
		line := make([]rune, 0, labelWidth)
		for index, letter := range label {
			if index > 0 {
				line = append(line, ' ')
			}
			line = append(line, []rune(letters[letter][row])...)
		}
		fmt.Fprintf(&frame, "\x1b[%d;%dH\x1b[1;38;2;236;87;87m%s", startRow+row, startColumn, string(line))
	}
	statusColumn := max((width-len(status))/2+1, 1)
	fmt.Fprintf(&frame, "\x1b[%d;%dH\x1b[0;38;2;160;166;174m%s", startRow+labelHeight+2, statusColumn, status)
	frame.WriteString("\x1b[0m\x1b[?2026l")
	return frame.String()
}

func renderDungeon(hero player, width, height, worldWidth, stage, runningFrame, swingFrame, transitionSeconds int, foes []enemy, pickups []healthPickup, gearDrops []gearDrop, deathEffects []enemyDeathEffect, bushes []bush, showEquipment bool) string {
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
	drawHUD(grid, hero, stage, foes, gearDrops)
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
	if drop, ok := nearestGearDrop(hero, gearDrops); ok && gearDropInRange(hero, drop) {
		drawHero, drawDrop := hero, drop
		drawHero.x -= viewLeft
		drawDrop.x -= viewLeft
		drawGearOverlay(grid, drawHero, drawDrop)
	}
	if showEquipment {
		drawEquipmentPanel(grid, hero)
	}
	if transitionSeconds > 0 {
		drawStageTransition(grid, transitionSeconds)
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
	if !showEquipment {
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
	imageID := 1
	if swingFrame >= 0 {
		imageID = 2 + swingFrame
	} else if runningFrame >= 0 {
		imageID = heroRunLeftMirroredImageID + runningFrame
	}
	facingRight := hero.facing == 'd' || hero.facing == 'e' || hero.facing == 'c'
	if facingRight {
		if swingFrame >= 0 {
			imageID += 5
		} else if runningFrame >= 0 {
			imageID = heroRunLeftImageID + runningFrame
		} else {
			imageID += 5
		}
	}
	heroScreenX := hero.x - viewLeft
	frame.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b_Ga=p,i=%d,p=1,c=%d,r=%d,q=2\x1b\\", max(hero.y-heroImageRows/2, 1), max(heroScreenX-heroImageCols/2, 1), imageID, heroImageCols, heroImageRows))
	return frame.String()
}

func drawStageTransition(grid [][]tile, seconds int) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	lines := []string{"STAGE CLEARED", fmt.Sprintf("NEXT STAGE IN %d", seconds)}
	centerY := len(grid) / 2
	for index, line := range lines {
		row := centerY + index
		if index == 0 {
			row--
		}
		if row < 2 || row >= len(grid) {
			continue
		}
		column := max((len(grid[row])-len(line))/2, 0)
		color := "\x1b[38;2;236;87;87m"
		if index == 1 {
			color = "\x1b[38;2;190;190;200m"
		}
		for offset, glyph := range line {
			if column+offset < len(grid[row]) {
				grid[row][column+offset] = tile{glyph, color}
			}
		}
	}
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

func drawHUD(grid [][]tile, hero player, stage int, foes []enemy, gearDrops []gearDrop) {
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
		{fmt.Sprintf("STAGE %d", stage), "\x1b[38;2;173;151;218m"},
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
	line(top+4, hudSegment{text: fmt.Sprintf(" Weapon DMG: %d  ATK SPD: %.1f/s", drop.weapon.Damage, weaponAttackSpeed(drop.weapon)), color: "\x1b[38;2;222;222;210m"})
	line(top+5, hudSegment{text: fmt.Sprintf(" Total DMG: %d", hero.Damage+drop.weapon.Damage), color: "\x1b[38;2;232;193;104m"})
	line(top+6, hudSegment{text: " Equipped: " + equippedWeaponName(hero.weapon), color: weaponTierColor(hero.weapon.Tier)})
	line(top+7, hudSegment{text: " Rarity: " + weaponTierName(hero.weapon.Tier), color: weaponTierColor(hero.weapon.Tier)})
	line(top+8, hudSegment{text: fmt.Sprintf(" Weapon DMG: %d  ATK SPD: %.1f/s", hero.weapon.Damage, weaponAttackSpeed(hero.weapon)), color: "\x1b[38;2;222;222;210m"})
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
