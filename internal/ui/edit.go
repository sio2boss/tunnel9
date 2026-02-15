package ui

import (
	"fmt"
	"strconv"
	"strings"

	"tunnel9/internal/config"
	"tunnel9/internal/parse"

	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	tea "github.com/charmbracelet/bubbletea"
)

// dialogField holds a single field in the create/edit tunnel dialog.
type dialogField struct {
	label    string
	value    string
	cursor   int
	isHidden bool
}

// dialogMode is whether the dialog is for creating a new tunnel or editing one.
type dialogMode int

const (
	modeNew dialogMode = iota
	modeEdit
)

// Dialog styles (used by create/edit tunnel dialog and by tag/delete dialogs)
var (
	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#2dd4bf")).
			Padding(1, 2)

	dialogActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#2dd4bf"))

	dialogSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#2d3436")).
				Foreground(lipgloss.Color("#2dd4bf"))
)

func (a *App) initDialog(mode dialogMode) {
	a.dialogMode = mode
	a.dialogError = ""
	a.dialogFields = []dialogField{
		{label: "Input Mode", value: "fields", cursor: 0, isHidden: true},
		{label: "SSH Command", value: "", cursor: 0, isHidden: true},
		{label: "Bind Address (optional)", value: "", cursor: 0},
		{label: "Local Port", value: "", cursor: 0},
		{label: "Remote Host", value: "", cursor: 0},
		{label: "Remote Port", value: "", cursor: 0},
		{label: "Bastion Host (optional)", value: "", cursor: 0},
		{label: "Bastion Port (optional)", value: "", cursor: 0},
		{label: "Bastion User (optional)", value: "", cursor: 0},
		{label: "Name", value: "", cursor: 0},
		{label: "Tag", value: "", cursor: 0},
		{label: "GCP Zone", value: "", cursor: 0},
		{label: "GCP Project ID (required)", value: "", cursor: 0},
		{label: "GCP gcloud config", value: "", cursor: 0},
	}

	if mode == modeEdit {
		cursor := a.table.Cursor()

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
			return
		}

		selectedTunnel := filteredTunnels[cursor]
		actualIndex := -1
		for i, t := range a.tunnels {
			if t.ID == selectedTunnel.ID {
				actualIndex = i
				break
			}
		}

		if actualIndex == -1 {
			return
		}

		a.editingIndex = actualIndex
		selected := &a.tunnels[actualIndex]

		var sshCmd string
		if selected.Config.GcpIap != nil {
			if strings.TrimSpace(selected.Config.GcpIap.Mode) == "direct" {
				sshCmd = fmt.Sprintf("gcloud compute start-iap-tunnel %s %d --zone=%s",
					selected.Config.Bastion.Host, selected.Config.RemotePort, selected.Config.GcpIap.Zone)
				if selected.Config.GcpIap.Project != "" {
					sshCmd += fmt.Sprintf(" --project=%s", selected.Config.GcpIap.Project)
				}
				host := selected.Config.BindAddress
				if host == "" {
					host = "localhost"
				}
				sshCmd += fmt.Sprintf(" --local-host-port=%s:%d", host, selected.Config.LocalPort)
			} else {
				instance := selected.Config.Bastion.Host
				if selected.Config.Bastion.User != "" {
					instance = selected.Config.Bastion.User + "@" + instance
				}
				sshCmd = fmt.Sprintf("gcloud compute ssh %s --tunnel-through-iap --zone=%s",
					instance, selected.Config.GcpIap.Zone)
				if selected.Config.GcpIap.Project != "" {
					sshCmd += fmt.Sprintf(" --project=%s", selected.Config.GcpIap.Project)
				}
				portMap := fmt.Sprintf("%d:%s:%d", selected.Config.LocalPort, selected.Config.RemoteHost, selected.Config.RemotePort)
				if selected.Config.BindAddress != "" {
					portMap = fmt.Sprintf("%s:%d:%s:%d", selected.Config.BindAddress, selected.Config.LocalPort, selected.Config.RemoteHost, selected.Config.RemotePort)
				}
				sshCmd += fmt.Sprintf(" --ssh-flag=\"-N -L %s\"", portMap)
			}
		} else {
			if selected.Config.BindAddress != "" {
				sshCmd = fmt.Sprintf("ssh -N -L %s:%d:%s:%d",
					selected.Config.BindAddress,
					selected.Config.LocalPort,
					selected.Config.RemoteHost,
					selected.Config.RemotePort)
			} else {
				sshCmd = fmt.Sprintf("ssh -N -L %d:%s:%d",
					selected.Config.LocalPort,
					selected.Config.RemoteHost,
					selected.Config.RemotePort)
			}
			if selected.Config.Bastion.Host != "" {
				sshCmd += fmt.Sprintf(" %s@%s",
					selected.Config.Bastion.User,
					selected.Config.Bastion.Host)
				if selected.Config.Bastion.Port != 22 {
					sshCmd += fmt.Sprintf(":%d", selected.Config.Bastion.Port)
				}
			}
		}

		a.dialogFields[1].value = sshCmd
		a.dialogFields[1].cursor = len(sshCmd)
		a.dialogFields[2].value = selected.Config.BindAddress
		a.dialogFields[2].cursor = len(selected.Config.BindAddress)
		a.dialogFields[3].value = fmt.Sprintf("%d", selected.Config.LocalPort)
		a.dialogFields[3].cursor = len(a.dialogFields[3].value)
		a.dialogFields[4].value = selected.Config.RemoteHost
		a.dialogFields[4].cursor = len(selected.Config.RemoteHost)
		a.dialogFields[5].value = fmt.Sprintf("%d", selected.Config.RemotePort)
		a.dialogFields[5].cursor = len(a.dialogFields[5].value)
		a.dialogFields[6].value = selected.Config.Bastion.Host
		a.dialogFields[6].cursor = len(selected.Config.Bastion.Host)
		a.dialogFields[7].value = strconv.Itoa(selected.Config.Bastion.Port)
		a.dialogFields[7].cursor = len(a.dialogFields[7].value)
		a.dialogFields[8].value = selected.Config.Bastion.User
		a.dialogFields[8].cursor = len(selected.Config.Bastion.User)
		a.dialogFields[9].value = selected.Config.Name
		a.dialogFields[9].cursor = len(selected.Config.Name)
		a.dialogFields[10].value = selected.Config.Tag
		a.dialogFields[10].cursor = len(selected.Config.Tag)
		if selected.Config.GcpIap != nil {
			a.dialogFields[11].value = selected.Config.GcpIap.Zone
			a.dialogFields[11].cursor = len(selected.Config.GcpIap.Zone)
			a.dialogFields[12].value = selected.Config.GcpIap.Project
			a.dialogFields[12].cursor = len(selected.Config.GcpIap.Project)
			a.dialogFields[13].value = selected.Config.GcpIap.GcloudConfiguration
			a.dialogFields[13].cursor = len(selected.Config.GcpIap.GcloudConfiguration)
			a.dialogFields[0].value = "gcloud"
			a.dialogFields[1].isHidden = false
			for i := 2; i <= 8; i++ {
				a.dialogFields[i].isHidden = true
			}
		} else {
			a.dialogFields[0].value = "ssh"
			a.dialogFields[1].isHidden = false
			for i := 2; i <= 8; i++ {
				a.dialogFields[i].isHidden = true
			}
		}
	}

	if mode == modeNew || (mode == modeEdit && a.editingIndex < len(a.tunnels) && a.tunnels[a.editingIndex].Status != "active") {
		for i := range a.dialogFields {
			if !a.dialogFields[i].isHidden {
				a.activeField = i
				break
			}
		}
	}
}

func (a *App) handleDialogSubmit() {
	var updatedConfig *config.TunnelConfig
	var err error

	if a.dialogMode == modeEdit {
		selected := &a.tunnels[a.editingIndex]
		if selected.Status == "active" {
			selected.Config.Name = a.dialogFields[9].value
			selected.Config.Tag = a.dialogFields[10].value
			a.Logf("Updated tunnel name/tag: %s", selected.Config.Name)
			a.updateTableRows()
			a.saveConfig()
			a.showDialog = false
			return
		}
	}

	a.dialogError = ""
	if a.dialogFields[0].value == "gcloud" {
		input := strings.TrimSpace(a.dialogFields[1].value)
		if input == "" && strings.Contains(a.dialogFields[9].value, "gcloud") && strings.Contains(a.dialogFields[9].value, "tunnel-through-iap") {
			input = strings.TrimSpace(a.dialogFields[9].value)
		}
		updatedConfig, err = parse.ParseGcloudSshString(input)
		if err != nil {
			a.dialogError = fmt.Sprintf("Error: %v", err)
			return
		}
		if updatedConfig == nil {
			updatedConfig, err = parse.ParseGcloudStartIapTunnel(input)
			if err != nil {
				a.dialogError = fmt.Sprintf("Error: %v", err)
				return
			}
			if updatedConfig == nil {
				if input == "" {
					a.dialogError = "Paste your gcloud command in the Gcloud Command field (press / until you see it), then Enter"
				} else {
					a.dialogError = "Invalid gcloud command: use compute ssh --tunnel-through-iap or compute start-iap-tunnel"
				}
				return
			}
		}
		if updatedConfig.GcpIap != nil {
			if a.dialogFields[11].value == "" {
				a.dialogFields[11].value = updatedConfig.GcpIap.Zone
			}
			if a.dialogFields[12].value == "" {
				a.dialogFields[12].value = updatedConfig.GcpIap.Project
			}
			if a.dialogFields[13].value == "" {
				a.dialogFields[13].value = updatedConfig.GcpIap.GcloudConfiguration
			}
		}
	} else if a.dialogFields[0].value == "ssh" {
		updatedConfig, err = parse.ParseGcloudSshString(a.dialogFields[1].value)
		if err != nil {
			a.dialogError = fmt.Sprintf("Error: %v", err)
			return
		}
		if updatedConfig == nil {
			updatedConfig, err = parse.ParseGcloudStartIapTunnel(a.dialogFields[1].value)
			if err != nil {
				a.dialogError = fmt.Sprintf("Error: %v", err)
				return
			}
		}
		if updatedConfig == nil {
			updatedConfig, err = parse.ParseSshString(a.dialogFields[1].value)
			if err != nil {
				a.dialogError = fmt.Sprintf("Error: %v", err)
				return
			}
		}
		if updatedConfig != nil && updatedConfig.GcpIap != nil {
			if a.dialogFields[11].value == "" {
				a.dialogFields[11].value = updatedConfig.GcpIap.Zone
			}
			if a.dialogFields[12].value == "" {
				a.dialogFields[12].value = updatedConfig.GcpIap.Project
			}
			if a.dialogFields[13].value == "" {
				a.dialogFields[13].value = updatedConfig.GcpIap.GcloudConfiguration
			}
		}
	} else {
		localPort, err := strconv.Atoi(a.dialogFields[3].value)
		if err != nil {
			a.dialogError = "Invalid local port"
			return
		}
		remotePort, err := strconv.Atoi(a.dialogFields[5].value)
		if err != nil {
			a.dialogError = "Invalid remote port"
			return
		}

		var bastion struct {
			Host string `yaml:"host"`
			User string `yaml:"user"`
			Port int    `yaml:"port,omitempty"`
		}
		if a.dialogFields[6].value != "" {
			bastion.Host = a.dialogFields[6].value
			bastion.User = a.dialogFields[8].value
			if a.dialogFields[7].value != "" {
				port, err := strconv.Atoi(a.dialogFields[7].value)
				if err != nil {
					a.dialogError = "Invalid bastion port number"
					return
				}
				bastion.Port = port
			} else {
				bastion.Port = 22
			}
		}

		updatedConfig = &config.TunnelConfig{
			LocalPort:   localPort,
			RemoteHost:  a.dialogFields[4].value,
			RemotePort:  remotePort,
			BindAddress: a.dialogFields[2].value,
			Bastion:     bastion,
		}
		if a.dialogFields[11].value != "" {
			updatedConfig.GcpIap = &config.GcpIapConfig{
				Zone:                strings.TrimSpace(a.dialogFields[11].value),
				Project:             strings.TrimSpace(a.dialogFields[12].value),
				GcloudConfiguration: strings.TrimSpace(a.dialogFields[13].value),
			}
		}

		if updatedConfig.Name == "" {
			updatedConfig.Name = updatedConfig.RemoteHost
		}
	}

	if a.dialogFields[9].value != "" {
		updatedConfig.Name = a.dialogFields[9].value
	}
	updatedConfig.Tag = a.dialogFields[10].value

	gcpZone := strings.TrimSpace(a.dialogFields[11].value)
	gcpProject := strings.TrimSpace(a.dialogFields[12].value)
	gcpGcloudConfig := strings.TrimSpace(a.dialogFields[13].value)
	parsedFromGcloudOrSsh := a.dialogFields[0].value == "gcloud" || (a.dialogFields[0].value == "ssh" && updatedConfig.GcpIap != nil)
	if gcpZone != "" {
		if updatedConfig.GcpIap == nil {
			updatedConfig.GcpIap = &config.GcpIapConfig{}
		}
		updatedConfig.GcpIap.Zone = gcpZone
		updatedConfig.GcpIap.Project = gcpProject
		updatedConfig.GcpIap.GcloudConfiguration = gcpGcloudConfig
		if !parsedFromGcloudOrSsh && a.dialogMode == modeEdit && a.editingIndex < len(a.tunnels) && a.tunnels[a.editingIndex].Config.GcpIap != nil {
			updatedConfig.GcpIap.Mode = a.tunnels[a.editingIndex].Config.GcpIap.Mode
		}
	} else if (gcpProject != "" || gcpGcloudConfig != "") && updatedConfig.GcpIap != nil {
		updatedConfig.GcpIap.Project = gcpProject
		updatedConfig.GcpIap.GcloudConfiguration = gcpGcloudConfig
		if !parsedFromGcloudOrSsh && a.dialogMode == modeEdit && a.editingIndex < len(a.tunnels) && a.tunnels[a.editingIndex].Config.GcpIap != nil {
			updatedConfig.GcpIap.Mode = a.tunnels[a.editingIndex].Config.GcpIap.Mode
		}
	}

	if a.dialogMode == modeEdit {
		selected := &a.tunnels[a.editingIndex]
		selected.Config = *updatedConfig
		a.Logf("Updated tunnel: %s", updatedConfig.Name)
	} else {
		tunnel := TunnelRecord{
			ID:      uuid.New().String(),
			Status:  "stopped",
			Config:  *updatedConfig,
			Metrics: "--",
		}
		a.tunnels = append(a.tunnels, tunnel)
		a.Logf("Added new tunnel: %s", updatedConfig.Name)
	}

	a.updateTableRows()
	a.saveConfig()
	a.showDialog = false
}

// handleDialogKey processes a key event for the create/edit tunnel dialog.
// Returns true if the key was handled (caller should return and not process further).
func (a *App) handleDialogKey(msg tea.KeyMsg) bool {
	if msg.String() == "/" {
		switch a.dialogFields[0].value {
		case "fields":
			a.dialogFields[0].value = "gcloud"
			for i := 2; i <= 8; i++ {
				a.dialogFields[i].isHidden = true
			}
			a.dialogFields[1].isHidden = false
			a.activeField = 1
		case "gcloud":
			a.dialogFields[0].value = "ssh"
			a.activeField = 1
		case "ssh":
			a.dialogFields[0].value = "fields"
			for i := 2; i <= 8; i++ {
				a.dialogFields[i].isHidden = false
			}
			a.dialogFields[1].isHidden = true
			a.activeField = 2
		default:
			a.dialogFields[0].value = "gcloud"
			for i := 2; i <= 8; i++ {
				a.dialogFields[i].isHidden = true
			}
			a.dialogFields[1].isHidden = false
			a.activeField = 1
		}
		return true
	}

	switch msg.Type {
	case tea.KeyRunes:
		if string(msg.Runes) == "/" {
			return true
		}
		field := &a.dialogFields[a.activeField]
		if !field.isHidden {
			if field.cursor == len(field.value) {
				field.value += string(msg.Runes)
			} else {
				field.value = field.value[:field.cursor] + string(msg.Runes) + field.value[field.cursor:]
			}
			field.cursor += len(msg.Runes)
			if a.activeField == 1 {
				a.syncGcloudFieldsFromCommand()
			}
		}
		return true

	case tea.KeySpace:
		field := &a.dialogFields[a.activeField]
		if !field.isHidden {
			if field.cursor == len(field.value) {
				field.value += " "
			} else {
				field.value = field.value[:field.cursor] + " " + field.value[field.cursor:]
			}
			field.cursor++
			if a.activeField == 1 {
				a.syncGcloudFieldsFromCommand()
			}
		}
		return true

	case tea.KeyUp, tea.KeyShiftTab:
		a.activeField = (a.activeField - 1 + len(a.dialogFields)) % len(a.dialogFields)
		for a.dialogFields[a.activeField].isHidden {
			a.activeField = (a.activeField - 1 + len(a.dialogFields)) % len(a.dialogFields)
		}
		return true

	case tea.KeyDown, tea.KeyTab:
		a.activeField = (a.activeField + 1) % len(a.dialogFields)
		for a.dialogFields[a.activeField].isHidden {
			a.activeField = (a.activeField + 1) % len(a.dialogFields)
		}
		return true

	case tea.KeyEnter:
		a.handleDialogSubmit()
		return true

	case tea.KeyEsc, tea.KeyCtrlC:
		a.showDialog = false
		return true

	case tea.KeyBackspace:
		field := &a.dialogFields[a.activeField]
		if len(field.value) > 0 && field.cursor > 0 {
			field.value = field.value[:field.cursor-1] + field.value[field.cursor:]
			field.cursor--
			if a.activeField == 1 {
				a.syncGcloudFieldsFromCommand()
			}
		}
		return true

	case tea.KeyLeft:
		field := &a.dialogFields[a.activeField]
		if field.cursor > 0 {
			field.cursor--
		}
		return true

	case tea.KeyRight:
		field := &a.dialogFields[a.activeField]
		if field.cursor < len(field.value) {
			field.cursor++
		}
		return true

	case tea.KeyHome:
		field := &a.dialogFields[a.activeField]
		field.cursor = 0
		return true

	case tea.KeyEnd:
		field := &a.dialogFields[a.activeField]
		field.cursor = len(field.value)
		return true

	case tea.KeyDelete:
		field := &a.dialogFields[a.activeField]
		if field.cursor < len(field.value) {
			field.value = field.value[:field.cursor] + field.value[field.cursor+1:]
			if a.activeField == 1 {
				a.syncGcloudFieldsFromCommand()
			}
		}
		return true
	}

	return false
}

// syncGcloudFieldsFromCommand parses the Gcloud Command field (index 1) and, if it
// looks like a valid gcloud IAP command, populates GCP Zone, GCP Project ID, and
// GCP gcloud config (fields 11, 12, 13) so they are filled when the user pastes.
func (a *App) syncGcloudFieldsFromCommand() {
	if a.activeField != 1 || (a.dialogFields[0].value != "gcloud" && a.dialogFields[0].value != "ssh") {
		return
	}
	input := strings.TrimSpace(a.dialogFields[1].value)
	if input == "" {
		return
	}
	cfg, err := parse.ParseGcloudSshString(input)
	if err != nil || cfg == nil {
		cfg, err = parse.ParseGcloudStartIapTunnel(input)
		if err != nil || cfg == nil {
			return
		}
	}
	if cfg.GcpIap == nil {
		return
	}
	a.dialogFields[11].value = cfg.GcpIap.Zone
	a.dialogFields[11].cursor = len(cfg.GcpIap.Zone)
	a.dialogFields[12].value = cfg.GcpIap.Project
	a.dialogFields[12].cursor = len(cfg.GcpIap.Project)
	if cfg.GcpIap.GcloudConfiguration != "" {
		a.dialogFields[13].value = cfg.GcpIap.GcloudConfiguration
		a.dialogFields[13].cursor = len(cfg.GcpIap.GcloudConfiguration)
	}
}

// renderDialogView returns the create/edit tunnel dialog content for View().
func (a *App) renderDialogView() string {
	title := "Add New Tunnel"
	if a.dialogMode == modeEdit {
		title = "Edit Tunnel"
	}
	content := dialogActiveStyle.Render(title) + "\n"
	currentMode := a.dialogFields[0].value
	highlight := func(name string) string {
		if name == currentMode {
			return dialogActiveStyle.Render(name)
		}
		return name
	}
	content += "Input mode: (" + highlight("fields") + " → " + highlight("gcloud") + " → " + highlight("ssh") + ", press / to cycle)\n\n"

	maxLabelWidth := 0
	for i, field := range a.dialogFields {
		if !field.isHidden {
			label := field.label
			if i == 1 && a.dialogFields[0].value == "gcloud" {
				label = "Gcloud Command"
			}
			if len(label) > maxLabelWidth {
				maxLabelWidth = len(label)
			}
		}
	}
	maxLabelWidth += 2

	for i, field := range a.dialogFields {
		if !field.isHidden {
			fieldLabel := field.label
			if i == 1 && a.dialogFields[0].value == "gcloud" {
				fieldLabel = "Gcloud Command"
			}
			labelContent := fieldLabel + ":"
			if i == a.activeField {
				labelContent = "> " + labelContent
			} else {
				labelContent = "  " + labelContent
			}
			for len(labelContent) < maxLabelWidth+4 {
				labelContent += " "
			}

			if i == a.activeField {
				content += dialogSelectedStyle.Render(labelContent)
			} else {
				content += labelContent
			}

			if i == a.activeField {
				valueContent := field.value
				if field.cursor == len(field.value) {
					valueContent += " "
					content += dialogSelectedStyle.Render(valueContent[:len(valueContent)-1]) + lipgloss.NewStyle().Underline(true).Render(" ")
				} else {
					beforeCursor := valueContent[:field.cursor]
					atCursor := lipgloss.NewStyle().Underline(true).Render(string(valueContent[field.cursor]))
					afterCursor := valueContent[field.cursor+1:]
					content += dialogSelectedStyle.Render(beforeCursor) + atCursor + dialogSelectedStyle.Render(afterCursor)
				}
			} else {
				content += field.value
			}
			content += "\n"
			if i == 1 || i == 5 || i == 8 {
				content += "\n"
			}
		}
	}

	if a.dialogError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true)
		content += "\n" + errorStyle.Render(a.dialogError) + "\n"
	}

	switch a.dialogFields[0].value {
	case "gcloud":
		content += "\nPaste a gcloud IAP command:\n"
		content += "  compute ssh … --tunnel-through-iap\n"
		content += "  or compute start-iap-tunnel … (include --zone=)\n"
	case "ssh":
		content += "\nFormat: ssh -N -L [bindAddress:]localPort:remoteHost:remotePort [user@host[:port]]\n"
		content += "Or paste a gcloud IAP command (use / to switch to Gcloud mode)\n"
	}
	content += "\n↑/↓: Change field • Enter: Save • Esc/Ctrl+C: Cancel • /: Cycle mode"

	dialogWidth := a.width * 85 / 100
	if dialogWidth < 80 {
		dialogWidth = 80
	}
	dialog := dialogStyle.Width(dialogWidth).Render(content)
	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}
