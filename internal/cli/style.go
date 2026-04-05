package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func printHeader(version string) {
	fmt.Fprintf(os.Stderr, "\n  %s\n\n", titleStyle.Render("lumber "+version))
	fmt.Fprintf(os.Stderr, "  %s\n\n", mutedStyle.Render("No connector configured. Let's set up."))
}

func printReady(provider, mode string) {
	fmt.Fprintf(os.Stderr, "\n  %s %s → %s\n\n",
		successStyle.Render("✓"),
		provider,
		mode,
	)
}
