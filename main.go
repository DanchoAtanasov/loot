package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J") // alternate screen, hide cursor, clear
	defer fmt.Print("\x1b[?2026l\x1b[0m\x1b[?25h\x1b[?1049l")

	frameDuration := time.Second / framesPerSecond
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		fmt.Print(renderDungeon())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func renderDungeon() string {
	width, height := readTerminalSize()
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
