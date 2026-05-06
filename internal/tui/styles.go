package tui

// Styling. Patterned on Charm's bubbletea list-fancy example: a
// coloured-background title block, padded app frame, polished
// selection accent in the delegate, status-style footer.
//
// All of these are vars (not constants), so we can rebuild them
// after Run() reconfigures lipgloss's default renderer to talk to
// /dev/tty (see Run for why). Without that, the package-level
// `lipgloss.NewStyle()` calls would have captured a renderer that
// thinks the output isn't a terminal and refuses to emit colours.

import "github.com/charmbracelet/lipgloss"

var (
	appStyle      lipgloss.Style
	titleStyle    lipgloss.Style
	inputBoxStyle lipgloss.Style
	iconStyle     lipgloss.Style
	branchStyle   lipgloss.Style
	wtArrowStyle  lipgloss.Style
	dimStyle      lipgloss.Style
	footerStyle   lipgloss.Style
	keyStyle      lipgloss.Style
	errStyle      lipgloss.Style
	noticeStyle   lipgloss.Style

	// Row styles used by opDelegate.
	activeIndicatorStyle lipgloss.Style
	activeTitleStyle     lipgloss.Style
	activeTypeIconStyle  lipgloss.Style
	activeMetaStyle      lipgloss.Style
	mutedTitleStyle      lipgloss.Style
	mutedTypeIconStyle   lipgloss.Style
	mutedStyle           lipgloss.Style

	// Mode styles for the textinput.
	insertModePromptStyle lipgloss.Style
	insertModeCursorStyle lipgloss.Style
	normalModePromptStyle lipgloss.Style
	normalModeCursorStyle lipgloss.Style
)

func init() { initStyles() } // safe defaults for tests + first import

func initStyles() {
	// Padding around the whole TUI — same as list-fancy.
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	// Title block: white text on cyan background, padded so it reads
	// as a tag. Mirrors list-fancy's "Groceries" style but in the
	// Claude /resume cyan family.
	titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#0094B0")).
		Padding(0, 1).
		Bold(true)

	inputBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5C5C5C"))

	iconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	branchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7BC97B"))
	wtArrowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB46B"))
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))

	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")).MarginTop(1)
	keyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#DDDDDD")).Bold(true)

	errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6E6E"))
	noticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DD7E5"))

	activeIndicatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5DC9F5")).
		Bold(true)
	activeTitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true)
	activeTypeIconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DC9F5"))
	activeMetaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DC9F5"))
	mutedTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9A9A9A"))
	mutedTypeIconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	// bubbles' fake cursor always renders as a Reverse-styled char
	// (always block-shaped). The only knob is colour: cyan in insert,
	// amber in normal — reinforced by prompt colour and the footer
	// NORMAL indicator.
	insertModePromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DC9F5")).Bold(true)
	insertModeCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DC9F5"))
	normalModePromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB46B")).Bold(true)
	normalModeCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB46B"))
}
