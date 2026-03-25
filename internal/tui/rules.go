package tui

import (
	"fmt"
	"strings"
	"time"

	"log"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

const (
	fieldName = iota
	fieldAction
	fieldDuration
	fieldOpType
	fieldOpOperand
	fieldOpData
	fieldEnabled
	fieldPrecedence
	fieldNolog
	fieldDescription
	fieldCount
)

var (
	actionOptions = []string{"allow", "deny", "reject"}
	durationOpts  = []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}
	opTypeOptions = []string{"simple", "regexp", "network", "list", "lists", "range"}
	operandOpts   = []string{
		"process.path", "process.command", "process.id",
		"process.parent.path", "process.hash.md5", "process.hash.sha1",
		"user.id", "user.name",
		"dest.ip", "dest.host", "dest.port", "dest.network",
		"source.ip", "source.port", "source.network",
		"protocol", "iface.in", "iface.out",
		"lists.domains", "lists.domains_regexp", "lists.ips", "lists.nets",
	}
)

type editorMode int

const (
	modeNone editorMode = iota
	modeAdd
	modeEdit
)

type rulesModel struct {
	width, height int
	rules         []db.RuleRow
	cursor        int
	database      *db.DB
	eventBus      *bus.EventBus

	editing      editorMode
	editorField  int
	editorValues [fieldCount]string
	textInput    string
	selectIdx    [fieldCount]int
	confirmDel   bool
	editOrigName string // original rule name before edit (to detect renames)
	editOrigNode string
}

func newRulesModel(database *db.DB, eventBus *bus.EventBus) *rulesModel {
	return &rulesModel{database: database, eventBus: eventBus}
}

func (m *rulesModel) loadRules() {
	rules, err := m.database.GetRules("")
	if err != nil {
		log.Printf("ERROR loadRules: %v", err)
		return
	}
	m.rules = rules
	if m.cursor >= len(m.rules) {
		m.cursor = len(m.rules) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *rulesModel) startAdd() {
	m.editing = modeAdd
	m.editorField = fieldName
	m.editorValues = [fieldCount]string{
		fieldName: "", fieldAction: "deny", fieldDuration: "always",
		fieldOpType: "simple", fieldOpOperand: "process.path", fieldOpData: "",
		fieldEnabled: "true", fieldPrecedence: "false", fieldNolog: "false",
		fieldDescription: "",
	}
	m.selectIdx = [fieldCount]int{fieldAction: 1, fieldDuration: 8}
	m.textInput = ""
}

// startAddFromConnection pre-fills the editor from a grouped connection.
func (m *rulesModel) startAddFromConnection(g *connGroup) {
	m.editing = modeAdd
	m.editorField = fieldAction

	// Determine the best operand based on what data we have.
	operand := "process.path"
	data := g.Process
	if g.Dest != "" {
		operand = "dest.host"
		data = g.Dest
	}

	name := fmt.Sprintf("allow-%s-%d", g.Process, g.Port)

	m.editorValues = [fieldCount]string{
		fieldName: name, fieldAction: "allow", fieldDuration: "always",
		fieldOpType: "simple", fieldOpOperand: operand, fieldOpData: data,
		fieldEnabled: "true", fieldPrecedence: "false", fieldNolog: "false",
		fieldDescription: fmt.Sprintf("Created from connection: %s -> %s:%d", g.Process, g.Dest, g.Port),
	}
	m.selectIdx[fieldAction] = 0  // allow
	m.selectIdx[fieldDuration] = 8 // always
	m.selectIdx[fieldOpType] = 0   // simple
	m.selectIdx[fieldOpOperand] = indexOf(operandOpts, operand)
	m.textInput = m.editorValues[m.editorField]
}

func (m *rulesModel) startEdit() {
	if m.cursor >= len(m.rules) {
		return
	}
	r := m.rules[m.cursor]
	m.editing = modeEdit
	m.editOrigName = r.Name
	m.editOrigNode = r.Node
	m.editorField = fieldName
	m.editorValues = [fieldCount]string{
		fieldName: r.Name, fieldAction: r.Action, fieldDuration: r.Duration,
		fieldOpType: r.OpType, fieldOpOperand: r.OpOperand, fieldOpData: r.OpData,
		fieldEnabled: r.Enabled, fieldPrecedence: r.Precedence, fieldNolog: r.Nolog,
		fieldDescription: r.Description,
	}
	m.selectIdx[fieldAction] = indexOf(actionOptions, r.Action)
	m.selectIdx[fieldDuration] = indexOf(durationOpts, r.Duration)
	m.selectIdx[fieldOpType] = indexOf(opTypeOptions, r.OpType)
	m.selectIdx[fieldOpOperand] = indexOf(operandOpts, r.OpOperand)
	m.textInput = m.editorValues[m.editorField]
}

func (m *rulesModel) saveEditor() {
	v := m.editorValues
	if v[fieldName] == "" || v[fieldOpData] == "" {
		return
	}
	node := "unix:local"
	if m.editing == modeEdit {
		node = m.editOrigNode
		// If name changed, delete the old rule first.
		if m.editOrigName != "" && m.editOrigName != v[fieldName] {
			if err := m.database.DeleteRule(m.editOrigName, node); err != nil {
				log.Printf("ERROR saveEditor delete old rule(%s): %v", m.editOrigName, err)
			}
			// Tell daemon to delete the old rule.
			delNotif := &pb.Notification{
				Id: uint64(time.Now().UnixNano()), Type: pb.Action_DELETE_RULE,
				Rules: []*pb.Rule{{Name: m.editOrigName}},
			}
			select {
			case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: node, Notification: delNotif}:
			default:
				log.Printf("WARN notification channel full, dropped DELETE_RULE for %s", m.editOrigName)
			}
		}
	}
	created := time.Now().Format(time.RFC3339)
	if err := m.database.InsertRule(node, v[fieldName], v[fieldEnabled], v[fieldPrecedence],
		v[fieldAction], v[fieldDuration], v[fieldOpType], "false",
		v[fieldOpOperand], v[fieldOpData], v[fieldDescription], v[fieldNolog], created); err != nil {
		log.Printf("ERROR saveEditor InsertRule(%s): %v", v[fieldName], err)
	}

	rule := &pb.Rule{
		Created: time.Now().Unix(), Name: v[fieldName], Description: v[fieldDescription],
		Enabled: v[fieldEnabled] == "true", Precedence: v[fieldPrecedence] == "true",
		Nolog: v[fieldNolog] == "true", Action: v[fieldAction], Duration: v[fieldDuration],
		Operator: &pb.Operator{Type: v[fieldOpType], Operand: v[fieldOpOperand], Data: v[fieldOpData]},
	}
	notif := &pb.Notification{
		Id: uint64(time.Now().UnixNano()), Type: pb.Action_CHANGE_RULE, Rules: []*pb.Rule{rule},
	}
	select {
	case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: node, Notification: notif}:
	default:
	}
	m.editing = modeNone
	m.loadRules()
}

func (m *rulesModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.confirmDel {
			if msg.String() == "y" || msg.String() == "Y" {
				m.doDelete()
			}
			m.confirmDel = false
			return nil
		}
		if m.editing != modeNone {
			return m.updateEditor(msg)
		}
		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.rules)-1 {
				m.cursor++
			}
		case key.Matches(msg, keys.Add):
			m.startAdd()
		case key.Matches(msg, keys.Edit), key.Matches(msg, keys.Enter):
			m.startEdit()
		case key.Matches(msg, keys.Toggle):
			m.toggleRule()
		case key.Matches(msg, keys.Delete):
			if len(m.rules) > 0 {
				m.confirmDel = true
			}
		}
	}
	return nil
}

func (m *rulesModel) updateEditor(msg tea.KeyMsg) tea.Cmd {
	f := m.editorField
	switch msg.String() {
	case "esc":
		m.editing = modeNone
		return nil
	case "enter":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		if f == fieldCount-1 {
			m.saveEditor()
			return nil
		}
		m.editorField++
		m.textInput = m.editorValues[m.editorField]
		return nil
	case "tab":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		m.editorField = (m.editorField + 1) % fieldCount
		m.textInput = m.editorValues[m.editorField]
		return nil
	case "shift+tab":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		m.editorField = (m.editorField - 1 + fieldCount) % fieldCount
		m.textInput = m.editorValues[m.editorField]
		return nil
	case "ctrl+s":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		m.saveEditor()
		return nil
	}

	switch {
	case isDropdownField(f):
		opts := getOptions(f)
		switch msg.String() {
		case "left", "h":
			m.selectIdx[f] = (m.selectIdx[f] - 1 + len(opts)) % len(opts)
			m.editorValues[f] = opts[m.selectIdx[f]]
		case "right", "l":
			m.selectIdx[f] = (m.selectIdx[f] + 1) % len(opts)
			m.editorValues[f] = opts[m.selectIdx[f]]
		}
	case isBoolField(f):
		if msg.String() == " " || msg.String() == "left" || msg.String() == "right" {
			if m.editorValues[f] == "true" {
				m.editorValues[f] = "false"
			} else {
				m.editorValues[f] = "true"
			}
		}
	case isTextField(f):
		switch msg.String() {
		case "backspace":
			if len(m.textInput) > 0 {
				m.textInput = m.textInput[:len(m.textInput)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.textInput += msg.String()
			}
		}
	}
	return nil
}

func (m *rulesModel) toggleRule() {
	if m.cursor >= len(m.rules) {
		return
	}
	r := m.rules[m.cursor]
	newEnabled := "true"
	if r.Enabled == "true" {
		newEnabled = "false"
	}
	if err := m.database.UpdateRuleEnabled(r.Name, r.Node, newEnabled); err != nil {
		log.Printf("ERROR toggleRule(%s): %v", r.Name, err)
	}
	action := pb.Action_ENABLE_RULE
	if newEnabled == "false" {
		action = pb.Action_DISABLE_RULE
	}
	notif := &pb.Notification{
		Id: uint64(time.Now().UnixNano()), Type: action,
		Rules: []*pb.Rule{{Name: r.Name, Enabled: newEnabled == "true"}},
	}
	select {
	case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: r.Node, Notification: notif}:
	default:
	}
	m.loadRules()
}

func (m *rulesModel) doDelete() {
	if m.cursor >= len(m.rules) {
		return
	}
	r := m.rules[m.cursor]
	if err := m.database.DeleteRule(r.Name, r.Node); err != nil {
		log.Printf("ERROR deleteRule(%s): %v", r.Name, err)
	}
	notif := &pb.Notification{
		Id: uint64(time.Now().UnixNano()), Type: pb.Action_DELETE_RULE,
		Rules: []*pb.Rule{{Name: r.Name}},
	}
	select {
	case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: r.Node, Notification: notif}:
	default:
	}
	m.loadRules()
}

func (m *rulesModel) View() string {
	// Split: list on top (~60%), detail/editor card on bottom (~40%).
	listHeight := (m.height * 60) / 100
	if listHeight < 6 {
		listHeight = 6
	}
	detailHeight := m.height - listHeight - 1
	if detailHeight < 5 {
		detailHeight = 5
	}
	tableRows := listHeight - 3
	if tableRows < 1 {
		tableRows = 1
	}

	// --- Rule list (always visible) ---
	header := tableHeaderStyle.Render(fmt.Sprintf(
		" %-3s  %-28s  %-8s  %-14s  %-3s",
		"#", "NAME", "ACTION", "DURATION", "EN",
	))

	var rows []string
	for i, r := range m.rules {
		if i >= tableRows {
			break
		}
		name := r.Name
		if len(name) > 28 {
			name = name[:27] + "…"
		}
		en := "Y"
		if r.Enabled != "true" {
			en = "N"
		}

		if i == m.cursor {
			// Selected row: plain text, let tableSelectedStyle control all colors.
			row := fmt.Sprintf(" %-3d  %-28s  %-8s  %-14s  %s",
				i+1, name, r.Action, r.Duration, en)
			rows = append(rows, tableSelectedStyle.Width(m.width-4).Render(row))
		} else {
			// Non-selected: style individual cells.
			actionStyle := statValueStyle
			switch r.Action {
			case "allow":
				actionStyle = statAcceptStyle
			case "deny", "reject":
				actionStyle = statDropStyle
			}
			enStyle := lipgloss.NewStyle().Foreground(colorGreen)
			if en == "N" {
				enStyle = lipgloss.NewStyle().Foreground(colorRed)
			}
			row := fmt.Sprintf(" %-3d  %-28s  %s  %-14s  %s",
				i+1, name, actionStyle.Render(fmt.Sprintf("%-8s", r.Action)),
				r.Duration, enStyle.Render(en))
			if i%2 == 0 {
				rows = append(rows, tableRowStyle.Render(row))
			} else {
				rows = append(rows, tableRowAltStyle.Render(row))
			}
		}
	}
	for len(rows) < tableRows {
		rows = append(rows, "")
	}
	listContent := header + "\n" + strings.Join(rows, "\n")
	listPanel := panelStyle.Width(m.width).Render(
		panelTitleStyle.Render(fmt.Sprintf("Rules (%d)", len(m.rules))) + "\n" + listContent)

	// --- Detail or editor card (bottom) ---
	var detailPanel string
	if m.editing != modeNone {
		detailPanel = m.renderEditorCard(detailHeight)
	} else {
		detailPanel = m.renderDetailCard(detailHeight)
	}

	// --- Footer ---
	footer := lipgloss.NewStyle().Foreground(colorDim).Render(
		"  [a]dd  [e]dit  [t]oggle  [d]elete  [↑↓] navigate")
	if m.confirmDel && m.cursor < len(m.rules) {
		footer = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(
			fmt.Sprintf("  Delete '%s'? [y] confirm / any key cancel", m.rules[m.cursor].Name))
	}

	return lipgloss.JoinVertical(lipgloss.Left, listPanel, detailPanel, footer)
}

func (m *rulesModel) renderDetailCard(height int) string {
	if len(m.rules) == 0 || m.cursor >= len(m.rules) {
		return panelStyle.Width(m.width).Height(height).Render(
			panelTitleStyle.Render("Details") + "\n" +
				lipgloss.NewStyle().Foreground(colorDim).Render("No rule selected"))
	}
	r := m.rules[m.cursor]

	lbl := lipgloss.NewStyle().Foreground(colorDim).Width(14)
	val := lipgloss.NewStyle().Foreground(colorFg)
	accent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	actionStyle := statAcceptStyle
	if r.Action == "deny" || r.Action == "reject" {
		actionStyle = statDropStyle
	}
	enStyle := lipgloss.NewStyle().Foreground(colorGreen)
	if r.Enabled != "true" {
		enStyle = lipgloss.NewStyle().Foreground(colorRed)
	}

	lines := []string{
		lbl.Render("Name:") + accent.Render(r.Name),
		lbl.Render("Action:") + actionStyle.Render(r.Action) +
			"    " + lbl.Render("Duration:") + val.Render(r.Duration),
		lbl.Render("Enabled:") + enStyle.Render(r.Enabled) +
			"    " + lbl.Render("Precedence:") + val.Render(r.Precedence) +
			"    " + lbl.Render("No Log:") + val.Render(r.Nolog),
		lbl.Render("Op Type:") + val.Render(r.OpType) +
			"    " + lbl.Render("Operand:") + val.Render(r.OpOperand),
		lbl.Render("Data:") + val.Render(r.OpData),
	}
	if r.Description != "" {
		lines = append(lines, lbl.Render("Description:") + val.Render(r.Description))
	}
	if r.Node != "" {
		lines = append(lines, lbl.Render("Node:") + val.Render(r.Node)+
			"    "+lbl.Render("Created:") + lipgloss.NewStyle().Foreground(colorDim).Render(r.Created))
	}

	content := strings.Join(lines, "\n")
	return panelActiveStyle.Width(m.width).Height(height).Render(
		panelTitleStyle.Render("Rule Details") + "\n" + content)
}

// renderEditorCard renders the editor inline in the detail card area.
func (m *rulesModel) renderEditorCard(height int) string {
	title := "Add Rule"
	if m.editing == modeEdit {
		title = "Edit Rule"
	}

	type fieldDef struct {
		label string
		idx   int
	}
	fields := []fieldDef{
		{"Name", fieldName}, {"Action", fieldAction}, {"Duration", fieldDuration},
		{"Op Type", fieldOpType}, {"Operand", fieldOpOperand}, {"Data", fieldOpData},
		{"Enabled", fieldEnabled}, {"Precedence", fieldPrecedence}, {"No Log", fieldNolog},
		{"Description", fieldDescription},
	}

	lbl := lipgloss.NewStyle().Foreground(colorDim).Width(14)

	var rows []string
	for _, f := range fields {
		val := m.editorValues[f.idx]
		if isTextField(f.idx) && f.idx == m.editorField {
			val = m.textInput
		}

		var rendered string
		if f.idx == m.editorField {
			if isDropdownField(f.idx) {
				rendered = renderDropdown(getOptions(f.idx), m.selectIdx[f.idx])
			} else if isBoolField(f.idx) {
				if val == "true" {
					rendered = promptSelectedStyle.Render(" ON ") + " " +
						lipgloss.NewStyle().Foreground(colorDim).Render("OFF")
				} else {
					rendered = lipgloss.NewStyle().Foreground(colorDim).Render("ON") + " " +
						promptSelectedStyle.Render(" OFF ")
				}
			} else {
				rendered = lipgloss.NewStyle().Foreground(colorWhite).
					Background(lipgloss.Color("#3b4261")).Render(val + "█")
			}
		} else {
			rendered = lipgloss.NewStyle().Foreground(colorFg).Render(val)
		}
		rows = append(rows, lbl.Render(f.label+":")+rendered)
	}

	hint := "[Tab] next  [Ctrl+S] save  [Esc] cancel"
	if isDropdownField(m.editorField) {
		hint = "[←/→] change  [Tab] next  [Ctrl+S] save  [Esc] cancel"
	} else if isBoolField(m.editorField) {
		hint = "[Space] toggle  [Tab] next  [Ctrl+S] save  [Esc] cancel"
	}

	content := strings.Join(rows, "\n") + "\n" +
		lipgloss.NewStyle().Foreground(colorDim).Render("  "+hint)

	return panelActiveStyle.Width(m.width).Height(height).Render(
		panelTitleStyle.Render(title) + "\n" + content)
}

// --- helpers ---

func isTextField(f int) bool {
	return f == fieldName || f == fieldOpData || f == fieldDescription
}
func isDropdownField(f int) bool {
	return f == fieldAction || f == fieldDuration || f == fieldOpType || f == fieldOpOperand
}
func isBoolField(f int) bool {
	return f == fieldEnabled || f == fieldPrecedence || f == fieldNolog
}
func getOptions(f int) []string {
	switch f {
	case fieldAction:
		return actionOptions
	case fieldDuration:
		return durationOpts
	case fieldOpType:
		return opTypeOptions
	case fieldOpOperand:
		return operandOpts
	}
	return nil
}
func renderDropdown(opts []string, selected int) string {
	var parts []string
	for i, o := range opts {
		if i == selected {
			parts = append(parts, promptSelectedStyle.Render(" "+o+" "))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorDim).Render(o))
		}
	}
	return strings.Join(parts, " ")
}
func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return 0
}
