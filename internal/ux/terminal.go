package ux

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Plain reports whether output should avoid ANSI styling and Unicode glyphs.
func Plain(flag bool) bool {
	if flag {
		return true
	}
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return true
	}
	return runtime.GOOS == "windows" && !term.IsTerminal(int(os.Stdout.Fd()))
}

func Badge(label string, color lipgloss.Color, plain bool) string {
	label = strings.TrimSpace(label)
	if Plain(plain) {
		return "[" + label + "]"
	}
	return lipgloss.NewStyle().
		Background(color).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 1).
		Render(" " + label + " ")
}

func Style(style lipgloss.Style, text string, plain bool) string {
	if Plain(plain) {
		return text
	}
	return style.Render(text)
}

func Symbol(name string, plain bool) string {
	if Plain(plain) {
		switch name {
		case "check":
			return "OK"
		case "cross":
			return "X"
		case "dot":
			return "*"
		case "play":
			return ">"
		case "arrow":
			return ">"
		case "up":
			return "^"
		case "down":
			return "v"
		case "bar-filled":
			return "#"
		case "bar-empty":
			return "-"
		case "branch":
			return "\\-"
		case "dash":
			return "-"
		}
		return ""
	}

	switch name {
	case "check":
		return "✔"
	case "cross":
		return "✖"
	case "dot":
		return "●"
	case "play":
		return "▶"
	case "arrow":
		return "➔"
	case "up":
		return "▲"
	case "down":
		return "▼"
	case "bar-filled":
		return "█"
	case "bar-empty":
		return "░"
	case "branch":
		return "└─"
	case "dash":
		return "—"
	}
	return ""
}

func Confirm(prompt string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("confirmation required; rerun with --yes to proceed in non-interactive mode")
	}

	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}
