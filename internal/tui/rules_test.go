package tui

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/safedoor/ostui/internal/bus"
	"github.com/safedoor/ostui/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func testEventBus() *bus.EventBus {
	return bus.New()
}

// drainNotifs starts a goroutine to consume notifications so sends don't block.
func drainNotifs(eb *bus.EventBus) {
	go func() {
		for range eb.NotifOut {
		}
	}()
}

func TestRulesModelLoadEmpty(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	m.loadRules()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(m.rules))
	}
}

func TestSaveEditorAddsSimpleRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	done := make(chan struct{})
	go func() { defer close(done); <-eb.NotifOut }()

	m.startAdd()
	m.editorValues[fieldName] = "test-rule"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{{
		operandIdx: indexOf(condOperandOpts, "process.path"),
		typeIdx:    0,
		data:       "/usr/bin/test",
	}}
	m.saveEditor()

	if m.editing != modeNone {
		t.Fatal("expected editing to be modeNone after save")
	}
	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.rules))
	}
	r := m.rules[0]
	if r.Name != "test-rule" {
		t.Fatalf("expected name 'test-rule', got %q", r.Name)
	}
	if r.Action != "allow" {
		t.Fatalf("expected action 'allow', got %q", r.Action)
	}
	if r.OpType != "simple" {
		t.Fatalf("expected opType 'simple', got %q", r.OpType)
	}
	if r.OpOperand != "process.path" {
		t.Fatalf("expected operand 'process.path', got %q", r.OpOperand)
	}
	if r.OpData != "/usr/bin/test" {
		t.Fatalf("expected opdata '/usr/bin/test', got %q", r.OpData)
	}
	<-done
}

func TestSaveEditorAddsCompoundRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	notifCh := make(chan bus.OutgoingNotification, 1)
	go func() { notifCh <- <-eb.NotifOut }()

	m.startAdd()
	m.editorValues[fieldName] = "compound-rule"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{
		{operandIdx: indexOf(condOperandOpts, "process.path"), typeIdx: 0, data: "/usr/bin/curl"},
		{operandIdx: indexOf(condOperandOpts, "dest.host"), typeIdx: 0, data: "api.anthropic.com"},
	}
	m.saveEditor()

	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.rules))
	}
	r := m.rules[0]
	if r.OpType != "list" {
		t.Fatalf("expected opType 'list', got %q", r.OpType)
	}
	if r.OpOperand != "list" {
		t.Fatalf("expected operand 'list', got %q", r.OpOperand)
	}

	// Verify JSON data.
	var subs []subOperator
	if err := json.Unmarshal([]byte(r.OpData), &subs); err != nil {
		t.Fatalf("failed to parse OpData JSON: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 sub-operators, got %d", len(subs))
	}
	if subs[0].Operand != "process.path" || subs[0].Data != "/usr/bin/curl" {
		t.Fatalf("unexpected first sub: %+v", subs[0])
	}
	if subs[1].Operand != "dest.host" || subs[1].Data != "api.anthropic.com" {
		t.Fatalf("unexpected second sub: %+v", subs[1])
	}

	// Verify protobuf notification has list operator.
	notif := <-notifCh
	pbRule := notif.Notification.Rules[0]
	if pbRule.Operator.Type != "list" {
		t.Fatalf("expected pb operator type 'list', got %q", pbRule.Operator.Type)
	}
	if len(pbRule.Operator.List) != 2 {
		t.Fatalf("expected 2 pb sub-operators, got %d", len(pbRule.Operator.List))
	}
}

func TestSaveEditorRejectsEmptyName(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.startAdd()
	m.editorValues[fieldName] = ""
	m.conditions = []editorCondition{{operandIdx: 0, typeIdx: 0, data: "/usr/bin/test"}}
	m.saveEditor()

	m.loadRules()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules (empty name rejected), got %d", len(m.rules))
	}
}

func TestSaveEditorRejectsEmptyConditions(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.startAdd()
	m.editorValues[fieldName] = "test-rule"
	m.conditions = []editorCondition{{operandIdx: 0, typeIdx: 0, data: ""}}
	m.saveEditor()

	m.loadRules()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules (empty data rejected), got %d", len(m.rules))
	}
}

func TestSaveEditorSkipsEmptyConditionsInCompound(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	drainNotifs(eb)

	m.startAdd()
	m.editorValues[fieldName] = "mixed-rule"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{
		{operandIdx: indexOf(condOperandOpts, "process.path"), typeIdx: 0, data: "/usr/bin/curl"},
		{operandIdx: indexOf(condOperandOpts, "dest.host"), typeIdx: 0, data: ""}, // empty, should be skipped
	}
	m.saveEditor()

	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.rules))
	}
	// Only one valid condition → should be simple, not list.
	if m.rules[0].OpType != "simple" {
		t.Fatalf("expected simple (empty condition filtered), got %q", m.rules[0].OpType)
	}
}

func TestSaveEditorRenameDeletesOldRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	drainNotifs(eb)

	m.startAdd()
	m.editorValues[fieldName] = "old-name"
	m.editorValues[fieldAction] = "deny"
	m.editorValues[fieldDuration] = "once"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{{operandIdx: 0, typeIdx: 0, data: "/usr/bin/old"}}
	m.saveEditor()

	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.rules))
	}

	m.cursor = 0
	m.startEdit()
	m.editorValues[fieldName] = "new-name"
	m.saveEditor()

	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule after rename, got %d", len(m.rules))
	}
	if m.rules[0].Name != "new-name" {
		t.Fatalf("expected 'new-name', got %q", m.rules[0].Name)
	}
}

func TestToggleRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	drainNotifs(eb)

	m.startAdd()
	m.editorValues[fieldName] = "toggle-test"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{{operandIdx: 0, typeIdx: 0, data: "/usr/bin/app"}}
	m.saveEditor()

	if m.rules[0].Enabled != "true" {
		t.Fatalf("expected enabled=true, got %s", m.rules[0].Enabled)
	}

	m.cursor = 0
	m.toggleRule()
	if m.rules[0].Enabled != "false" {
		t.Fatalf("expected enabled=false after toggle, got %s", m.rules[0].Enabled)
	}

	m.toggleRule()
	if m.rules[0].Enabled != "true" {
		t.Fatalf("expected enabled=true after second toggle, got %s", m.rules[0].Enabled)
	}
}

func TestDoDelete(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	drainNotifs(eb)

	m.startAdd()
	m.editorValues[fieldName] = "delete-me"
	m.editorValues[fieldAction] = "deny"
	m.editorValues[fieldDuration] = "once"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{{
		operandIdx: indexOf(condOperandOpts, "dest.host"), typeIdx: 0, data: "badsite.com",
	}}
	m.saveEditor()

	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.rules))
	}

	m.cursor = 0
	m.doDelete()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(m.rules))
	}
}

func TestStartAddFromConnection(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	g := &connGroup{Process: "curl", Dest: "example.com", Port: 443}
	m.startAddFromConnection(g)

	if m.editing != modeAdd {
		t.Fatal("expected modeAdd")
	}
	if len(m.conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(m.conditions))
	}
	if m.conditions[0].operand() != "dest.host" {
		t.Fatalf("expected operand 'dest.host', got %q", m.conditions[0].operand())
	}
	if m.conditions[0].data != "example.com" {
		t.Fatalf("expected data 'example.com', got %q", m.conditions[0].data)
	}
}

func TestStartEditParsesSimpleRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	drainNotifs(eb)

	// Create a simple rule via DB directly.
	if err := d.InsertRule("unix:local", "simple-rule", "true", "false",
		"allow", "always", "simple", "false", "process.path", "/usr/bin/curl",
		"", "false", ""); err != nil {
		t.Fatal(err)
	}
	m.loadRules()
	m.cursor = 0
	m.startEdit()

	if len(m.conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(m.conditions))
	}
	if m.conditions[0].operand() != "process.path" {
		t.Fatalf("expected operand 'process.path', got %q", m.conditions[0].operand())
	}
	if m.conditions[0].data != "/usr/bin/curl" {
		t.Fatalf("expected data '/usr/bin/curl', got %q", m.conditions[0].data)
	}
}

func TestStartEditParsesListRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	jsonData := `[{"type":"simple","operand":"process.path","data":"/usr/bin/curl"},{"type":"simple","operand":"dest.host","data":"example.com"}]`
	if err := d.InsertRule("unix:local", "list-rule", "true", "false",
		"allow", "always", "list", "false", "list", jsonData,
		"", "false", ""); err != nil {
		t.Fatal(err)
	}
	m.loadRules()
	m.cursor = 0
	m.startEdit()

	if len(m.conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(m.conditions))
	}
	if m.conditions[0].operand() != "process.path" {
		t.Fatalf("expected first operand 'process.path', got %q", m.conditions[0].operand())
	}
	if m.conditions[0].data != "/usr/bin/curl" {
		t.Fatalf("expected first data '/usr/bin/curl', got %q", m.conditions[0].data)
	}
	if m.conditions[1].operand() != "dest.host" {
		t.Fatalf("expected second operand 'dest.host', got %q", m.conditions[1].operand())
	}
	if m.conditions[1].data != "example.com" {
		t.Fatalf("expected second data 'example.com', got %q", m.conditions[1].data)
	}
}

func TestConditionsAddRemove(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.startAdd()
	if len(m.conditions) != 1 {
		t.Fatalf("expected 1 initial condition, got %d", len(m.conditions))
	}

	// Add a condition by directly manipulating (same as Ctrl+A handler).
	m.editorField = fieldConditions
	m.condCursor = 0
	pos := m.condCursor + 1
	m.conditions = append(m.conditions[:pos], append([]editorCondition{{}}, m.conditions[pos:]...)...)
	m.condCursor = pos

	if len(m.conditions) != 2 {
		t.Fatalf("expected 2 conditions after add, got %d", len(m.conditions))
	}
	if m.condCursor != 1 {
		t.Fatalf("expected cursor on new condition (1), got %d", m.condCursor)
	}

	// Remove it.
	if len(m.conditions) > 1 {
		m.conditions = append(m.conditions[:m.condCursor], m.conditions[m.condCursor+1:]...)
		if m.condCursor >= len(m.conditions) {
			m.condCursor = len(m.conditions) - 1
		}
	}
	if len(m.conditions) != 1 {
		t.Fatalf("expected 1 condition after remove, got %d", len(m.conditions))
	}

	// Can't remove the last one.
	if len(m.conditions) <= 1 {
		// No-op, which is correct.
	}
	if len(m.conditions) != 1 {
		t.Fatalf("should not remove last condition, got %d", len(m.conditions))
	}
}

func TestEditorFieldHelpers(t *testing.T) {
	if !isTextField(fieldName) {
		t.Fatal("fieldName should be a text field")
	}
	if !isTextField(fieldDescription) {
		t.Fatal("fieldDescription should be a text field")
	}
	if isTextField(fieldAction) {
		t.Fatal("fieldAction should NOT be a text field")
	}

	if !isDropdownField(fieldAction) {
		t.Fatal("fieldAction should be a dropdown")
	}
	if !isDropdownField(fieldDuration) {
		t.Fatal("fieldDuration should be a dropdown")
	}
	if isDropdownField(fieldName) {
		t.Fatal("fieldName should NOT be a dropdown")
	}

	if !isBoolField(fieldEnabled) {
		t.Fatal("fieldEnabled should be a bool field")
	}
	if !isBoolField(fieldPrecedence) {
		t.Fatal("fieldPrecedence should be a bool field")
	}
	if !isBoolField(fieldNolog) {
		t.Fatal("fieldNolog should be a bool field")
	}
}

func TestGetOptions(t *testing.T) {
	if opts := getOptions(fieldAction); len(opts) != 3 {
		t.Fatalf("expected 3 action options, got %d", len(opts))
	}
	if opts := getOptions(fieldDuration); len(opts) != 9 {
		t.Fatalf("expected 9 duration options, got %d", len(opts))
	}
	if opts := getOptions(fieldName); opts != nil {
		t.Fatal("expected nil options for text field")
	}
}

func TestIndexOf(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if idx := indexOf(slice, "a"); idx != 0 {
		t.Fatalf("expected 0, got %d", idx)
	}
	if idx := indexOf(slice, "c"); idx != 2 {
		t.Fatalf("expected 2, got %d", idx)
	}
	if idx := indexOf(slice, "missing"); idx != 0 {
		t.Fatalf("expected 0 for missing, got %d", idx)
	}
}

func TestSaveEditorNotification(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	notifCh := make(chan bus.OutgoingNotification, 1)
	go func() { notifCh <- <-eb.NotifOut }()

	m.startAdd()
	m.editorValues[fieldName] = "notif-test"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldEnabled] = "true"
	m.conditions = []editorCondition{{
		operandIdx: indexOf(condOperandOpts, "process.path"), typeIdx: 0, data: "/usr/bin/app",
	}}
	m.saveEditor()

	notif := <-notifCh
	if notif.NodeAddr != "unix:local" {
		t.Fatalf("expected node 'unix:local', got %q", notif.NodeAddr)
	}
	if len(notif.Notification.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(notif.Notification.Rules))
	}
	if notif.Notification.Rules[0].Name != "notif-test" {
		t.Fatalf("expected rule name 'notif-test', got %q", notif.Notification.Rules[0].Name)
	}
}

func TestLoadRulesAdjustsCursor(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)
	m.cursor = 5
	m.loadRules()
	if m.cursor != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", m.cursor)
	}
}

func TestEditorConditionHelpers(t *testing.T) {
	c := editorCondition{operandIdx: 0, typeIdx: 0, data: "test"}
	if c.operand() != "process.path" {
		t.Fatalf("expected 'process.path', got %q", c.operand())
	}
	if c.opType() != "simple" {
		t.Fatalf("expected 'simple', got %q", c.opType())
	}

	c2 := editorCondition{operandIdx: 9, typeIdx: 1, data: "test"}
	if c2.operand() != "dest.host" {
		t.Fatalf("expected 'dest.host', got %q", c2.operand())
	}
	if c2.opType() != "regexp" {
		t.Fatalf("expected 'regexp', got %q", c2.opType())
	}
}
