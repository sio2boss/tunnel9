package ui

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"tunnel9/internal/config"
	"tunnel9/internal/ssh"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// Add a tick message type for periodic updates
type tickMsg time.Time

// Add a log message type for the tea.Msg interface
type logMsg string

// Add a status message type for the tea.Msg interface
type statusMsg ssh.TunnelStatus

type TunnelRecord struct {
	ID      string
	Status  string // "stopped", "active", "error"
	Config  config.TunnelConfig
	Metrics string
}

type App struct {
	table             table.Model
	tunnels           []TunnelRecord
	currentTag        string
	manager           *ssh.TunnelManager
	height            int
	width             int
	showHelp          bool
	showConsole       bool
	sortColumn        int
	sortReverse       bool
	baseColumns       []string // Store original column titles
	errorLog          []string
	viewport          viewport.Model
	filterLogs        bool // Whether to filter logs by selected tunnel
	showDialog        bool
	dialogFields      []dialogField
	activeField       int
	dialogMode        dialogMode
	editingIndex      int
	loader            *config.ConfigLoader
	showTagDialog     bool
	tagOptions        []string
	selectedTags      map[string]bool
	showDeleteConfirm bool
	deleteIndex       int
	privacyMode       bool
	logCursor         int  // Track position in logs for scrolling
	autoScroll        bool // Whether to auto-scroll to bottom
	isWideMode        bool // Whether to show wide or compact view
	dialogError       string // Inline error shown in the dialog
}

func convertConfigsToRecords(configs []config.TunnelConfig) []TunnelRecord {
	tunnels := make([]TunnelRecord, len(configs))
	for i, tc := range configs {

		// Make sure the name is set
		if tc.Name == "" {
			tc.Name = tc.RemoteHost
		}

		tunnels[i] = TunnelRecord{
			ID:      uuid.New().String(),
			Status:  "stopped",
			Config:  tc,
			Metrics: "--",
		}
	}
	return tunnels
}

var (
	maxConsoleHeight = 16

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#2dd4bf")).
			Align(lipgloss.Center).
			MarginBottom(1)

	consoleStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#2dd4bf")).
			BorderRight(true).
			Padding(0, 1)

	controlsStyle = lipgloss.NewStyle()
)

func NewApp(loader *config.ConfigLoader, configs []config.TunnelConfig, initialTag string) *App {

	tunnels := convertConfigsToRecords(configs)

	// Store base column titles for both wide and compact modes
	baseColumns := []string{
		"STATUS",
		"NAME",
		"LOCAL",
		"BIND",
		"HOST",
		"REMOTE",
		"BASTION",
		"TAG",
		"MESSAGE",
	}

	// Create columns with initial widths for compact mode
	columns := []table.Column{
		{Title: baseColumns[0], Width: 8},  // STATUS
		{Title: baseColumns[1], Width: 30}, // NAME
		{Title: "TUNNEL", Width: 30},       // Combined LOCAL:HOST:REMOTE
		{Title: baseColumns[7], Width: 12}, // TAG
		{Title: baseColumns[8], Width: 40}, // MESSAGE
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
	)

	// Use default table styles but customize them
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Bold(true).
		Align(lipgloss.Left).
		AlignHorizontal(lipgloss.Left).
		MarginLeft(0).
		PaddingLeft(0).
		Width(0)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("212")).
		Bold(true)
	s.Cell = s.Cell.
		Align(lipgloss.Left).
		AlignHorizontal(lipgloss.Left).
		PaddingLeft(0).
		PaddingRight(1)
	t.SetStyles(s)

	// Initialize viewport with a default size and scrollbar
	vp := viewport.New(0, maxConsoleHeight)
	vp.Style = consoleStyle
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 1 // Make mouse scrolling smoother
	vp.SetContent("")
	vp.YPosition = 0

	app := &App{
		table:        t,
		tunnels:      tunnels,
		currentTag:   initialTag,
		manager:      ssh.NewTunnelManager(),
		baseColumns:  baseColumns,
		viewport:     vp,
		filterLogs:   false,
		showDialog:   false,
		dialogFields: make([]dialogField, 13),
		activeField:  0,
		loader:       loader,
		selectedTags: make(map[string]bool),
		autoScroll:   true,
		isWideMode:   false,
	}

	// Set initial rows
	app.updateTableRows()

	return app
}

func (a *App) updateTableRows() {
	// Update column headers to show sort indicators
	columns := a.table.Columns()
	for i := range columns {
		var title string
		if a.isWideMode {
			title = a.baseColumns[i]
		} else {
			switch i {
			case 0:
				title = a.baseColumns[0] // STATUS
			case 1:
				title = a.baseColumns[1] // NAME
			case 2:
				title = "TUNNEL"
			case 3:
				title = a.baseColumns[7] // TAG
			case 4:
				title = a.baseColumns[8] // MESSAGE
			}
		}

		// Add sort indicator if this is the sorted column
		if i == a.sortColumn {
			if a.sortReverse {
				title += " ▼"
			} else {
				title += " ▲"
			}
		} else {
			title += "  " // Add padding to maintain alignment
		}
		columns[i].Title = title
	}
	a.table.SetColumns(columns)

	// Filter tunnels based on selected tags
	filteredTunnels := a.tunnels
	if a.currentTag != "" {
		selectedTags := strings.Split(a.currentTag, ",")
		filteredTunnels = make([]TunnelRecord, 0)
		for _, t := range a.tunnels {
			for _, tag := range selectedTags {
				if t.Config.Tag == tag {
					filteredTunnels = append(filteredTunnels, t)
					break
				}
			}
		}
	}

	rows := make([]table.Row, len(filteredTunnels))
	for i, t := range filteredTunnels {
		// Format status without lipgloss styling
		status := "[x]"
		switch t.Status {
		case "active":
			status = "[✓]"
		case "error":
			status = "[!]"
		case "connecting":
			status = "[~]"
		}

		// Format message without lipgloss styling
		message := t.Metrics

		// Mask sensitive information in privacy mode
		remoteHost := t.Config.RemoteHost
		bastionHost := t.Config.Bastion.Host
		bindAddr := t.Config.BindAddress
		if bindAddr == "" {
			bindAddr = "localhost"
		}
		if a.privacyMode {
			if remoteHost != "" {
				remoteHost = "********"
			}
			if bastionHost != "" {
				bastionHost = "********"
			}
			if bindAddr != "0.0.0.0" {
				bindAddr = "********"
			}
		}

		if a.isWideMode {
			rows[i] = table.Row{
				status,
				t.Config.Name,
				fmt.Sprintf("%*d", 7, t.Config.LocalPort),
				bindAddr,
				remoteHost,
				fmt.Sprintf("%*d", 8, t.Config.RemotePort),
				bastionHost,
				t.Config.Tag,
				message,
			}
		} else {
			// Compact mode: combine local:host:remote into one field
			shortRemoteHost := remoteHost
			if remoteHost == "localhost" && t.Config.Bastion.Host != "" {
				shortRemoteHost = bastionHost
			}
			// Only shorten domain names (strip after first dot); leave IP addresses intact
			if net.ParseIP(shortRemoteHost) == nil {
				if idx := strings.Index(shortRemoteHost, "."); idx > 0 {
					shortRemoteHost = shortRemoteHost[:idx]
				}
			}

			tunnel := fmt.Sprintf("%d:%s:%d", t.Config.LocalPort, shortRemoteHost, t.Config.RemotePort)

			rows[i] = table.Row{
				status,
				t.Config.Name,
				tunnel,
				t.Config.Tag,
				message,
			}
		}
	}
	a.table.SetRows(rows)
}

func (a *App) Init() tea.Cmd {
	// Return multiple commands using tea.Batch
	return tea.Batch(
		// Original tick command
		tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
		// New command to read from log channel
		func() tea.Msg {
			msg := <-a.manager.LogChan
			return logMsg(msg)
		},
		// New command to read from status channel
		func() tea.Msg {
			status := <-a.manager.StatusChan
			return statusMsg(status)
		},
	)
}

func (a *App) logError(format string, args ...interface{}) {
	msg := fmt.Sprintf("%s ERROR %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	a.errorLog = append(a.errorLog, msg)
	// Keep only last 10 messages
	if len(a.errorLog) > 10 {
		a.errorLog = a.errorLog[len(a.errorLog)-10:]
	}
}

func (a *App) getAllFilteredLogs() []string {
	if !a.filterLogs {
		return a.errorLog
	}

	cursor := a.table.Cursor()

	// Get the filtered tunnels if there's a tag filter
	filteredTunnels := a.tunnels
	if a.currentTag != "" {
		selectedTags := strings.Split(a.currentTag, ",")
		filteredTunnels = make([]TunnelRecord, 0)
		for _, t := range a.tunnels {
			for _, tag := range selectedTags {
				if t.Config.Tag == tag {
					filteredTunnels = append(filteredTunnels, t)
					break
				}
			}
		}
	}

	if cursor >= len(filteredTunnels) {
		return a.errorLog
	}

	// Find the actual tunnel from the filtered list
	selectedTunnel := filteredTunnels[cursor]
	var selected *TunnelRecord
	for i := range a.tunnels {
		if a.tunnels[i].ID == selectedTunnel.ID {
			selected = &a.tunnels[i]
			break
		}
	}

	if selected == nil {
		return a.errorLog
	}

	prefix := fmt.Sprintf("[%s]", selected.Config.Name)

	filtered := make([]string, 0)
	for _, log := range a.errorLog {
		// Skip timestamp (first 8 chars) when looking for the tunnel name prefix
		if len(log) > 9 && strings.Contains(log[9:], prefix) {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func (a *App) getVisibleLogs(logs []string) []string {
	if len(logs) == 0 {
		return logs
	}

	// Calculate visible range based on viewport height
	visibleLines := a.viewport.Height

	// If we have fewer logs than visible lines, show all logs
	if len(logs) <= visibleLines {
		a.logCursor = len(logs) - 1
		return logs
	}

	// Ensure cursor is at least visibleLines from start
	if a.logCursor < visibleLines-1 {
		a.logCursor = visibleLines - 1
	}

	// Ensure cursor doesn't go past end
	if a.logCursor >= len(logs) {
		a.logCursor = len(logs) - 1
	}

	// Calculate window with cursor at bottom
	start := a.logCursor - (visibleLines - 1)
	end := a.logCursor + 1

	// Handle auto-scroll
	if a.autoScroll {
		a.logCursor = len(logs) - 1
		start = len(logs) - visibleLines
		end = len(logs)
	}

	return logs[start:end]
}

func (a *App) getFilteredLogs() []string {
	return a.getVisibleLogs(a.getAllFilteredLogs())
}

func (a *App) colorizeLogLine(line string) string {
	// Color styles for log levels
	debugStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))    // cyan
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))     // red
	tunnelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13"))   // pink/magenta
	timestampStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // grey

	// Parse the line and rebuild with colors
	// Format: timestamp DEBUG [tunnel-name] message
	timestampRegex := regexp.MustCompile(`^(\d{2}:\d{2}:\d{2})\s+(DEBUG|ERROR)\s+(\[[^\]]+\])\s+(.*)$`)
	matches := timestampRegex.FindStringSubmatch(line)

	if len(matches) == 5 {
		// Format: timestamp LEVEL [tunnel] message
		timestamp := timestampStyle.Render(matches[1])
		var level string
		if matches[2] == "DEBUG" {
			level = debugStyle.Render(matches[2])
		} else {
			level = errorStyle.Render(matches[2])
		}
		tunnel := tunnelStyle.Render(matches[3])
		message := matches[4]

		return fmt.Sprintf("%s %s %s %s", timestamp, level, tunnel, message)
	}

	// Fallback for lines without tunnel name or different format
	// Try: timestamp ERROR message
	simpleErrorRegex := regexp.MustCompile(`^(\d{2}:\d{2}:\d{2})\s+(ERROR)\s+(.*)$`)
	simpleMatches := simpleErrorRegex.FindStringSubmatch(line)
	if len(simpleMatches) == 4 {
		timestamp := timestampStyle.Render(simpleMatches[1])
		level := errorStyle.Render(simpleMatches[2])
		message := simpleMatches[3]
		return fmt.Sprintf("%s %s %s", timestamp, level, message)
	}

	// If no match, just colorize what we can find
	// Colorize timestamp if present
	timestampOnlyRegex := regexp.MustCompile(`^(\d{2}:\d{2}:\d{2})`)
	line = timestampOnlyRegex.ReplaceAllStringFunc(line, func(match string) string {
		return timestampStyle.Render(match)
	})

	// Colorize tunnel name if present
	tunnelRegex := regexp.MustCompile(`(\[[^\]]+\])`)
	line = tunnelRegex.ReplaceAllStringFunc(line, func(match string) string {
		return tunnelStyle.Render(match)
	})

	// Colorize DEBUG/ERROR
	if strings.Contains(line, " DEBUG ") {
		line = strings.ReplaceAll(line, " DEBUG ", " "+debugStyle.Render("DEBUG")+" ")
	}
	if strings.Contains(line, " ERROR ") {
		line = strings.ReplaceAll(line, " ERROR ", " "+errorStyle.Render("ERROR")+" ")
	}

	return line
}

func (a *App) updateViewport() {
	if !a.showConsole {
		return
	}

	logs := a.getFilteredLogs()
	coloredLogs := make([]string, len(logs))
	for i, log := range logs {
		coloredLogs[i] = a.colorizeLogLine(log)
	}
	content := strings.Join(coloredLogs, "\n")
	a.viewport.SetContent(content)
	a.viewport.GotoBottom()
}

func (a *App) initTagDialog() {
	// Collect unique tags
	tagMap := make(map[string]bool)
	for _, tunnel := range a.tunnels {
		if tunnel.Config.Tag != "" {
			tagMap[tunnel.Config.Tag] = false
		}
	}

	// Convert to sorted slice
	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	a.tagOptions = tags
	a.selectedTags = tagMap
	a.showTagDialog = true
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Handle delete confirmation dialog
	if a.showDeleteConfirm {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				if a.deleteIndex >= 0 && a.deleteIndex < len(a.tunnels) {
					selected := &a.tunnels[a.deleteIndex]
					// Don't allow deletion of active tunnels
					if selected.Status == "active" || selected.Status == "connecting" {
						a.logError("Cannot delete active tunnel. Stop it first.")
						a.showDeleteConfirm = false
						return a, nil
					}
					// Remove the tunnel
					a.tunnels = append(a.tunnels[:a.deleteIndex], a.tunnels[a.deleteIndex+1:]...)
					a.Logf("Deleted tunnel: %s", selected.Config.Name)
					a.saveConfig()
					a.updateTableRows()
				}
				a.showDeleteConfirm = false
				return a, nil
			case tea.KeyEsc, tea.KeyCtrlC:
				a.showDeleteConfirm = false
				return a, nil
			}
			return a, nil
		}
	}

	// Handle dialog input if it's shown
	if a.showDialog {
		if keyMsg, ok := msg.(tea.KeyMsg); ok && a.handleDialogKey(keyMsg) {
			return a, nil
		}
	}

	// Handle tag dialog input
	if a.showTagDialog {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEsc, tea.KeyCtrlC:
				a.showTagDialog = false
				return a, nil

			case tea.KeyEnter:
				// Apply tag filter
				selectedTags := make([]string, 0)
				for tag, selected := range a.selectedTags {
					if selected {
						selectedTags = append(selectedTags, tag)
					}
				}
				if len(selectedTags) > 0 {
					a.currentTag = strings.Join(selectedTags, ",")
				} else {
					a.currentTag = ""
				}
				a.showTagDialog = false
				return a, nil

			case tea.KeyUp:
				if len(a.tagOptions) > 0 {
					a.activeField = (a.activeField - 1 + len(a.tagOptions)) % len(a.tagOptions)
				}
				return a, nil

			case tea.KeyDown:
				if len(a.tagOptions) > 0 {
					a.activeField = (a.activeField + 1) % len(a.tagOptions)
				}
				return a, nil

			case tea.KeySpace:
				if len(a.tagOptions) > 0 {
					tag := a.tagOptions[a.activeField]
					a.selectedTags[tag] = !a.selectedTags[tag]
				}
				return a, nil
			}
		}
	}

	switch msg := msg.(type) {
	case statusMsg:
		// Find the tunnel and update its status
		for i, t := range a.tunnels {
			if t.ID == string(msg.ID) {
				a.tunnels[i].Status = string(msg.State)
				a.tunnels[i].Metrics = msg.Message
				a.updateTableRows()
				break
			}
		}
		// Continue reading from the channel
		return a, func() tea.Msg {
			status := <-a.manager.StatusChan
			return statusMsg(status)
		}

	case logMsg:
		// Add the new log message to our log
		a.errorLog = append(a.errorLog, string(msg))
		// Keep only last 100 messages for scrolling
		if len(a.errorLog) > 100 {
			a.errorLog = a.errorLog[len(a.errorLog)-100:]
		}
		// Update viewport content
		a.updateViewport()
		// Continue reading from the channel
		return a, func() tea.Msg {
			msg := <-a.manager.LogChan
			return logMsg(msg)
		}

	case tickMsg:
		// Update metrics for active tunnels
		for i, t := range a.tunnels {
			if t.Status == "active" {
				a.tunnels[i].Metrics = a.manager.GetMetrics(t.ID)
			}
		}
		a.updateTableRows()

		// Schedule next update
		return a, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})

	case tea.WindowSizeMsg:
		// Save the window size
		a.height = msg.Height
		a.width = msg.Width

		// Update table height to use available space
		headerHeight := 2 // Title + newline
		footerHeight := 2 // Status bar + controls
		consoleHeight := 0
		if a.showConsole {
			consoleHeight = maxConsoleHeight + 1 // Console box height + spacing
		}
		availableHeight := a.height - headerHeight - footerHeight - consoleHeight

		// Ensure we show at least all tunnels if we have space
		minHeight := len(a.tunnels)
		if availableHeight < minHeight {
			availableHeight = minHeight
		}

		a.table.SetHeight(availableHeight)

		// Update viewport and console style width to match screen width
		a.viewport.Width = a.width - 2
		consoleStyle = consoleStyle.Width(a.width - 2)
		a.viewport.Style = consoleStyle
		return a, nil

	case tea.KeyMsg:
		if a.showHelp {
			if msg.String() == "esc" || msg.String() == "h" || msg.String() == "ctrl+c" {
				a.showHelp = false
				return a, nil
			}
			return a, nil
		}

		// Handle viewport scrolling when console is shown
		if a.showConsole {
			switch msg.String() {
			case "pgup", "[":
				if a.logCursor > a.viewport.Height-1 {
					a.logCursor--
					a.autoScroll = false
					a.updateViewport()
				}
				return a, nil
			case "pgdown", "]":
				allLogs := a.getAllFilteredLogs()
				if a.logCursor < len(allLogs)-1 {
					a.logCursor++
					a.autoScroll = false
					a.updateViewport()
				}
				return a, nil
			case "home":
				a.logCursor = a.viewport.Height - 1
				a.autoScroll = false
				a.updateViewport()
				return a, nil
			case "end":
				allLogs := a.getAllFilteredLogs()
				a.logCursor = len(allLogs) - 1
				a.autoScroll = false
				a.updateViewport()
				return a, nil
			case "a":
				a.autoScroll = !a.autoScroll
				if a.autoScroll {
					allLogs := a.getAllFilteredLogs()
					a.logCursor = len(allLogs) - 1
					a.updateViewport()
				}
				return a, nil
			case "f":
				a.filterLogs = !a.filterLogs
				allLogs := a.getAllFilteredLogs()
				a.logCursor = len(allLogs) - 1
				a.updateViewport()
				return a, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			// Cleanup all resources before quitting
			a.manager.Cleanup()
			return a, tea.Quit

		case "h":
			a.showHelp = true
			return a, nil

		case "l":
			a.showConsole = !a.showConsole
			if a.showConsole {
				// Update viewport content when showing console
				a.updateViewport()
			}
			// Trigger a window resize to adjust table height
			return a.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})

		case "<", ",":
			// Move to previous column
			a.sortColumn--
			if a.sortColumn < 0 {
				a.sortColumn = len(a.table.Columns()) - 1
			}
			a.sortTunnels()
			a.updateTableRows()

		case ">", ".":
			// Move to next column
			a.sortColumn++
			if a.sortColumn >= len(a.table.Columns()) {
				a.sortColumn = 0
			}
			a.sortTunnels()
			a.updateTableRows()

		case "r":
			// Reverse sort order
			a.sortReverse = !a.sortReverse
			a.sortTunnels()
			a.updateTableRows()

		case "enter":
			if len(a.tunnels) == 0 {
				return a, nil
			}

			cursor := a.table.Cursor()

			// Get the filtered tunnels if there's a tag filter
			filteredTunnels := a.tunnels
			if a.currentTag != "" {
				selectedTags := strings.Split(a.currentTag, ",")
				filteredTunnels = make([]TunnelRecord, 0)
				for _, t := range a.tunnels {
					for _, tag := range selectedTags {
						if t.Config.Tag == tag {
							filteredTunnels = append(filteredTunnels, t)
							break
						}
					}
				}
			}

			if cursor >= len(filteredTunnels) {
				return a, nil
			}

			// Find the actual tunnel from the filtered list
			selectedTunnel := filteredTunnels[cursor]
			var selected *TunnelRecord
			for i := range a.tunnels {
				if a.tunnels[i].ID == selectedTunnel.ID {
					selected = &a.tunnels[i]
					break
				}
			}

			if selected == nil {
				return a, nil
			}

			switch selected.Status {
			case "stopped", "error":
				// Try to start the tunnel
				tunnel := a.manager.CreateTunnel(
					selected.ID,
					selected.Config,
				)
				if tunnel == nil {
					selected.Status = "error"
					selected.Metrics = "failed to start"
					a.logError("Failed to start tunnel to %s", selected.Config.RemoteHost)
				} else {
					selected.Status = "connecting"
					selected.Metrics = "initializing"
					a.manager.StartTunnel(tunnel)
				}
			case "active", "connecting":
				// Try to stop the tunnel
				err := a.manager.StopTunnel(selected.ID)
				if err != nil {
					selected.Status = "error"
					selected.Metrics = fmt.Sprintf("stop: %v", err)
					a.logError("Failed to stop tunnel %s: %v", selected.Config.RemoteHost, err)
				} else {
					selected.Status = "stopped"
					selected.Metrics = "stopped"
				}
			}

			a.updateTableRows()

		case "delete", "backspace":
			if !a.showDialog && !a.showTagDialog && !a.showDeleteConfirm {
				cursor := a.table.Cursor()
				// Get the filtered tunnels if there's a tag filter
				filteredTunnels := a.tunnels
				if a.currentTag != "" {
					selectedTags := strings.Split(a.currentTag, ",")
					filteredTunnels = make([]TunnelRecord, 0)
					for _, t := range a.tunnels {
						for _, tag := range selectedTags {
							if t.Config.Tag == tag {
								filteredTunnels = append(filteredTunnels, t)
								break
							}
						}
					}
				}

				if cursor >= len(filteredTunnels) {
					return a, nil
				}

				// Find the actual tunnel index from the filtered tunnel
				selectedTunnel := filteredTunnels[cursor]
				if selectedTunnel.Status == "active" {
					a.logError("Cannot delete active tunnel. Stop it first.")
					return a, nil
				}

				actualIndex := -1
				for i, t := range a.tunnels {
					if t.ID == selectedTunnel.ID {
						actualIndex = i
						break
					}
				}

				if actualIndex != -1 {
					a.deleteIndex = actualIndex
					a.showDeleteConfirm = true
				}
				return a, nil
			}
		}

		// After handling up/down keys in table, update viewport
		if msg.String() == "up" || msg.String() == "down" {
			a.table, cmd = a.table.Update(msg)
			if a.filterLogs {
				a.updateViewport()
			}
			return a, cmd
		}

		switch msg.String() {
		case "n":
			if !a.showDialog {
				a.showDialog = true
				a.initDialog(modeNew)
				return a, nil
			}
		case "G":
			// New tunnel in Gcloud mode (paste gcloud IAP command)
			if !a.showDialog {
				a.showDialog = true
				a.initDialog(modeNew)
				a.dialogFields[0].value = "gcloud"
				a.dialogFields[1].isHidden = false
				for i := 2; i <= 8; i++ {
					a.dialogFields[i].isHidden = true
				}
				a.activeField = 1
				return a, nil
			}
		case "e":
			if !a.showDialog && len(a.tunnels) > 0 {
				cursor := a.table.Cursor()
				if cursor >= len(a.tunnels) {
					return a, nil
				}
				selected := &a.tunnels[cursor]
				// Don't allow editing of active or connecting tunnels
				if selected.Status == "active" || selected.Status == "connecting" {
					a.logError("Cannot edit tunnel while it is %s. Stop it first.", selected.Status)
					return a, nil
				}
				a.showDialog = true
				a.initDialog(modeEdit)
				return a, nil
			}
		case "t":
			if !a.showDialog && !a.showTagDialog {
				a.initTagDialog()
				return a, nil
			}
		case "p":
			a.privacyMode = !a.privacyMode
			a.updateTableRows()
			return a, nil
		case "o":
			// Open browser to selected tunnel's local port
			if !a.showDialog && !a.showTagDialog && !a.showDeleteConfirm && len(a.tunnels) > 0 {
				cursor := a.table.Cursor()

				// Get the filtered tunnels if there's a tag filter
				filteredTunnels := a.tunnels
				if a.currentTag != "" {
					selectedTags := strings.Split(a.currentTag, ",")
					filteredTunnels = make([]TunnelRecord, 0)
					for _, t := range a.tunnels {
						for _, tag := range selectedTags {
							if t.Config.Tag == tag {
								filteredTunnels = append(filteredTunnels, t)
								break
							}
						}
					}
				}

				if cursor >= len(filteredTunnels) {
					return a, nil
				}

				// Find the actual tunnel from the filtered list
				selectedTunnel := filteredTunnels[cursor]
				var selected *TunnelRecord
				for i := range a.tunnels {
					if a.tunnels[i].ID == selectedTunnel.ID {
						selected = &a.tunnels[i]
						break
					}
				}

				if selected != nil {
					url := fmt.Sprintf("http://localhost:%d", selected.Config.LocalPort)
					var cmd *exec.Cmd
					switch runtime.GOOS {
					case "windows":
						cmd = exec.Command("cmd", "/c", "start", url)
					case "darwin":
						cmd = exec.Command("open", url)
					default:
						cmd = exec.Command("xdg-open", url)
					}
					if err := cmd.Start(); err != nil {
						a.logError("Failed to open browser: %v", err)
					} else {
						a.Logf("Opened %s in browser", url)
					}
				}
				return a, nil
			}
		case "w":
			a.isWideMode = !a.isWideMode
			// Update columns based on mode
			if a.isWideMode {
				// First set empty rows to avoid index out of range errors
				a.table.SetRows([]table.Row{})
				columns := []table.Column{
					{Title: a.baseColumns[0], Width: 8},
					{Title: a.baseColumns[1], Width: 25},
					{Title: a.baseColumns[2], Width: 7},
					{Title: a.baseColumns[3], Width: 15},
					{Title: a.baseColumns[4], Width: 30},
					{Title: a.baseColumns[5], Width: 8},
					{Title: a.baseColumns[6], Width: 30},
					{Title: a.baseColumns[7], Width: 12},
					{Title: a.baseColumns[8], Width: 40},
				}
				a.table.SetColumns(columns)
			} else {
				// First set empty rows to avoid index out of range errors
				a.table.SetRows([]table.Row{})
				columns := []table.Column{
					{Title: a.baseColumns[0], Width: 8},  // STATUS
					{Title: a.baseColumns[1], Width: 35}, // NAME
					{Title: "TUNNEL", Width: 40},         // Combined LOCAL:HOST:REMOTE
					{Title: a.baseColumns[7], Width: 12}, // TAG
					{Title: a.baseColumns[8], Width: 40}, // MESSAGE
				}
				a.table.SetColumns(columns)
			}
			// Reset sort column if it's out of range for the new mode
			if !a.isWideMode && a.sortColumn >= 5 {
				a.sortColumn = 0
			}
			a.updateTableRows()
			return a, nil
		case "A":
			// Start all stopped tunnels
			if !a.showDialog && !a.showTagDialog && !a.showDeleteConfirm {
				startedCount := 0
				for i := range a.tunnels {
					tunnel := &a.tunnels[i]
					if tunnel.Status == "stopped" || tunnel.Status == "error" {
						t := a.manager.CreateTunnel(tunnel.ID, tunnel.Config)
						if t == nil {
							tunnel.Status = "error"
							tunnel.Metrics = "failed to start"
							a.logError("Failed to start tunnel to %s", tunnel.Config.RemoteHost)
						} else {
							tunnel.Status = "connecting"
							tunnel.Metrics = "initializing"
							a.manager.StartTunnel(t)
							startedCount++
						}
					}
				}
				if startedCount > 0 {
					a.Logf("Started %d tunnel(s)", startedCount)
				}
				a.updateTableRows()
				return a, nil
			}
		case "C":
			// Stop all active tunnels in background
			if !a.showDialog && !a.showTagDialog && !a.showDeleteConfirm {
				toStop := make([]*TunnelRecord, 0)
				for i := range a.tunnels {
					tunnel := &a.tunnels[i]
					if tunnel.Status == "active" || tunnel.Status == "connecting" {
						toStop = append(toStop, tunnel)
						// Update status immediately for responsive UI
						tunnel.Status = "stopping"
						tunnel.Metrics = "stopping..."
					}
				}
				if len(toStop) > 0 {
					a.Logf("Stopping %d tunnel(s)...", len(toStop))
					// Stop tunnels in background goroutines
					for _, tunnel := range toStop {
						go func(t *TunnelRecord) {
							err := a.manager.StopTunnel(t.ID)
							if err != nil {
								t.Status = "error"
								t.Metrics = fmt.Sprintf("stop: %v", err)
							} else {
								t.Status = "stopped"
								t.Metrics = "stopped"
							}
							// Note: updateTableRows() is called by the tick handler,
							// so the UI will update automatically on the next tick
						}(tunnel)
					}
				}
				a.updateTableRows()
				return a, nil
			}
		}
	}

	a.table, cmd = a.table.Update(msg)
	return a, cmd
}

func (a *App) sortTunnels() {
	col := a.sortColumn
	rev := a.sortReverse

	sort.SliceStable(a.tunnels, func(i, j int) bool {
		var less bool
		if a.isWideMode {
			switch col {
			case 0: // Status
				less = a.tunnels[i].Status < a.tunnels[j].Status
			case 1: // Name
				less = a.tunnels[i].Config.Name < a.tunnels[j].Config.Name
			case 2: // Local Port
				less = a.tunnels[i].Config.LocalPort < a.tunnels[j].Config.LocalPort
			case 3: // Bind
				less = a.tunnels[i].Config.BindAddress < a.tunnels[j].Config.BindAddress
			case 4: // Host
				less = a.tunnels[i].Config.RemoteHost < a.tunnels[j].Config.RemoteHost
			case 5: // Remote Port
				less = a.tunnels[i].Config.RemotePort < a.tunnels[j].Config.RemotePort
			case 6: // Bastion
				less = a.tunnels[i].Config.Bastion.Host < a.tunnels[j].Config.Bastion.Host
			case 7: // Tag
				less = a.tunnels[i].Config.Tag < a.tunnels[j].Config.Tag
			case 8: // Message
				less = a.tunnels[i].Metrics < a.tunnels[j].Metrics
			}
		} else {
			switch col {
			case 0: // Status
				less = a.tunnels[i].Status < a.tunnels[j].Status
			case 1: // Name
				less = a.tunnels[i].Config.Name < a.tunnels[j].Config.Name
			case 2: // Combined tunnel
				iTunnel := fmt.Sprintf("%d:%s:%d", a.tunnels[i].Config.LocalPort, a.tunnels[i].Config.RemoteHost, a.tunnels[i].Config.RemotePort)
				jTunnel := fmt.Sprintf("%d:%s:%d", a.tunnels[j].Config.LocalPort, a.tunnels[j].Config.RemoteHost, a.tunnels[j].Config.RemotePort)
				less = iTunnel < jTunnel
			case 3: // Tag
				less = a.tunnels[i].Config.Tag < a.tunnels[j].Config.Tag
			case 4: // Message
				less = a.tunnels[i].Metrics < a.tunnels[j].Metrics
			}
		}
		if rev {
			return !less
		}
		return less
	})
}

func (a *App) View() string {
	if a.showHelp {
		return a.helpView()
	}

	if a.showTagDialog {
		content := dialogActiveStyle.Render("Filter by Tags") + "\n\n"
		content += "Select tags with space, confirm with enter:\n\n"

		for i, tag := range a.tagOptions {
			if i == a.activeField {
				content += dialogActiveStyle.Render("> ")
			} else {
				content += "  "
			}

			if a.selectedTags[tag] {
				content += "[x] " + tag + "\n"
			} else {
				content += "[ ] " + tag + "\n"
			}
		}

		if len(a.tagOptions) == 0 {
			content += "No tags available\n"
		}

		content += "\n↑/↓: Move • Space: Toggle • Enter: Apply • Esc/Ctrl+C: Cancel"

		dialog := dialogStyle.Width(60).Render(content)
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			dialog)
	}

	if a.showDeleteConfirm {
		if a.deleteIndex >= 0 && a.deleteIndex < len(a.tunnels) {
			tunnel := a.tunnels[a.deleteIndex]
			content := dialogActiveStyle.Render("Confirm Delete") + "\n\n"
			content += fmt.Sprintf("Are you sure you want to delete tunnel '%s'?\n", tunnel.Config.Name)
			content += fmt.Sprintf("Local: %d, Remote: %s:%d\n",
				tunnel.Config.LocalPort,
				tunnel.Config.RemoteHost,
				tunnel.Config.RemotePort)
			if tunnel.Config.Bastion.Host != "" {
				content += fmt.Sprintf("Bastion: %s@%s\n",
					tunnel.Config.Bastion.User,
					tunnel.Config.Bastion.Host)
			}
			content += "\nEnter: Confirm • Esc/Ctrl+C: Cancel"

			dialog := dialogStyle.Width(60).Render(content)
			return lipgloss.Place(a.width, a.height,
				lipgloss.Center, lipgloss.Center,
				dialog)
		}
		a.showDeleteConfirm = false
	}

	if a.showDialog {
		return a.renderDialogView()
	}

	var s string

	// Add title with optional right-aligned tags
	titleText := "tunnel9 - SSH Tunnel Manager"
	if a.currentTag != "" {
		tagStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#2dd4bf")). // same as titleStyle foreground
			Foreground(lipgloss.Color("0")).       // black text
			Padding(0, 2)
		tagText := tagStyle.Render(a.currentTag)

		// Align tag's right edge with log panel's right edge (log panel width is a.width - 2)
		tagWidth := lipgloss.Width(tagText)
		rightEdgePos := a.width - 2
		spacing := rightEdgePos - len(titleText) - tagWidth
		if spacing > 0 {
			titleText = titleText + strings.Repeat(" ", spacing) + tagText
		} else {
			titleText = titleText + "  " + tagText
		}
		// Use left-align style to position title on left and tag on right
		titleLeftStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#2dd4bf")).
			Align(lipgloss.Left).
			MarginBottom(1)
		s += titleLeftStyle.Width(a.width).Render(titleText) + "\n"
	} else {
		s += titleStyle.Render(titleText) + "\n"
	}
	// Add a bit more space before the table
	s += "\n"

	// Table (no extra newlines)
	s += a.table.View()

	// Status bar (with proper spacing)
	s += "\n" // Single newline before status

	activeCount := 0
	errorCount := 0
	for _, t := range a.tunnels {
		switch t.Status {
		case "active":
			activeCount++
		case "error":
			errorCount++
		}
	}

	// Bottom status without sort info
	// Color just the first letter of each word with selected style color
	selectedColorStyle := controlsStyle.Foreground(lipgloss.Color("#2dd4bf"))
	upDownText := selectedColorStyle.Render("↑/↓") + ":select"
	enterText := selectedColorStyle.Render("enter") + ":toggle"
	sortText := selectedColorStyle.Render("</>") + ":sort"
	openText := selectedColorStyle.Render("o") + "pen"
	logText := selectedColorStyle.Render("l") + "og"
	filterText := ""
	if a.showConsole && a.filterLogs {
		filterText = selectedColorStyle.Render("u") + "nfilter"
	} else {
		filterText = selectedColorStyle.Render("f") + "ilter"
	}
	autoText := ""
	if a.showConsole {
		if a.autoScroll {
			autoText = selectedColorStyle.Render("a") + "uto"
		} else {
			autoText = selectedColorStyle.Render("m") + "anual"
		}
	}
	helpText := selectedColorStyle.Render("h") + "elp"
	tagsText := selectedColorStyle.Render("t") + "ags"
	wideText := selectedColorStyle.Render("w") + "ide"
	quitText := selectedColorStyle.Render("q") + "uit"
	scrollText := selectedColorStyle.Render("[/]") + ":scroll"

	controls := controlsStyle.Render(upDownText + " • " + enterText + " • " + sortText + " • " + openText)
	if strings.Count(strings.Join(a.errorLog, ""), "ERROR") > 0 {
		controls += controlsStyle.Foreground(lipgloss.Color("227")).Render(" • " + logText)
	} else {
		controls += controlsStyle.Render(" • " + logText)
	}
	if a.showConsole {
		controls += controlsStyle.Render(" • " + filterText)
		controls += controlsStyle.Render(" • " + scrollText)
		controls += controlsStyle.Render(" • " + autoText)
	}
	controls += controlsStyle.Render(" • " + helpText + " • " + tagsText + " • " + wideText + " • " + quitText)
	s += controls

	// Add console if enabled
	if a.showConsole {
		// Update viewport content
		a.updateViewport()
		s += "\n" + a.viewport.View()
	}

	return s
}

func (a *App) saveConfig() {
	configs := make([]config.TunnelConfig, len(a.tunnels))
	for i, t := range a.tunnels {
		configs[i] = t.Config
	}

	if err := a.loader.Save(configs); err != nil {
		a.logError("Failed to save config: %v", err)
	} else {
		a.Logf("Configuration saved successfully")
	}
}

func (a *App) Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	a.errorLog = append(a.errorLog, fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg))
	if len(a.errorLog) > 100 {
		a.errorLog = a.errorLog[len(a.errorLog)-100:]
	}
	a.updateViewport()
}
