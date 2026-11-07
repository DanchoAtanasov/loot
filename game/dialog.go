package game

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

type dialogueLine struct {
	speaker string
	text    string
}

type dialogueBook map[string][]dialogueLine

func loadDialogues(path string) (dialogueBook, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	book := make(dialogueBook)
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section == "" {
			continue
		}
		speaker, text, found := strings.Cut(line, ":")
		if !found {
			speaker = ""
			text = line
		}
		book[section] = append(book[section], dialogueLine{
			speaker: strings.TrimSpace(speaker),
			text:    strings.TrimSpace(text),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(book["intro"]) == 0 {
		return nil, fmt.Errorf("dialogue file %q has no [intro] lines", path)
	}
	if len(book["quest"]) == 0 {
		book["quest"] = []dialogueLine{{speaker: "Wizard", text: "The boss is still somewhere in this dungeon. Keep searching."}}
	}
	if len(book["quest_complete"]) == 0 {
		book["quest_complete"] = []dialogueLine{{speaker: "Wizard", text: "You defeated the boss. The dungeon is safe again."}}
	}
	if len(book["prompt"]) == 0 {
		book["prompt"] = []dialogueLine{{text: "Press Space to talk"}}
	}
	if len(book["quest_status"]) == 0 {
		book["quest_status"] = []dialogueLine{{text: "Talk to the wizard"}}
	}
	if len(book["quest_complete_status"]) == 0 {
		book["quest_complete_status"] = []dialogueLine{{text: "Return to the wizard"}}
	}
	return book, nil
}

func renderDialogue(heroX, heroY, wizardX, wizardY, width int, line *dialogueLine, prompt []dialogueLine, nearWizard bool) string {
	var frame bytes.Buffer
	var current dialogueLine
	if line != nil {
		current = *line
	} else if nearWizard && len(prompt) > 0 {
		current = prompt[0]
	} else {
		return ""
	}
	centerX, row := wizardX, max(wizardY-wizardImageRows/2-1, 3)
	color := "\x1b[1;38;2;255;226;150m"
	if strings.EqualFold(current.speaker, "hero") {
		centerX, row = heroX, max(heroY-heroImageRows/2-1, 3)
		color = "\x1b[1;38;2;140;220;255m"
	}
	maxLineWidth := max(min(width-4, 64), 1)
	lines := wrapDialogue(current.text, maxLineWidth)
	startRow := max(row-len(lines)+1, 3)
	for index, text := range lines {
		drawSpeechText(&frame, text, color, centerX, startRow+index, width)
	}
	return frame.String()
}

func wrapDialogue(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, 2)
	line := words[0]
	for _, word := range words[1:] {
		if len(line)+1+len(word) <= width {
			line += " " + word
			continue
		}
		lines = append(lines, line)
		line = word
	}
	lines = append(lines, line)
	return lines
}

func renderQuestOverlay(width int, complete bool, book dialogueBook) string {
	heading, section, color := "QUEST", "quest_status", "\x1b[1;38;2;255;218;132m"
	if complete {
		heading, section, color = "QUEST COMPLETE", "quest_complete_status", "\x1b[1;38;2;115;207;174m"
	}
	objective := dialogueText(book, section, "")
	headingColumn := max(width-len(heading)+1, 1)
	objectiveColumn := max(width-len(objective)+1, 1)
	return fmt.Sprintf("\x1b[3;%dH%s%s\x1b[4;%dH%s%s\x1b[0m", headingColumn, color, heading, objectiveColumn, color, objective)
}

func dialogueText(book dialogueBook, section, fallback string) string {
	lines := book[section]
	if len(lines) == 0 || lines[0].text == "" {
		return fallback
	}
	return lines[0].text
}

func drawSpeechText(frame *bytes.Buffer, message, color string, centerX, row, width int) {
	column := min(max(centerX-len(message)/2+1, 1), max(width-len(message)+1, 1))
	fmt.Fprintf(frame, "\x1b[%d;%dH%s%s\x1b[0m", row, column, color, message)
}
