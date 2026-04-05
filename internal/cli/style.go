package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

// noColor is true when the NO_COLOR env var is set (any value), per https://no-color.org/.
var noColor = os.Getenv("NO_COLOR") != ""

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// render applies a lipgloss style, respecting NO_COLOR.
func render(style lipgloss.Style, s string) string {
	if noColor {
		return s
	}
	return style.Render(s)
}

func printHeader(version string) {
	fmt.Fprintf(os.Stderr, "\n  %s\n\n", render(titleStyle, "lumber "+version))
	fmt.Fprintf(os.Stderr, "  %s\n\n", render(mutedStyle, "No connector configured. Let's set up."))
}

func printReady(provider, mode string) {
	fmt.Fprintf(os.Stderr, "\n  %s %s → %s\n\n",
		render(successStyle, "✓"),
		provider,
		mode,
	)
}
