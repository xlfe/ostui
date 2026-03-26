package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/db"
	pb "github.com/safedoor/ostui/proto/protocol"
)

// Editor field indices.
const (
	fieldName = iota
	fieldAction
	fieldDuration
	fieldConditions // compound conditions section (replaces OpType/Operand/Data)
	fieldEnabled
	fieldPrecedence
	fieldNolog
	fieldDescription
	fieldCount
)

// Condition sub-field indices.
const (
	condFieldOperand = iota
	condFieldType
	condFieldData
	condFieldCount
)

var (
	actionOptions = []string{"allow", "deny", "reject"}
	durationOpts  = []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}
	condTypeOpts  = []string{"simple", "regexp", "network"}
	condOperandOpts = []string{
		"process.path", "process.command", "process.id",
		"process.parent.path", "process.hash.md5", "process.hash.sha1",
		"user.id", "user.name",
		"dest.ip", "dest.host", "dest.port", "dest.network",
		"source.ip", "source.port", "source.network",
		"protocol", "iface.in", "iface.out",
	}
)

type editorCondition struct {
	operandIdx int
	typeIdx    int
	data       string
}

func (c editorCondition) operand() string {
	if c.operandIdx >= 0 && c.operandIdx < len(condOperandOpts) {
		return condOperandOpts[c.operandIdx]
	}
	return condOperandOpts[0]
}

func (c editorCondition) opType() string {
	if c.typeIdx >= 0 && c.typeIdx < len(condTypeOpts) {
		return condTypeOpts[c.typeIdx]
	}
	return "simple"
}

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
	editOrigName string
	editOrigNode string
	statusMsg    string
	statusTime   time.Time

	// Conditions editor state.
	conditions    []editorCondition
	condCursor    int
	condSubField  int
	condTextInput string
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

func (m *rulesModel) initConditions(conds []editorCondition) {
	m.conditions = conds
	m.condCursor = 0
	m.condSubField = condFieldOperand
	m.condTextInput = ""
}

func (m *rulesModel) startAdd() {
	m.editing = modeAdd
	m.editorField = fieldName
	m.editorValues = [fieldCount]string{
		fieldName: "", fieldAction: "deny", fieldDuration: "always",
		fieldEnabled: "true", fieldPrecedence: "false", fieldNolog: "false",
		fieldDescription: "",
	}
	m.selectIdx = [fieldCount]int{fieldAction: 1, fieldDuration: 8}
	m.textInput = ""
	m.initConditions([]editorCondition{{operandIdx: 0, typeIdx: 0, data: ""}})
}

// startAddFromConnection pre-fills the editor from a grouped connection.
func (m *rulesModel) startAddFromConnection(g *connGroup) {
	m.editing = modeAdd
	m.editorField = fieldAction

	operand := "process.path"
	data := g.Process
	if g.Dest != "" {
		operand = "dest.host"
		data = g.Dest
	}

	name := fmt.Sprintf("allow-%s-%d", g.Process, g.Port)

	m.editorValues = [fieldCount]string{
		fieldName: name, fieldAction: "allow", fieldDuration: "always",
		fieldEnabled: "true", fieldPrecedence: "false", fieldNolog: "false",
		fieldDescription: fmt.Sprintf("Created from connection: %s -> %s:%d", g.Process, g.Dest, g.Port),
	}
	m.selectIdx[fieldAction] = 0  // allow
	m.selectIdx[fieldDuration] = 8 // always
	m.textInput = m.editorValues[m.editorField]
	m.initConditions([]editorCondition{{
		operandIdx: indexOf(condOperandOpts, operand),
		typeIdx:    0,
		data:       data,
	}})
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
		fieldEnabled: r.Enabled, fieldPrecedence: r.Precedence, fieldNolog: r.Nolog,
		fieldDescription: r.Description,
	}
	m.selectIdx[fieldAction] = indexOf(actionOptions, r.Action)
	m.selectIdx[fieldDuration] = indexOf(durationOpts, r.Duration)
	m.textInput = m.editorValues[m.editorField]

	// Parse conditions from existing rule.
	if r.OpType == "list" || r.OpType == "lists" {
		var subs []subOperator
		if err := json.Unmarshal([]byte(r.OpData), &subs); err == nil && len(subs) > 0 {
			conds := make([]editorCondition, len(subs))
			for i, sub := range subs {
				conds[i] = editorCondition{
					operandIdx: indexOf(condOperandOpts, sub.Operand),
					typeIdx:    indexOf(condTypeOpts, sub.Type),
					data:       sub.Data,
				}
			}
			m.initConditions(conds)
			return
		}
	}
	// Simple rule or parse failure — single condition.
	m.initConditions([]editorCondition{{
		operandIdx: indexOf(condOperandOpts, r.OpOperand),
		typeIdx:    indexOf(condTypeOpts, r.OpType),
		data:       r.OpData,
	}})
}

func (m *rulesModel) saveEditor() {
	v := m.editorValues
	if v[fieldName] == "" {
		return
	}

	// Commit any in-progress condition text.
	if m.editorField == fieldConditions && m.condSubField == condFieldData && m.condCursor < len(m.conditions) {
		m.conditions[m.condCursor].data = m.condTextInput
	}

	// Filter conditions with non-empty data.
	var valid []editorCondition
	for _, c := range m.conditions {
		if c.data != "" {
			valid = append(valid, c)
		}
	}
	if len(valid) == 0 {
		return
	}

	node := "unix:local"
	if m.editing == modeEdit {
		node = m.editOrigNode
		if m.editOrigName != "" && m.editOrigName != v[fieldName] {
			if err := m.database.DeleteRule(m.editOrigName, node); err != nil {
				log.Printf("ERROR saveEditor delete old rule(%s): %v", m.editOrigName, err)
			}
			delNotif := &pb.Notification{
				Id: uint64(time.Now().UnixNano()), Type: pb.Action_DELETE_RULE,
				Rules: []*pb.Rule{{Name: m.editOrigName}},
			}
			select {
			case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: node, Notification: delNotif}:
			case <-time.After(5 * time.Second):
				log.Fatalf("FATAL: NotifOut channel blocked for 5s, DELETE_RULE for %s lost", m.editOrigName)
			}
		}
	}

	var dbOpType, dbOpOperand, dbOpData string
	var pbOp *pb.Operator

	if len(valid) == 1 {
		c := valid[0]
		dbOpType = c.opType()
		dbOpOperand = c.operand()
		dbOpData = c.data
		pbOp = &pb.Operator{Type: dbOpType, Operand: dbOpOperand, Data: dbOpData}
	} else {
		var subs []*pb.Operator
		var jsonSubs []subOperator
		for _, c := range valid {
			subs = append(subs, &pb.Operator{Type: c.opType(), Operand: c.operand(), Data: c.data})
			jsonSubs = append(jsonSubs, subOperator{Type: c.opType(), Operand: c.operand(), Data: c.data})
		}
		jsonBytes, _ := json.Marshal(jsonSubs)
		dbOpType = "list"
		dbOpOperand = "list"
		dbOpData = string(jsonBytes)
		pbOp = &pb.Operator{Type: "list", Operand: "list", List: subs}
	}

	created := time.Now().Format(time.RFC3339)
	if err := m.database.InsertRule(node, v[fieldName], v[fieldEnabled], v[fieldPrecedence],
		v[fieldAction], v[fieldDuration], dbOpType, "false",
		dbOpOperand, dbOpData, v[fieldDescription], v[fieldNolog], created); err != nil {
		log.Printf("ERROR saveEditor InsertRule(%s): %v", v[fieldName], err)
	}

	rule := &pb.Rule{
		Created: time.Now().Unix(), Name: v[fieldName], Description: v[fieldDescription],
		Enabled: v[fieldEnabled] == "true", Precedence: v[fieldPrecedence] == "true",
		Nolog: v[fieldNolog] == "true", Action: v[fieldAction], Duration: v[fieldDuration],
		Operator: pbOp,
	}
	notif := &pb.Notification{
		Id: uint64(time.Now().UnixNano()), Type: pb.Action_CHANGE_RULE, Rules: []*pb.Rule{rule},
	}
	select {
	case m.eventBus.NotifOut <- bus.OutgoingNotification{NodeAddr: node, Notification: notif}:
	case <-time.After(5 * time.Second):
		log.Fatalf("FATAL: NotifOut channel blocked for 5s, CHANGE_RULE for %s lost", v[fieldName])
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
		case key.Matches(msg, keys.Export):
			m.exportNix()
		}
	}
	return nil
}

func (m *rulesModel) updateEditor(msg tea.KeyMsg) tea.Cmd {
	f := m.editorField

	// Global editor keys.
	switch msg.String() {
	case "esc":
		m.editing = modeNone
		return nil
	case "ctrl+s":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		m.saveEditor()
		return nil
	}

	// Conditions section gets its own handler.
	if f == fieldConditions {
		return m.updateConditionsEditor(msg)
	}

	switch msg.String() {
	case "enter":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		if f == fieldCount-1 {
			m.saveEditor()
			return nil
		}
		m.editorField++
		if m.editorField == fieldConditions {
			m.condSubField = condFieldOperand
			if len(m.conditions) > 0 {
				m.condTextInput = m.conditions[m.condCursor].data
			}
		} else {
			m.textInput = m.editorValues[m.editorField]
		}
		return nil
	case "tab":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		m.editorField = (m.editorField + 1) % fieldCount
		if m.editorField == fieldConditions {
			m.condSubField = condFieldOperand
		} else {
			m.textInput = m.editorValues[m.editorField]
		}
		return nil
	case "shift+tab":
		if isTextField(f) {
			m.editorValues[f] = m.textInput
		}
		m.editorField = (m.editorField - 1 + fieldCount) % fieldCount
		if m.editorField == fieldConditions {
			// Enter from below — put cursor on last condition's data.
			m.condCursor = len(m.conditions) - 1
			m.condSubField = condFieldData
			m.condTextInput = m.conditions[m.condCursor].data
		} else {
			m.textInput = m.editorValues[m.editorField]
		}
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

func (m *rulesModel) updateConditionsEditor(msg tea.KeyMsg) tea.Cmd {
	if len(m.conditions) == 0 {
		return nil
	}
	c := &m.conditions[m.condCursor]
	sf := m.condSubField

	switch msg.String() {
	case "tab":
		if sf == condFieldData {
			c.data = m.condTextInput
		}
		if sf < condFieldCount-1 {
			m.condSubField++
			if m.condSubField == condFieldData {
				m.condTextInput = c.data
			}
		} else {
			// On data of current condition — advance to next condition or leave.
			if m.condCursor < len(m.conditions)-1 {
				m.condCursor++
				m.condSubField = condFieldOperand
			} else {
				m.editorField++
				m.textInput = m.editorValues[m.editorField]
			}
		}
		return nil

	case "shift+tab":
		if sf == condFieldData {
			c.data = m.condTextInput
		}
		if sf > 0 {
			m.condSubField--
			if m.condSubField == condFieldData {
				m.condTextInput = m.conditions[m.condCursor].data
			}
		} else {
			if m.condCursor > 0 {
				m.condCursor--
				m.condSubField = condFieldData
				m.condTextInput = m.conditions[m.condCursor].data
			} else {
				m.editorField--
				m.textInput = m.editorValues[m.editorField]
			}
		}
		return nil

	case "enter":
		// Same as tab within conditions.
		if sf == condFieldData {
			c.data = m.condTextInput
		}
		if sf < condFieldCount-1 {
			m.condSubField++
			if m.condSubField == condFieldData {
				m.condTextInput = c.data
			}
		} else if m.condCursor < len(m.conditions)-1 {
			m.condCursor++
			m.condSubField = condFieldOperand
		} else {
			m.editorField++
			m.textInput = m.editorValues[m.editorField]
		}
		return nil

	case "up":
		if sf == condFieldData {
			c.data = m.condTextInput
		}
		if m.condCursor > 0 {
			m.condCursor--
			if m.condSubField == condFieldData {
				m.condTextInput = m.conditions[m.condCursor].data
			}
		}
		return nil

	case "down":
		if sf == condFieldData {
			c.data = m.condTextInput
		}
		if m.condCursor < len(m.conditions)-1 {
			m.condCursor++
			if m.condSubField == condFieldData {
				m.condTextInput = m.conditions[m.condCursor].data
			}
		}
		return nil

	case "ctrl+a":
		if sf == condFieldData {
			c.data = m.condTextInput
		}
		pos := m.condCursor + 1
		m.conditions = slices.Insert(m.conditions, pos, editorCondition{operandIdx: 0, typeIdx: 0})
		m.condCursor = pos
		m.condSubField = condFieldOperand
		return nil

	case "ctrl+d":
		if len(m.conditions) > 1 {
			if sf == condFieldData {
				c.data = m.condTextInput
			}
			m.conditions = slices.Delete(m.conditions, m.condCursor, m.condCursor+1)
			if m.condCursor >= len(m.conditions) {
				m.condCursor = len(m.conditions) - 1
			}
			if m.condSubField == condFieldData {
				m.condTextInput = m.conditions[m.condCursor].data
			}
		}
		return nil
	}

	// Sub-field-specific input.
	switch sf {
	case condFieldOperand:
		switch msg.String() {
		case "left", "h":
			c.operandIdx = (c.operandIdx - 1 + len(condOperandOpts)) % len(condOperandOpts)
		case "right", "l":
			c.operandIdx = (c.operandIdx + 1) % len(condOperandOpts)
		}
	case condFieldType:
		switch msg.String() {
		case "left", "h", "right", "l", " ":
			c.typeIdx = (c.typeIdx + 1) % len(condTypeOpts)
		}
	case condFieldData:
		switch msg.String() {
		case "backspace":
			if len(m.condTextInput) > 0 {
				m.condTextInput = m.condTextInput[:len(m.condTextInput)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.condTextInput += msg.String()
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
	case <-time.After(5 * time.Second):
		log.Fatalf("FATAL: NotifOut channel blocked for 5s, toggle rule %s lost", r.Name)
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
	case <-time.After(5 * time.Second):
		log.Fatalf("FATAL: NotifOut channel blocked for 5s, DELETE_RULE for %s lost", r.Name)
	}
	m.loadRules()
}

const nixExportFile = "opensnitch-rules.nix"

func (m *rulesModel) exportNix() {
	m.loadRules()
	if len(m.rules) == 0 {
		m.statusMsg = "No rules to export"
		m.statusTime = time.Now()
		return
	}

	nix := ExportRulesToNix(m.rules)
	if err := os.WriteFile(nixExportFile, []byte(nix), 0644); err != nil {
		log.Printf("ERROR exportNix: %v", err)
		m.statusMsg = fmt.Sprintf("Export failed: %v", err)
		m.statusTime = time.Now()
		return
	}

	m.statusMsg = fmt.Sprintf("Exported %d rules to %s", len(m.rules), nixExportFile)
	m.statusTime = time.Now()
}

// --- View ---

func (m *rulesModel) View() string {
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
			row := fmt.Sprintf(" %-3d  %-28s  %-8s  %-14s  %s",
				i+1, name, r.Action, r.Duration, en)
			rows = append(rows, tableSelectedStyle.Width(m.width-4).Render(row))
		} else {
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

	var detailPanel string
	if m.editing != modeNone {
		detailPanel = m.renderEditorCard(detailHeight)
	} else {
		detailPanel = m.renderDetailCard(detailHeight)
	}

	footer := lipgloss.NewStyle().Foreground(colorDim).Render(
		"  [a]dd  [e]dit  [t]oggle  [d]elete  [x] export nix  [↑↓] navigate")
	if m.confirmDel && m.cursor < len(m.rules) {
		footer = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(
			fmt.Sprintf("  Delete '%s'? [y] confirm / any key cancel", m.rules[m.cursor].Name))
	}
	if m.statusMsg != "" && time.Since(m.statusTime) < 5*time.Second {
		footer += "\n" + lipgloss.NewStyle().Foreground(colorAccent).Render("  "+m.statusMsg)
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
	}

	// Render conditions from rule.
	if r.OpType == "list" || r.OpType == "lists" {
		var subs []subOperator
		if err := json.Unmarshal([]byte(r.OpData), &subs); err == nil && len(subs) > 0 {
			for i, sub := range subs {
				prefix := "              "
				if i == 0 {
					prefix = lbl.Render("Conditions:")
				}
				suffix := ""
				if i < len(subs)-1 {
					suffix = lipgloss.NewStyle().Foreground(colorDim).Render("  AND")
				}
				lines = append(lines, prefix+val.Render(fmt.Sprintf("%s %s = %s", sub.Type, sub.Operand, sub.Data))+suffix)
			}
		} else {
			lines = append(lines, lbl.Render("Operator:")+val.Render(r.OpType+" "+r.OpOperand+" = "+r.OpData))
		}
	} else {
		lines = append(lines, lbl.Render("Condition:")+val.Render(fmt.Sprintf("%s %s = %s", r.OpType, r.OpOperand, r.OpData)))
	}

	if r.Description != "" {
		lines = append(lines, lbl.Render("Description:")+val.Render(r.Description))
	}
	if r.Node != "" {
		lines = append(lines, lbl.Render("Node:")+val.Render(r.Node)+
			"    "+lbl.Render("Created:")+lipgloss.NewStyle().Foreground(colorDim).Render(r.Created))
	}

	content := strings.Join(lines, "\n")
	return panelActiveStyle.Width(m.width).Height(height).Render(
		panelTitleStyle.Render("Rule Details") + "\n" + content)
}

func (m *rulesModel) renderEditorCard(height int) string {
	title := "Add Rule"
	if m.editing == modeEdit {
		title = "Edit Rule"
	}

	lbl := lipgloss.NewStyle().Foreground(colorDim).Width(14)

	var rows []string

	// Simple fields before conditions.
	for _, f := range []struct {
		label string
		idx   int
	}{
		{"Name", fieldName}, {"Action", fieldAction}, {"Duration", fieldDuration},
	} {
		rows = append(rows, lbl.Render(f.label+":")+m.renderFieldValue(f.idx))
	}

	// Conditions section.
	condActive := m.editorField == fieldConditions
	condLabel := "Conditions:"
	if condActive {
		condLabel = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Conditions:")
	} else {
		condLabel = lbl.Render("Conditions:")
	}

	for i, c := range m.conditions {
		prefix := "  "
		if i == 0 {
			prefix = ""
		}
		isCur := condActive && i == m.condCursor

		operandStr := c.operand()
		typeStr := c.opType()
		dataStr := c.data
		if condActive && isCur && m.condSubField == condFieldData {
			dataStr = m.condTextInput
		}

		var opRendered, typeRendered, dataRendered string

		if isCur && m.condSubField == condFieldOperand {
			opRendered = promptSelectedStyle.Render(" " + operandStr + " ")
		} else {
			opRendered = lipgloss.NewStyle().Foreground(colorFg).Width(18).Render(operandStr)
		}

		if isCur && m.condSubField == condFieldType {
			typeRendered = promptSelectedStyle.Render(" " + typeStr + " ")
		} else {
			typeRendered = lipgloss.NewStyle().Foreground(colorDim).Render(typeStr)
		}

		if isCur && m.condSubField == condFieldData {
			dataRendered = lipgloss.NewStyle().Foreground(colorWhite).
				Background(lipgloss.Color("#3b4261")).Render(dataStr + "█")
		} else {
			dataRendered = lipgloss.NewStyle().Foreground(colorFg).Render(dataStr)
		}

		line := opRendered + "  " + typeRendered + "  " + dataRendered
		if isCur {
			line = promptSelectedStyle.Render("▸") + " " + line
		} else {
			line = "  " + line
		}

		if i == 0 {
			rows = append(rows, condLabel+prefix+line)
		} else {
			rows = append(rows, strings.Repeat(" ", 14)+prefix+line)
		}
	}

	// Fields after conditions.
	for _, f := range []struct {
		label string
		idx   int
	}{
		{"Enabled", fieldEnabled}, {"Precedence", fieldPrecedence}, {"No Log", fieldNolog},
		{"Description", fieldDescription},
	} {
		rows = append(rows, lbl.Render(f.label+":")+m.renderFieldValue(f.idx))
	}

	// Hint.
	var hint string
	if m.editorField == fieldConditions {
		hint = "[←/→] operand  [Tab] next  [↑↓] condition  [Ctrl+A] add  [Ctrl+D] remove  [Ctrl+S] save"
	} else if isDropdownField(m.editorField) {
		hint = "[←/→] change  [Tab] next  [Ctrl+S] save  [Esc] cancel"
	} else if isBoolField(m.editorField) {
		hint = "[Space] toggle  [Tab] next  [Ctrl+S] save  [Esc] cancel"
	} else {
		hint = "[Tab] next  [Ctrl+S] save  [Esc] cancel"
	}

	content := strings.Join(rows, "\n") + "\n" +
		lipgloss.NewStyle().Foreground(colorDim).Render("  "+hint)

	return panelActiveStyle.Width(m.width).Height(height).Render(
		panelTitleStyle.Render(title) + "\n" + content)
}

func (m *rulesModel) renderFieldValue(idx int) string {
	val := m.editorValues[idx]
	if isTextField(idx) && idx == m.editorField {
		val = m.textInput
	}

	if idx == m.editorField {
		if isDropdownField(idx) {
			return renderDropdown(getOptions(idx), m.selectIdx[idx])
		} else if isBoolField(idx) {
			if val == "true" {
				return promptSelectedStyle.Render(" ON ") + " " +
					lipgloss.NewStyle().Foreground(colorDim).Render("OFF")
			}
			return lipgloss.NewStyle().Foreground(colorDim).Render("ON") + " " +
				promptSelectedStyle.Render(" OFF ")
		}
		return lipgloss.NewStyle().Foreground(colorWhite).
			Background(lipgloss.Color("#3b4261")).Render(val + "█")
	}
	return lipgloss.NewStyle().Foreground(colorFg).Render(val)
}

// --- helpers ---

func isTextField(f int) bool {
	return f == fieldName || f == fieldDescription
}
func isDropdownField(f int) bool {
	return f == fieldAction || f == fieldDuration
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
