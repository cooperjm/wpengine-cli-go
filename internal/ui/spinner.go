package ui

import (
	"fmt"
	"os"
	"time"

	"wpengine-cli/internal/ux"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// RunWithSpinner displays an animated spinner with message while fn executes.
// Falls back to a direct fn() call when plain output is active or stderr is not a terminal.
func RunWithSpinner(message string, plain bool, fn func() error) error {
	if ux.Plain(plain) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return fn()
	}

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	style := lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	done := make(chan error, 1)

	go func() {
		done <- fn()
	}()

	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			fmt.Fprint(os.Stderr, "\r\033[K")
			return err
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\r%s %s", style.Render(frames[i%len(frames)]), message)
			i++
		}
	}
}
