package tui

import (
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

func TestRulesModelLoadEmpty(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.loadRules()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(m.rules))
	}
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestSaveEditorAddsRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	// Drain NotifOut in background to prevent blocking.
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-eb.NotifOut
	}()

	m.startAdd()
	m.editorValues[fieldName] = "test-rule"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldOpType] = "simple"
	m.editorValues[fieldOpOperand] = "process.path"
	m.editorValues[fieldOpData] = "/usr/bin/test"
	m.editorValues[fieldEnabled] = "true"
	m.editorValues[fieldPrecedence] = "false"
	m.editorValues[fieldNolog] = "false"
	m.editorValues[fieldDescription] = "test description"

	m.saveEditor()

	if m.editing != modeNone {
		t.Fatal("expected editing to be modeNone after save")
	}
	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule after save, got %d", len(m.rules))
	}
	r := m.rules[0]
	if r.Name != "test-rule" {
		t.Fatalf("expected name 'test-rule', got %q", r.Name)
	}
	if r.Action != "allow" {
		t.Fatalf("expected action 'allow', got %q", r.Action)
	}
	if r.OpData != "/usr/bin/test" {
		t.Fatalf("expected opdata '/usr/bin/test', got %q", r.OpData)
	}
	if r.Node != "unix:local" {
		t.Fatalf("expected node 'unix:local', got %q", r.Node)
	}

	<-done
}

func TestSaveEditorRejectsEmptyName(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.startAdd()
	m.editorValues[fieldName] = ""
	m.editorValues[fieldOpData] = "/usr/bin/test"

	m.saveEditor()

	// Should still be in editing mode since validation failed.
	m.loadRules()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules (empty name should be rejected), got %d", len(m.rules))
	}
}

func TestSaveEditorRejectsEmptyData(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.startAdd()
	m.editorValues[fieldName] = "test-rule"
	m.editorValues[fieldOpData] = ""

	m.saveEditor()

	m.loadRules()
	if len(m.rules) != 0 {
		t.Fatalf("expected 0 rules (empty data should be rejected), got %d", len(m.rules))
	}
}

func TestSaveEditorRenameDeletesOldRule(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	// Drain NotifOut in background.
	go func() {
		for range eb.NotifOut {
		}
	}()

	// Create initial rule.
	m.startAdd()
	m.editorValues[fieldName] = "old-name"
	m.editorValues[fieldAction] = "deny"
	m.editorValues[fieldDuration] = "once"
	m.editorValues[fieldOpType] = "simple"
	m.editorValues[fieldOpOperand] = "process.path"
	m.editorValues[fieldOpData] = "/usr/bin/old"
	m.editorValues[fieldEnabled] = "true"
	m.editorValues[fieldPrecedence] = "false"
	m.editorValues[fieldNolog] = "false"
	m.saveEditor()

	if len(m.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(m.rules))
	}

	// Edit and rename.
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

	// Drain NotifOut.
	go func() {
		for range eb.NotifOut {
		}
	}()

	// Create a rule.
	m.startAdd()
	m.editorValues[fieldName] = "toggle-test"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldOpType] = "simple"
	m.editorValues[fieldOpOperand] = "process.path"
	m.editorValues[fieldOpData] = "/usr/bin/app"
	m.editorValues[fieldEnabled] = "true"
	m.editorValues[fieldPrecedence] = "false"
	m.editorValues[fieldNolog] = "false"
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

	// Drain NotifOut.
	go func() {
		for range eb.NotifOut {
		}
	}()

	// Create a rule.
	m.startAdd()
	m.editorValues[fieldName] = "delete-me"
	m.editorValues[fieldAction] = "deny"
	m.editorValues[fieldDuration] = "once"
	m.editorValues[fieldOpType] = "simple"
	m.editorValues[fieldOpOperand] = "dest.host"
	m.editorValues[fieldOpData] = "badsite.com"
	m.editorValues[fieldEnabled] = "true"
	m.editorValues[fieldPrecedence] = "false"
	m.editorValues[fieldNolog] = "false"
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

	g := &connGroup{
		Process: "curl",
		Dest:    "example.com",
		Port:    443,
	}
	m.startAddFromConnection(g)

	if m.editing != modeAdd {
		t.Fatal("expected editing to be modeAdd")
	}
	if m.editorValues[fieldAction] != "allow" {
		t.Fatalf("expected action 'allow', got %q", m.editorValues[fieldAction])
	}
	if m.editorValues[fieldOpOperand] != "dest.host" {
		t.Fatalf("expected operand 'dest.host', got %q", m.editorValues[fieldOpOperand])
	}
	if m.editorValues[fieldOpData] != "example.com" {
		t.Fatalf("expected opdata 'example.com', got %q", m.editorValues[fieldOpData])
	}
}

func TestSaveEditorSendsChangeRuleNotification(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	m.startAdd()
	m.editorValues[fieldName] = "notif-test"
	m.editorValues[fieldAction] = "allow"
	m.editorValues[fieldDuration] = "always"
	m.editorValues[fieldOpType] = "simple"
	m.editorValues[fieldOpOperand] = "process.path"
	m.editorValues[fieldOpData] = "/usr/bin/app"
	m.editorValues[fieldEnabled] = "true"
	m.editorValues[fieldPrecedence] = "false"
	m.editorValues[fieldNolog] = "false"

	// Listen for notification.
	notifCh := make(chan bus.OutgoingNotification, 1)
	go func() {
		notif := <-eb.NotifOut
		notifCh <- notif
	}()

	m.saveEditor()

	notif := <-notifCh
	if notif.NodeAddr != "unix:local" {
		t.Fatalf("expected node 'unix:local', got %q", notif.NodeAddr)
	}
	if len(notif.Notification.Rules) != 1 {
		t.Fatalf("expected 1 rule in notification, got %d", len(notif.Notification.Rules))
	}
	if notif.Notification.Rules[0].Name != "notif-test" {
		t.Fatalf("expected rule name 'notif-test', got %q", notif.Notification.Rules[0].Name)
	}
}

func TestLoadRulesAdjustsCursorDown(t *testing.T) {
	d := openTestDB(t)
	eb := testEventBus()
	m := newRulesModel(d, eb)

	// Set cursor beyond range.
	m.cursor = 5
	m.loadRules()

	if m.cursor != 0 {
		t.Fatalf("expected cursor clamped to 0 on empty list, got %d", m.cursor)
	}
}

func TestEditorFieldHelpers(t *testing.T) {
	// Text fields.
	if !isTextField(fieldName) {
		t.Fatal("fieldName should be a text field")
	}
	if !isTextField(fieldOpData) {
		t.Fatal("fieldOpData should be a text field")
	}
	if !isTextField(fieldDescription) {
		t.Fatal("fieldDescription should be a text field")
	}
	if isTextField(fieldAction) {
		t.Fatal("fieldAction should NOT be a text field")
	}

	// Dropdown fields.
	if !isDropdownField(fieldAction) {
		t.Fatal("fieldAction should be a dropdown")
	}
	if !isDropdownField(fieldDuration) {
		t.Fatal("fieldDuration should be a dropdown")
	}
	if isDropdownField(fieldName) {
		t.Fatal("fieldName should NOT be a dropdown")
	}

	// Bool fields.
	if !isBoolField(fieldEnabled) {
		t.Fatal("fieldEnabled should be a bool field")
	}
	if !isBoolField(fieldPrecedence) {
		t.Fatal("fieldPrecedence should be a bool field")
	}
	if !isBoolField(fieldNolog) {
		t.Fatal("fieldNolog should be a bool field")
	}
	if isBoolField(fieldName) {
		t.Fatal("fieldName should NOT be a bool field")
	}
}

func TestGetOptions(t *testing.T) {
	if opts := getOptions(fieldAction); len(opts) != 3 {
		t.Fatalf("expected 3 action options, got %d", len(opts))
	}
	if opts := getOptions(fieldDuration); len(opts) != 9 {
		t.Fatalf("expected 9 duration options, got %d", len(opts))
	}
	if opts := getOptions(fieldOpType); len(opts) != 6 {
		t.Fatalf("expected 6 opType options, got %d", len(opts))
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
