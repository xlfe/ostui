package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// InsertConnection inserts a connection event, ignoring duplicates.
func (d *DB) InsertConnection(t, node, action, protocol, srcIP, srcPort, dstIP, dstHost, dstPort, uid, pid, process, processArgs, processCwd, rule string) error {
	_, err := d.conn.Exec(
		`INSERT OR IGNORE INTO connections (time, node, action, protocol, src_ip, src_port, dst_ip, dst_host, dst_port, uid, pid, process, process_args, process_cwd, rule)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t, node, action, protocol, srcIP, srcPort, dstIP, dstHost, dstPort, uid, pid, process, processArgs, processCwd, rule,
	)
	return err
}

// GetConnections returns recent connections, newest first.
func (d *DB) GetConnections(limit int) ([]ConnectionRow, error) {
	rows, err := d.conn.Query(
		`SELECT time, node, action, protocol, src_ip, src_port, dst_ip, dst_host, dst_port, uid, pid, process, process_args, process_cwd, rule
		 FROM connections ORDER BY time DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ConnectionRow
	for rows.Next() {
		var r ConnectionRow
		if err := rows.Scan(&r.Time, &r.Node, &r.Action, &r.Protocol, &r.SrcIP, &r.SrcPort,
			&r.DstIP, &r.DstHost, &r.DstPort, &r.UID, &r.PID, &r.Process, &r.ProcessArgs, &r.ProcessCwd, &r.Rule); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type ConnectionRow struct {
	Time, Node, Action, Protocol          string
	SrcIP, SrcPort, DstIP, DstHost, DstPort string
	UID, PID, Process, ProcessArgs, ProcessCwd, Rule string
}

// UpsertNode inserts or updates a node record.
func (d *DB) UpsertNode(addr, hostname, daemonVersion, uptime, rules, cons, consDropped, version, status string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := d.conn.Exec(
		`INSERT INTO nodes (addr, hostname, daemon_version, daemon_uptime, daemon_rules, cons, cons_dropped, version, status, last_connection)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(addr) DO UPDATE SET
			hostname=excluded.hostname,
			daemon_version=excluded.daemon_version,
			daemon_uptime=excluded.daemon_uptime,
			daemon_rules=excluded.daemon_rules,
			cons=excluded.cons,
			cons_dropped=excluded.cons_dropped,
			version=excluded.version,
			status=excluded.status,
			last_connection=excluded.last_connection`,
		addr, hostname, daemonVersion, uptime, rules, cons, consDropped, version, status, now,
	)
	return err
}

// SetNodeStatus updates the status of a node.
func (d *DB) SetNodeStatus(addr, status string) error {
	_, err := d.conn.Exec(`UPDATE nodes SET status=? WHERE addr=?`, status, addr)
	return err
}

// DeleteNode removes a node and all its rules from the database.
func (d *DB) DeleteNode(addr string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM rules WHERE node=?`, addr); err != nil {
		return fmt.Errorf("delete rules for node %s: %w", addr, err)
	}
	if _, err := tx.Exec(`DELETE FROM connections WHERE node=?`, addr); err != nil {
		return fmt.Errorf("delete connections for node %s: %w", addr, err)
	}
	if _, err := tx.Exec(`DELETE FROM nodes WHERE addr=?`, addr); err != nil {
		return fmt.Errorf("delete node %s: %w", addr, err)
	}
	return tx.Commit()
}

// GetNodes returns all known nodes.
func (d *DB) GetNodes() ([]NodeRow, error) {
	rows, err := d.conn.Query(`SELECT addr, hostname, daemon_version, daemon_uptime, daemon_rules, cons, cons_dropped, version, status, last_connection FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NodeRow
	for rows.Next() {
		var r NodeRow
		if err := rows.Scan(&r.Addr, &r.Hostname, &r.DaemonVersion, &r.DaemonUptime, &r.DaemonRules, &r.Cons, &r.ConsDropped, &r.Version, &r.Status, &r.LastConnection); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type NodeRow struct {
	Addr, Hostname, DaemonVersion, DaemonUptime, DaemonRules string
	Cons, ConsDropped, Version, Status, LastConnection       string
}

// InsertRule inserts or replaces a rule for a node.
func (d *DB) InsertRule(node, name, enabled, precedence, action, duration, opType, opSensitive, opOperand, opData, description, nolog, created string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := d.conn.Exec(
		`INSERT INTO rules (time, node, name, enabled, precedence, action, duration, operator_type, operator_sensitive, operator_operand, operator_data, description, nolog, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(node, name) DO UPDATE SET
			time=excluded.time,
			enabled=excluded.enabled,
			precedence=excluded.precedence,
			action=excluded.action,
			duration=excluded.duration,
			operator_type=excluded.operator_type,
			operator_sensitive=excluded.operator_sensitive,
			operator_operand=excluded.operator_operand,
			operator_data=excluded.operator_data,
			description=excluded.description,
			nolog=excluded.nolog`,
		now, node, name, enabled, precedence, action, duration, opType, opSensitive, opOperand, opData, description, nolog, created,
	)
	return err
}

// DeleteRule removes a rule by name and node.
func (d *DB) DeleteRule(name, node string) error {
	_, err := d.conn.Exec(`DELETE FROM rules WHERE name=? AND node=?`, name, node)
	return err
}

// GetRules returns all rules for a node (or all nodes if node is empty).
func (d *DB) GetRules(node string) ([]RuleRow, error) {
	var rows *sql.Rows
	var err error
	if node == "" {
		rows, err = d.conn.Query(`SELECT time, node, name, enabled, precedence, action, duration, operator_type, operator_sensitive, operator_operand, operator_data, description, nolog, created FROM rules ORDER BY name`)
	} else {
		rows, err = d.conn.Query(`SELECT time, node, name, enabled, precedence, action, duration, operator_type, operator_sensitive, operator_operand, operator_data, description, nolog, created FROM rules WHERE node=? ORDER BY name`, node)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RuleRow
	for rows.Next() {
		var r RuleRow
		if err := rows.Scan(&r.Time, &r.Node, &r.Name, &r.Enabled, &r.Precedence, &r.Action, &r.Duration, &r.OpType, &r.OpSensitive, &r.OpOperand, &r.OpData, &r.Description, &r.Nolog, &r.Created); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type RuleRow struct {
	Time, Node, Name, Enabled, Precedence, Action, Duration string
	OpType, OpSensitive, OpOperand, OpData                  string
	Description, Nolog, Created                              string
}

// InsertAlert inserts an alert record.
func (d *DB) InsertAlert(node, alertType, action, priority, what, body string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := d.conn.Exec(
		`INSERT INTO alerts (time, node, type, action, priority, what, body, status) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		now, node, alertType, action, priority, what, body,
	)
	return err
}

// GetAlerts returns recent alerts.
func (d *DB) GetAlerts(limit int) ([]AlertRow, error) {
	rows, err := d.conn.Query(`SELECT time, node, type, action, priority, what, body, status FROM alerts ORDER BY rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AlertRow
	for rows.Next() {
		var r AlertRow
		if err := rows.Scan(&r.Time, &r.Node, &r.Type, &r.Action, &r.Priority, &r.What, &r.Body, &r.Status); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type AlertRow struct {
	Time, Node, Type, Action, Priority, What, Body string
	Status                                          int
}

// UpsertStats batch upserts hit counts for a stats table (hosts, procs, addrs, ports, users).
func (d *DB) UpsertStats(table string, data map[string]uint64) error {
	if len(data) == 0 {
		return nil
	}
	// Validate table name (prevent SQL injection).
	switch table {
	case "hosts", "procs", "addrs", "ports", "users":
	default:
		return fmt.Errorf("invalid stats table: %s", table)
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(fmt.Sprintf(`INSERT INTO %s (what, hits) VALUES (?, ?) ON CONFLICT(what) DO UPDATE SET hits=excluded.hits`, table))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for what, hits := range data {
		if _, err := stmt.Exec(what, hits); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetTopStats returns the top-N entries from a stats table ordered by hits descending.
func (d *DB) GetTopStats(table string, limit int) ([]StatsRow, error) {
	switch table {
	case "hosts", "procs", "addrs", "ports", "users":
	default:
		return nil, fmt.Errorf("invalid stats table: %s", table)
	}

	rows, err := d.conn.Query(fmt.Sprintf(`SELECT what, hits FROM %s ORDER BY hits DESC LIMIT ?`, table), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []StatsRow
	for rows.Next() {
		var r StatsRow
		if err := rows.Scan(&r.What, &r.Hits); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

type StatsRow struct {
	What string
	Hits  int64
}

// PurgeConnections deletes connections older than the given duration.
func (d *DB) PurgeConnections(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339)
	res, err := d.conn.Exec(`DELETE FROM connections WHERE time < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpdateRuleEnabled sets the enabled field of a rule.
func (d *DB) UpdateRuleEnabled(name, node, enabled string) error {
	_, err := d.conn.Exec(`UPDATE rules SET enabled=? WHERE name=? AND node=?`, enabled, name, node)
	return err
}

// BulkInsertRules inserts multiple rules in a transaction.
func (d *DB) BulkInsertRules(node string, rules []RuleRow) error {
	if len(rules) == 0 {
		return nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR REPLACE INTO rules (time, node, name, enabled, precedence, action, duration, operator_type, operator_sensitive, operator_operand, operator_data, description, nolog, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Format(time.RFC3339)
	for _, r := range rules {
		created := r.Created
		if created == "" {
			created = now
		}
		if _, err := stmt.Exec(now, node, r.Name, r.Enabled, r.Precedence, r.Action, r.Duration, r.OpType, r.OpSensitive, r.OpOperand, r.OpData, r.Description, r.Nolog, created); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FormatArgs joins string slices for storage.
func FormatArgs(args []string) string {
	return strings.Join(args, " ")
}
