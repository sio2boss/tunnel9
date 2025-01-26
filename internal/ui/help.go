package ui

import "github.com/charmbracelet/lipgloss"

var helpStyle = lipgloss.NewStyle().
	Align(lipgloss.Left).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#2dd4bf")).
	Padding(1, 2)

func (a *App) helpView() string {
	help := `Keyboard Controls:

Navigation
  ↑/↓: Select tunnel
  enter: Toggle selected tunnel
  h: Toggle help
  l: Toggle error log
  q/esc: Quit

Console
  pgup/pgdn: Scroll console
  home/end: Jump to top/bottom
  l: Toggle console view
  f: Toggle filtering by selected tunnel

Sorting
  </>: Change sort column
  r: Reverse sort order

Tunnel Status
  [✓] Active tunnel
  [~] Connecting tunnel
  [x] Stopped tunnel
  [!] Error state

Filtering
  t: Filter by tag

Management
  n: Create new tunnel from SSH string
  e: Edit selected tunnel
  backspace: Delete selected tunnel

Press h or esc to close help`

	helpBox := helpStyle.Width(60).Render(help)
	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		helpBox)
}
