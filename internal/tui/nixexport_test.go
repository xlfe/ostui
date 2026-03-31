package tui

import (
	"strings"
	"testing"

	"github.com/safedoor/ostui/internal/db"
)

func TestExportRulesToNixEmpty(t *testing.T) {
	out := ExportRulesToNix(nil)
	if !strings.Contains(out, "{\n}\n") {
		t.Fatalf("expected empty attribute set, got:\n%s", out)
	}
}

func TestExportRulesToNixSingleRule(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name:        "systemd-timesyncd",
			Enabled:     "true",
			Action:      "allow",
			Duration:    "always",
			OpType:      "simple",
			OpSensitive: "false",
			OpOperand:   "process.path",
			OpData:      "/usr/lib/systemd/systemd-timesyncd",
			Precedence:  "false",
			Nolog:       "false",
		},
	}

	out := ExportRulesToNix(rules)

	// Verify structure.
	if !strings.Contains(out, "systemd-timesyncd = {") {
		t.Fatalf("missing rule attribute name in output:\n%s", out)
	}
	if !strings.Contains(out, `name = "systemd-timesyncd";`) {
		t.Fatalf("missing name field in output:\n%s", out)
	}
	if !strings.Contains(out, "enabled = true;") {
		t.Fatalf("missing enabled field in output:\n%s", out)
	}
	if !strings.Contains(out, `action = "allow";`) {
		t.Fatalf("missing action field in output:\n%s", out)
	}
	if !strings.Contains(out, `duration = "always";`) {
		t.Fatalf("missing duration field in output:\n%s", out)
	}
	if !strings.Contains(out, "operator = {") {
		t.Fatalf("missing operator block in output:\n%s", out)
	}
	if !strings.Contains(out, `type = "simple";`) {
		t.Fatalf("missing operator.type in output:\n%s", out)
	}
	if !strings.Contains(out, "sensitive = false;") {
		t.Fatalf("missing operator.sensitive in output:\n%s", out)
	}
	if !strings.Contains(out, `operand = "process.path";`) {
		t.Fatalf("missing operator.operand in output:\n%s", out)
	}
	if !strings.Contains(out, `data = "/usr/lib/systemd/systemd-timesyncd";`) {
		t.Fatalf("missing operator.data in output:\n%s", out)
	}
}

func TestExportRulesToNixMultipleRules(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "rule-a", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/bin/a", Precedence: "false", Nolog: "false",
		},
		{
			Name: "rule-b", Enabled: "false", Action: "deny", Duration: "once",
			OpType: "regexp", OpSensitive: "true", OpOperand: "dest.host",
			OpData: ".*\\.evil\\.com", Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	if !strings.Contains(out, "rule-a = {") {
		t.Fatalf("missing rule-a:\n%s", out)
	}
	if !strings.Contains(out, "rule-b = {") {
		t.Fatalf("missing rule-b:\n%s", out)
	}
	if !strings.Contains(out, "enabled = false;") {
		t.Fatalf("rule-b should be disabled:\n%s", out)
	}
	if !strings.Contains(out, `type = "regexp";`) {
		t.Fatalf("rule-b should have regexp type:\n%s", out)
	}
	if !strings.Contains(out, "sensitive = true;") {
		t.Fatalf("rule-b should be sensitive:\n%s", out)
	}
}

func TestExportRulesToNixPrecedenceAndNolog(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "prio-rule", Enabled: "true", Action: "deny", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "dest.host",
			OpData: "tracker.com", Precedence: "true", Nolog: "true",
		},
	}

	out := ExportRulesToNix(rules)

	if !strings.Contains(out, "precedence = true;") {
		t.Fatalf("missing precedence:\n%s", out)
	}
	if !strings.Contains(out, "nolog = true;") {
		t.Fatalf("missing nolog:\n%s", out)
	}
}

func TestExportRulesToNixOmitsFalsePrecedenceAndNolog(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "basic-rule", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/bin/app", Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	if strings.Contains(out, "precedence") {
		t.Fatalf("should omit precedence when false:\n%s", out)
	}
	if strings.Contains(out, "nolog") {
		t.Fatalf("should omit nolog when false:\n%s", out)
	}
}

func TestNixStringEscaping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", `"simple"`},
		{`has "quotes"`, `"has \"quotes\""`},
		{`has\backslash`, `"has\\backslash"`},
		{"has$dollar", `"has\$dollar"`},
		{`${lib.getBin pkgs.foo}`, `"\${lib.getBin pkgs.foo}"`},
		{"", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := nixString(tt.input)
			if got != tt.want {
				t.Errorf("nixString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNixBool(t *testing.T) {
	if nixBool("true") != "true" {
		t.Fatal("nixBool(true) should be true")
	}
	if nixBool("false") != "false" {
		t.Fatal("nixBool(false) should be false")
	}
	if nixBool("") != "false" {
		t.Fatal("nixBool('') should be false")
	}
	if nixBool("anything") != "false" {
		t.Fatal("nixBool(anything) should be false")
	}
}

func TestNixAttrName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with-hyphens", "with-hyphens"},
		{"with_underscores", "with_underscores"},
		{"mixedCase", "mixedCase"},
		{"has spaces", `"has spaces"`},
		{"123starts-with-digit", `"123starts-with-digit"`},
		{"has.dot", `"has.dot"`},
		{"has/slash", `"has/slash"`},
		{"", `""`},
		{"allow-curl-443-12345-1", "allow-curl-443-12345-1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := nixAttrName(tt.input)
			if got != tt.want {
				t.Errorf("nixAttrName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsNixIdent(t *testing.T) {
	valid := []string{"foo", "foo-bar", "foo_bar", "_private", "a1", "it's"}
	for _, s := range valid {
		if !isNixIdent(s) {
			t.Errorf("isNixIdent(%q) should be true", s)
		}
	}

	invalid := []string{"", "1foo", "has space", "has.dot", "has/slash", "-start"}
	for _, s := range invalid {
		if isNixIdent(s) {
			t.Errorf("isNixIdent(%q) should be false", s)
		}
	}
}

func TestExportRulesToNixSpecialCharsInData(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "nix-path-rule", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData:     "${lib.getBin pkgs.systemd}/lib/systemd/systemd-resolved",
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// Dollar-brace should be escaped so Nix doesn't interpolate.
	if !strings.Contains(out, `\${lib.getBin pkgs.systemd}`) {
		t.Fatalf("Nix interpolation should be escaped in data:\n%s", out)
	}
}

func TestExportRulesToNixListOperator(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-claude-anthropic", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "list", OpSensitive: "false", OpOperand: "list",
			OpData:     `[{"type":"simple","operand":"process.path","data":"/usr/bin/claude"},{"type":"simple","operand":"dest.host","data":"api.anthropic.com"}]`,
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	if !strings.Contains(out, `type = "list";`) {
		t.Fatalf("missing list type:\n%s", out)
	}
	if !strings.Contains(out, `operand = "list";`) {
		t.Fatalf("missing list operand:\n%s", out)
	}
	if !strings.Contains(out, "list = [") {
		t.Fatalf("missing list attribute:\n%s", out)
	}
	// Should NOT contain raw JSON data field.
	if strings.Contains(out, `"data" = "["`) || strings.Contains(out, `data = "[`) {
		t.Fatalf("list operator should not have raw JSON data field:\n%s", out)
	}
	// Should contain expanded sub-operators.
	if !strings.Contains(out, `operand = "process.path";`) {
		t.Fatalf("missing process.path sub-operator:\n%s", out)
	}
	if !strings.Contains(out, `data = "/usr/bin/claude";`) {
		t.Fatalf("missing claude data:\n%s", out)
	}
	if !strings.Contains(out, `operand = "dest.host";`) {
		t.Fatalf("missing dest.host sub-operator:\n%s", out)
	}
	if !strings.Contains(out, `data = "api.anthropic.com";`) {
		t.Fatalf("missing anthropic data:\n%s", out)
	}
}

func TestExportRulesToNixListOperatorThreeConditions(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-curl-https-google", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "list", OpSensitive: "false", OpOperand: "list",
			OpData: `[{"type":"simple","operand":"process.path","data":"/usr/bin/curl"},{"type":"simple","operand":"dest.host","data":"www.google.com"},{"type":"simple","operand":"dest.port","data":"443"}]`,
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// Should have three sub-operators.
	if strings.Count(out, `type = "simple";`) != 3 {
		t.Fatalf("expected 3 simple sub-operators:\n%s", out)
	}
	if !strings.Contains(out, `data = "/usr/bin/curl";`) {
		t.Fatalf("missing curl:\n%s", out)
	}
	if !strings.Contains(out, `data = "www.google.com";`) {
		t.Fatalf("missing google:\n%s", out)
	}
	if !strings.Contains(out, `data = "443";`) {
		t.Fatalf("missing port 443:\n%s", out)
	}
}

func TestExportRulesToNixListOperatorInvalidJSON(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "bad-list", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "list", OpSensitive: "false", OpOperand: "list",
			OpData:     "not valid json",
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// Should fall back to raw data field.
	if !strings.Contains(out, `data = "not valid json";`) {
		t.Fatalf("invalid JSON should fall back to raw data:\n%s", out)
	}
}

func TestExportRulesToNixListOperatorSensitiveSubOps(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "sensitive-list", Enabled: "true", Action: "deny", Duration: "always",
			OpType: "list", OpSensitive: "false", OpOperand: "list",
			OpData:     `[{"type":"simple","operand":"process.path","data":"/bin/evil","sensitive":true}]`,
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// The sub-operator should have sensitive = true.
	// Find the sub-operator block and verify it has sensitive = true
	if !strings.Contains(out, "sensitive = true;") {
		t.Fatalf("missing sensitive sub-operator:\n%s", out)
	}
}

func TestExportRulesToNixBalancedBrackets(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "simple-rule", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/bin/a", Precedence: "false", Nolog: "false",
		},
		{
			Name: "list-rule", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "list", OpSensitive: "false", OpOperand: "list",
			OpData:     `[{"type":"simple","operand":"process.path","data":"/bin/b"},{"type":"simple","operand":"dest.host","data":"example.com"}]`,
			Precedence: "true", Nolog: "true",
		},
	}

	out := ExportRulesToNix(rules)

	opens := strings.Count(out, "{")
	closes := strings.Count(out, "}")
	if opens != closes {
		t.Fatalf("unbalanced braces: %d opens, %d closes\n%s", opens, closes, out)
	}

	openBrackets := strings.Count(out, "[")
	closeBrackets := strings.Count(out, "]")
	if openBrackets != closeBrackets {
		t.Fatalf("unbalanced brackets: %d opens, %d closes\n%s", openBrackets, closeBrackets, out)
	}
}

func TestCleanNixAttrName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Rule name with nix hash gets cleaned.
		{"allow-always-simple-nix-store-2md38jpiicw64pngxqifiw8casjvmk0h-google-chrome", "allow-always-simple-nix-store-google-chrome"},
		// Short name without hash stays the same.
		{"allow-curl-443-12345-1", "allow-curl-443-12345-1"},
		// Hash-only prefix in name.
		{"allow-fnfajsmy8pyz1slb02chfpw2fpd5f8hn-claude-443", "allow-claude-443"},
		// No hash at all.
		{"systemd-resolved", "systemd-resolved"},
		// Empty string.
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanNixAttrName(tt.input)
			if got != tt.want {
				t.Errorf("cleanNixAttrName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExportNixCleanedAttrKeyAndNameValue(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-fnfajsmy8pyz1slb02chfpw2fpd5f8hn-claude-443", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/nix/store/fnfajsmy8pyz1slb02chfpw2fpd5f8hn-claude", Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// Attribute key should be cleaned (hash removed).
	if !strings.Contains(out, "allow-claude-443 = {") {
		t.Fatalf("attribute key should have hash stripped:\n%s", out)
	}
	// name value should also be cleaned (hash removed), matching the attribute key.
	if !strings.Contains(out, `name = "allow-claude-443";`) {
		t.Fatalf("name value should have hash stripped:\n%s", out)
	}
}

func TestExportNixDeduplicatesCleanedNames(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1-ssh-22", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/usr/bin/ssh", Precedence: "false", Nolog: "false",
		},
		{
			Name: "allow-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-ssh-22", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/usr/bin/ssh", Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// First should keep the clean name, second should get a suffix.
	if !strings.Contains(out, "allow-ssh-22 = {") {
		t.Fatalf("first rule should have clean name:\n%s", out)
	}
	if !strings.Contains(out, "allow-ssh-22-2 = {") {
		t.Fatalf("second rule should have deduplicated name:\n%s", out)
	}
}

func TestExportNixProcessComment(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-chrome-443", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData:     "/nix/store/2md38jpiicw64pngxqifiw8casjvmk0h-google-chrome-146.0.7680.153/share/google/chrome/chrome",
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	if !strings.Contains(out, "  # chrome\n") {
		t.Fatalf("expected basename comment for nix store path:\n%s", out)
	}
}

func TestExportNixNoCommentForNonNixPath(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-curl", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/usr/bin/curl", Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	if strings.Contains(out, "  #") {
		t.Fatalf("should not add comment for non-nix path:\n%s", out)
	}
}

func TestExportNixNoCommentForNonProcessOperand(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "allow-host", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "dest.host",
			OpData: "example.com", Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	if strings.Contains(out, "  #") {
		t.Fatalf("should not add comment for non-process operand:\n%s", out)
	}
}

func TestExportRulesToNixIsValidNixSyntax(t *testing.T) {
	rules := []db.RuleRow{
		{
			Name: "test-rule", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "simple", OpSensitive: "false", OpOperand: "process.path",
			OpData: "/bin/test", Precedence: "false", Nolog: "false",
		},
		{
			Name: "list-rule", Enabled: "true", Action: "allow", Duration: "always",
			OpType: "list", OpSensitive: "false", OpOperand: "list",
			OpData:     `[{"type":"simple","operand":"process.path","data":"/bin/x"},{"type":"simple","operand":"dest.host","data":"x.com"}]`,
			Precedence: "false", Nolog: "false",
		},
	}

	out := ExportRulesToNix(rules)

	// Count braces — they should be balanced.
	opens := strings.Count(out, "{")
	closes := strings.Count(out, "}")
	if opens != closes {
		t.Fatalf("unbalanced braces: %d opens, %d closes\n%s", opens, closes, out)
	}

	// Count brackets — they should be balanced.
	openBrackets := strings.Count(out, "[")
	closeBrackets := strings.Count(out, "]")
	if openBrackets != closeBrackets {
		t.Fatalf("unbalanced brackets: %d opens, %d closes\n%s", openBrackets, closeBrackets, out)
	}

	// Every attribute line should end with a semicolon.
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Structural lines that don't need semicolons.
		if trimmed == "{" || trimmed == "}" {
			continue
		}
		// Lines that open a block (end with " = {" or " = [").
		if strings.HasSuffix(trimmed, "= {") || strings.HasSuffix(trimmed, "= [") {
			continue
		}
		// Closing braces/brackets with semicolons.
		if trimmed == "};" || trimmed == "];" {
			continue
		}
		if !strings.HasSuffix(trimmed, ";") {
			t.Errorf("line missing semicolon: %q", trimmed)
		}
	}
}
