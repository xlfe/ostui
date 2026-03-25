package db

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// ---- Connection tests ----

func TestInsertConnection(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertConnection(
		time.Now().Format(time.RFC3339), "peer:1.2.3.4", "allow", "tcp",
		"127.0.0.1", "12345", "8.8.8.8", "dns.google", "443",
		"1000", "9876", "/usr/bin/curl", "--url https://dns.google", "/tmp", "allow-curl",
	)
	if err != nil {
		t.Fatalf("InsertConnection failed: %v", err)
	}
}

func TestInsertConnectionDuplicate(t *testing.T) {
	d := openTestDB(t)
	args := []string{
		time.Now().Format(time.RFC3339), "peer:1.2.3.4", "allow", "tcp",
		"127.0.0.1", "12345", "8.8.8.8", "dns.google", "443",
		"1000", "9876", "/usr/bin/curl", "--url test", "/tmp", "allow-curl",
	}
	if err := d.InsertConnection(args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14]); err != nil {
		t.Fatal(err)
	}
	// Duplicate insert should be ignored (INSERT OR IGNORE).
	if err := d.InsertConnection(args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14]); err != nil {
		t.Fatal(err)
	}
	rows, err := d.GetConnections(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 connection after duplicate insert, got %d", len(rows))
	}
}

func TestGetConnections(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 5; i++ {
		err := d.InsertConnection(
			time.Now().Add(time.Duration(i)*time.Second).Format(time.RFC3339),
			"peer:1.2.3.4", "allow", "tcp",
			"127.0.0.1", "12345", "8.8.8.8", "dns.google", "443",
			"1000", "9876", "/usr/bin/curl", "arg"+string(rune('0'+i)), "/tmp", "rule",
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	rows, err := d.GetConnections(3)
	if err != nil {
		t.Fatalf("GetConnections failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(rows))
	}
	// Should be newest first.
	if rows[0].Time < rows[1].Time {
		t.Fatal("expected newest connection first")
	}
}

func TestGetConnectionsEmpty(t *testing.T) {
	d := openTestDB(t)
	rows, err := d.GetConnections(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(rows))
	}
}

// ---- Rule tests ----

func TestInsertRule(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertRule(
		"peer:1.2.3.4", "test-rule", "true", "false",
		"allow", "always", "simple", "false",
		"dest.host", "example.com", "test rule", "false",
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("InsertRule failed: %v", err)
	}
}

func TestDeleteRule(t *testing.T) {
	d := openTestDB(t)
	d.InsertRule("peer:1.2.3.4", "delete-me", "true", "false",
		"deny", "once", "simple", "false",
		"dest.host", "bad.com", "", "false", "")

	err := d.DeleteRule("delete-me", "peer:1.2.3.4")
	if err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	rules, err := d.GetRules("peer:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.Name == "delete-me" {
			t.Fatal("rule should have been deleted")
		}
	}
}

func TestGetRules(t *testing.T) {
	d := openTestDB(t)

	// Insert rules for two nodes.
	d.InsertRule("peer:1.2.3.4", "rule1", "true", "false", "allow", "always", "simple", "false", "dest.host", "a.com", "", "false", "")
	d.InsertRule("peer:1.2.3.4", "rule2", "true", "false", "deny", "once", "simple", "false", "dest.host", "b.com", "", "false", "")
	d.InsertRule("peer:5.6.7.8", "rule3", "true", "false", "allow", "always", "simple", "false", "dest.host", "c.com", "", "false", "")

	// Get rules for a specific node.
	rules, err := d.GetRules("peer:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules for peer:1.2.3.4, got %d", len(rules))
	}

	// Get all rules.
	all, err := d.GetRules("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 total rules, got %d", len(all))
	}
}

func TestUpdateRuleEnabled(t *testing.T) {
	d := openTestDB(t)
	d.InsertRule("peer:1.2.3.4", "toggle-rule", "true", "false",
		"allow", "always", "simple", "false",
		"dest.host", "example.com", "", "false", "")

	err := d.UpdateRuleEnabled("toggle-rule", "peer:1.2.3.4", "false")
	if err != nil {
		t.Fatal(err)
	}

	rules, err := d.GetRules("peer:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rules {
		if r.Name == "toggle-rule" {
			found = true
			if r.Enabled != "false" {
				t.Fatalf("expected enabled=false, got %s", r.Enabled)
			}
		}
	}
	if !found {
		t.Fatal("rule not found after update")
	}
}

func TestBulkInsertRules(t *testing.T) {
	d := openTestDB(t)

	rules := []RuleRow{
		{Name: "bulk1", Enabled: "true", Precedence: "false", Action: "allow", Duration: "always", OpType: "simple", OpSensitive: "false", OpOperand: "dest.host", OpData: "a.com", Description: "", Nolog: "false"},
		{Name: "bulk2", Enabled: "true", Precedence: "false", Action: "deny", Duration: "once", OpType: "simple", OpSensitive: "false", OpOperand: "dest.host", OpData: "b.com", Description: "", Nolog: "false"},
		{Name: "bulk3", Enabled: "false", Precedence: "true", Action: "reject", Duration: "5m", OpType: "regexp", OpSensitive: "true", OpOperand: "process.path", OpData: "/usr/.*", Description: "test", Nolog: "true"},
	}

	err := d.BulkInsertRules("peer:1.2.3.4", rules)
	if err != nil {
		t.Fatalf("BulkInsertRules failed: %v", err)
	}

	result, err := d.GetRules("peer:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(result))
	}
}

func TestBulkInsertRulesEmpty(t *testing.T) {
	d := openTestDB(t)
	err := d.BulkInsertRules("peer:1.2.3.4", nil)
	if err != nil {
		t.Fatalf("BulkInsertRules with nil should succeed, got: %v", err)
	}
}

// ---- Rule type tests (adapted from test_ruleseditor.py) ----

func TestRuleTypes(t *testing.T) {
	tests := []struct {
		name      string
		opType    string
		operand   string
		data      string
		action    string
		duration  string
		enabled   string
		sensitive string
	}{
		{"simple-dest-host", "simple", "dest.host", "example.com", "allow", "always", "true", "false"},
		{"simple-dest-ip", "simple", "dest.ip", "8.8.8.8", "deny", "always", "true", "false"},
		{"simple-dest-port", "simple", "dest.port", "443", "allow", "30s", "true", "false"},
		{"simple-process-path", "simple", "process.path", "/usr/bin/curl", "deny", "5m", "true", "false"},
		{"simple-process-command", "simple", "process.command", "--some-argument", "allow", "15m", "true", "false"},
		{"simple-user-id", "simple", "user.id", "1000", "deny", "30m", "true", "false"},
		{"simple-process-id", "simple", "process.id", "1234", "deny", "1h", "true", "false"},
		{"simple-source-port", "simple", "source.port", "12345", "deny", "12h", "true", "false"},
		{"simple-source-ip", "simple", "source.ip", "192.168.1.100", "deny", "until restart", "true", "false"},
		{"simple-protocol", "simple", "protocol", "tcp", "deny", "always", "true", "false"},
		{"regexp-process-path", "regexp", "process.path", "/usr/bin/python.*", "deny", "always", "true", "false"},
		{"regexp-process-command", "regexp", "process.command", "--config=.*", "deny", "always", "true", "false"},
		{"regexp-dest-host", "regexp", "dest.host", ".*\\.example\\.com", "deny", "always", "true", "false"},
		{"network-dest-LAN", "network", "dest.network", "LAN", "deny", "always", "true", "false"},
		{"network-dest-cidr", "network", "dest.network", "192.168.111.0/24", "deny", "always", "true", "false"},
		{"list-type", "list", "list", `[{"type":"simple","operand":"dest.port","data":"443"},{"type":"simple","operand":"dest.host","data":"www.test.com"}]`, "allow", "always", "true", "false"},
	}

	d := openTestDB(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := d.InsertRule(
				"peer:1.2.3.4", tt.name, tt.enabled, "false",
				tt.action, tt.duration, tt.opType, tt.sensitive,
				tt.operand, tt.data, "", "false", "",
			)
			if err != nil {
				t.Fatalf("InsertRule failed: %v", err)
			}

			rules, err := d.GetRules("peer:1.2.3.4")
			if err != nil {
				t.Fatal(err)
			}
			var found *RuleRow
			for i := range rules {
				if rules[i].Name == tt.name {
					found = &rules[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("rule %s not found", tt.name)
			}
			if found.OpType != tt.opType {
				t.Errorf("expected opType %s, got %s", tt.opType, found.OpType)
			}
			if found.OpOperand != tt.operand {
				t.Errorf("expected operand %s, got %s", tt.operand, found.OpOperand)
			}
			if found.OpData != tt.data {
				t.Errorf("expected data %s, got %s", tt.data, found.OpData)
			}
			if found.Action != tt.action {
				t.Errorf("expected action %s, got %s", tt.action, found.Action)
			}
			if found.Duration != tt.duration {
				t.Errorf("expected duration %s, got %s", tt.duration, found.Duration)
			}
		})
	}
}

func TestRuleDurations(t *testing.T) {
	durations := []string{"once", "30s", "5m", "15m", "30m", "1h", "12h", "until restart", "always"}
	d := openTestDB(t)

	for _, dur := range durations {
		t.Run("duration-"+dur, func(t *testing.T) {
			name := "dur-" + dur
			err := d.InsertRule("peer:1.2.3.4", name, "true", "false",
				"allow", dur, "simple", "false",
				"dest.host", "example.com", "", "false", "")
			if err != nil {
				t.Fatal(err)
			}

			rules, err := d.GetRules("peer:1.2.3.4")
			if err != nil {
				t.Fatal(err)
			}
			var found *RuleRow
			for i := range rules {
				if rules[i].Name == name {
					found = &rules[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("rule %s not found", name)
			}
			if found.Duration != dur {
				t.Errorf("expected duration %s, got %s", dur, found.Duration)
			}
		})
	}
}

func TestRuleActions(t *testing.T) {
	actions := []string{"allow", "deny", "reject"}
	d := openTestDB(t)

	for _, action := range actions {
		t.Run("action-"+action, func(t *testing.T) {
			name := "action-" + action
			err := d.InsertRule("peer:1.2.3.4", name, "true", "false",
				action, "always", "simple", "false",
				"dest.host", "example.com", "", "false", "")
			if err != nil {
				t.Fatal(err)
			}

			rules, err := d.GetRules("peer:1.2.3.4")
			if err != nil {
				t.Fatal(err)
			}
			var found *RuleRow
			for i := range rules {
				if rules[i].Name == name {
					found = &rules[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("rule %s not found", name)
			}
			if found.Action != action {
				t.Errorf("expected action %s, got %s", action, found.Action)
			}
		})
	}
}

func TestRulePrecedence(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertRule("peer:1.2.3.4", "prec-rule", "true", "true",
		"allow", "always", "simple", "false",
		"dest.host", "example.com", "", "false", "")
	if err != nil {
		t.Fatal(err)
	}

	rules, _ := d.GetRules("peer:1.2.3.4")
	for _, r := range rules {
		if r.Name == "prec-rule" {
			if r.Precedence != "true" {
				t.Fatalf("expected precedence true, got %s", r.Precedence)
			}
			return
		}
	}
	t.Fatal("rule not found")
}

func TestRuleNolog(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertRule("peer:1.2.3.4", "nolog-rule", "true", "false",
		"allow", "always", "simple", "false",
		"dest.host", "example.com", "", "true", "")
	if err != nil {
		t.Fatal(err)
	}

	rules, _ := d.GetRules("peer:1.2.3.4")
	for _, r := range rules {
		if r.Name == "nolog-rule" {
			if r.Nolog != "true" {
				t.Fatalf("expected nolog true, got %s", r.Nolog)
			}
			return
		}
	}
	t.Fatal("rule not found")
}

func TestRuleEnabledDisabled(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertRule("peer:1.2.3.4", "disabled-rule", "false", "false",
		"deny", "always", "simple", "false",
		"dest.host", "disabled.com", "", "false", "")
	if err != nil {
		t.Fatal(err)
	}

	rules, _ := d.GetRules("peer:1.2.3.4")
	for _, r := range rules {
		if r.Name == "disabled-rule" {
			if r.Enabled != "false" {
				t.Fatalf("expected enabled false, got %s", r.Enabled)
			}
			return
		}
	}
	t.Fatal("rule not found")
}

func TestDuplicateRuleHandling(t *testing.T) {
	d := openTestDB(t)

	// Insert a rule.
	err := d.InsertRule("peer:1.2.3.4", "dup-rule", "true", "false",
		"allow", "always", "simple", "false",
		"dest.host", "first.com", "first desc", "false", "")
	if err != nil {
		t.Fatal(err)
	}

	// Insert a rule with same (node, name) should update via ON CONFLICT.
	err = d.InsertRule("peer:1.2.3.4", "dup-rule", "false", "true",
		"deny", "once", "regexp", "true",
		"dest.host", "second.com", "second desc", "true", "")
	if err != nil {
		t.Fatal(err)
	}

	rules, err := d.GetRules("peer:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range rules {
		if r.Name == "dup-rule" {
			count++
			// Should have the updated values.
			if r.Action != "deny" {
				t.Errorf("expected action deny, got %s", r.Action)
			}
			if r.OpData != "second.com" {
				t.Errorf("expected data second.com, got %s", r.OpData)
			}
			if r.Description != "second desc" {
				t.Errorf("expected description 'second desc', got %s", r.Description)
			}
			if r.Enabled != "false" {
				t.Errorf("expected enabled false, got %s", r.Enabled)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 rule named dup-rule, got %d", count)
	}
}

func TestRuleDescription(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertRule("peer:1.2.3.4", "desc-rule", "true", "false",
		"allow", "always", "simple", "false",
		"dest.host", "example.com", "This is a test rule description", "false", "")
	if err != nil {
		t.Fatal(err)
	}

	rules, _ := d.GetRules("peer:1.2.3.4")
	for _, r := range rules {
		if r.Name == "desc-rule" {
			if r.Description != "This is a test rule description" {
				t.Fatalf("expected description 'This is a test rule description', got '%s'", r.Description)
			}
			return
		}
	}
	t.Fatal("rule not found")
}

func TestRuleSensitiveCase(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertRule("peer:1.2.3.4", "sensitive-rule", "true", "false",
		"allow", "always", "simple", "true",
		"dest.host", "Example.COM", "", "false", "")
	if err != nil {
		t.Fatal(err)
	}

	rules, _ := d.GetRules("peer:1.2.3.4")
	for _, r := range rules {
		if r.Name == "sensitive-rule" {
			if r.OpSensitive != "true" {
				t.Fatalf("expected sensitive true, got %s", r.OpSensitive)
			}
			return
		}
	}
	t.Fatal("rule not found")
}

// ---- Node tests ----

func TestUpsertNode(t *testing.T) {
	d := openTestDB(t)
	err := d.UpsertNode("peer:1.2.3.4", "testhost", "v1.2.3", "3600", "10", "100", "5", "v0.1.0", "online")
	if err != nil {
		t.Fatalf("UpsertNode failed: %v", err)
	}

	nodes, err := d.GetNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Hostname != "testhost" {
		t.Fatalf("expected hostname testhost, got %s", nodes[0].Hostname)
	}
	if nodes[0].Status != "online" {
		t.Fatalf("expected status online, got %s", nodes[0].Status)
	}
}

func TestUpsertNodeUpdate(t *testing.T) {
	d := openTestDB(t)
	d.UpsertNode("peer:1.2.3.4", "host1", "v1.0", "0", "0", "0", "0", "v0.1", "online")
	d.UpsertNode("peer:1.2.3.4", "host2", "v2.0", "1000", "5", "50", "2", "v0.2", "online")

	nodes, err := d.GetNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after upsert, got %d", len(nodes))
	}
	if nodes[0].Hostname != "host2" {
		t.Fatalf("expected hostname host2, got %s", nodes[0].Hostname)
	}
	if nodes[0].DaemonVersion != "v2.0" {
		t.Fatalf("expected version v2.0, got %s", nodes[0].DaemonVersion)
	}
}

func TestSetNodeStatus(t *testing.T) {
	d := openTestDB(t)
	d.UpsertNode("peer:1.2.3.4", "testhost", "v1.0", "0", "0", "0", "0", "v0.1", "online")

	err := d.SetNodeStatus("peer:1.2.3.4", "offline")
	if err != nil {
		t.Fatalf("SetNodeStatus failed: %v", err)
	}

	nodes, _ := d.GetNodes()
	if len(nodes) != 1 {
		t.Fatal("expected 1 node")
	}
	if nodes[0].Status != "offline" {
		t.Fatalf("expected status offline, got %s", nodes[0].Status)
	}
}

func TestGetNodes(t *testing.T) {
	d := openTestDB(t)
	d.UpsertNode("peer:1.2.3.4", "host1", "v1.0", "0", "0", "0", "0", "v0.1", "online")
	d.UpsertNode("peer:5.6.7.8", "host2", "v1.0", "0", "0", "0", "0", "v0.1", "online")
	d.UpsertNode("unix:local", "host3", "v1.0", "0", "0", "0", "0", "v0.1", "offline")

	nodes, err := d.GetNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestGetNodesEmpty(t *testing.T) {
	d := openTestDB(t)
	nodes, err := d.GetNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(nodes))
	}
}

// ---- Alert tests ----

func TestInsertAlert(t *testing.T) {
	d := openTestDB(t)
	err := d.InsertAlert("peer:1.2.3.4", "warning", "alert", "high", "connection", "suspicious connection to 1.2.3.4")
	if err != nil {
		t.Fatalf("InsertAlert failed: %v", err)
	}
}

func TestGetAlerts(t *testing.T) {
	d := openTestDB(t)

	d.InsertAlert("peer:1.2.3.4", "warning", "alert", "high", "connection", "alert1")
	d.InsertAlert("peer:1.2.3.4", "error", "block", "critical", "process", "alert2")
	d.InsertAlert("peer:5.6.7.8", "info", "allow", "low", "rule", "alert3")

	alerts, err := d.GetAlerts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts, got %d", len(alerts))
	}
	// Should be newest first (by rowid DESC).
	if alerts[0].Body != "alert3" {
		t.Fatalf("expected newest alert first (alert3), got %s", alerts[0].Body)
	}

	// Test limit.
	limited, err := d.GetAlerts(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 alerts with limit, got %d", len(limited))
	}
}

func TestGetAlertsEmpty(t *testing.T) {
	d := openTestDB(t)
	alerts, err := d.GetAlerts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(alerts))
	}
}

// ---- Stats tests ----

func TestUpsertStats(t *testing.T) {
	d := openTestDB(t)

	tables := []string{"hosts", "procs", "addrs", "ports", "users"}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			data := map[string]uint64{
				"entry1": 10,
				"entry2": 20,
				"entry3": 5,
			}
			err := d.UpsertStats(table, data)
			if err != nil {
				t.Fatalf("UpsertStats(%s) failed: %v", table, err)
			}

			rows, err := d.GetTopStats(table, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 3 {
				t.Fatalf("expected 3 rows, got %d", len(rows))
			}
		})
	}
}

func TestUpsertStatsUpdate(t *testing.T) {
	d := openTestDB(t)

	d.UpsertStats("hosts", map[string]uint64{"example.com": 10})
	d.UpsertStats("hosts", map[string]uint64{"example.com": 25})

	rows, _ := d.GetTopStats("hosts", 10)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", len(rows))
	}
	if rows[0].Hits != 25 {
		t.Fatalf("expected hits 25, got %d", rows[0].Hits)
	}
}

func TestUpsertStatsEmpty(t *testing.T) {
	d := openTestDB(t)
	err := d.UpsertStats("hosts", nil)
	if err != nil {
		t.Fatalf("UpsertStats with nil should succeed, got: %v", err)
	}
}

func TestUpsertStatsInvalidTable(t *testing.T) {
	d := openTestDB(t)
	err := d.UpsertStats("invalid_table", map[string]uint64{"a": 1})
	if err == nil {
		t.Fatal("expected error for invalid table")
	}
}

func TestGetTopStats(t *testing.T) {
	d := openTestDB(t)

	data := map[string]uint64{
		"a": 100,
		"b": 50,
		"c": 200,
		"d": 10,
		"e": 75,
	}
	d.UpsertStats("hosts", data)

	top3, err := d.GetTopStats("hosts", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(top3) != 3 {
		t.Fatalf("expected 3 top stats, got %d", len(top3))
	}
	// Should be ordered by hits descending.
	if top3[0].Hits < top3[1].Hits || top3[1].Hits < top3[2].Hits {
		t.Fatal("expected descending order by hits")
	}
	if top3[0].What != "c" || top3[0].Hits != 200 {
		t.Fatalf("expected top entry 'c' with 200 hits, got %s with %d", top3[0].What, top3[0].Hits)
	}
}

func TestGetTopStatsInvalidTable(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetTopStats("invalid_table", 10)
	if err == nil {
		t.Fatal("expected error for invalid table")
	}
}

// ---- Purge tests ----

func TestPurgeConnections(t *testing.T) {
	d := openTestDB(t)

	// Insert old connections.
	old := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	d.InsertConnection(old, "peer:1.2.3.4", "allow", "tcp", "127.0.0.1", "111", "8.8.8.8", "dns.google", "443", "1000", "100", "/bin/old", "", "/tmp", "rule1")

	// Insert recent connections.
	recent := time.Now().Format(time.RFC3339)
	d.InsertConnection(recent, "peer:1.2.3.4", "allow", "tcp", "127.0.0.1", "222", "8.8.8.8", "dns.google", "443", "1000", "200", "/bin/new", "", "/tmp", "rule2")

	// Purge connections older than 24 hours.
	deleted, err := d.PurgeConnections(24 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeConnections failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	// Recent connection should still exist.
	rows, _ := d.GetConnections(10)
	if len(rows) != 1 {
		t.Fatalf("expected 1 remaining connection, got %d", len(rows))
	}
}

func TestPurgeConnectionsNothingToDelete(t *testing.T) {
	d := openTestDB(t)

	recent := time.Now().Format(time.RFC3339)
	d.InsertConnection(recent, "peer:1.2.3.4", "allow", "tcp", "127.0.0.1", "333", "8.8.8.8", "dns.google", "443", "1000", "300", "/bin/test", "", "/tmp", "rule")

	deleted, err := d.PurgeConnections(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

// ---- FormatArgs test ----

func TestFormatArgs(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"/bin/cmd", "--flag", "value"}, "/bin/cmd --flag value"},
		{[]string{}, ""},
		{[]string{"single"}, "single"},
	}
	for _, tt := range tests {
		got := FormatArgs(tt.input)
		if got != tt.want {
			t.Errorf("FormatArgs(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
