package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

const schemaVersion = 3

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	dsn := path + "?_journal_mode=wal&_busy_timeout=5000&_synchronous=normal"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)

	d := &DB{conn: conn}
	if err := d.createTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}

func (d *DB) createTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS connections (
			time TEXT,
			node TEXT,
			action TEXT,
			protocol TEXT,
			src_ip TEXT,
			src_port TEXT,
			dst_ip TEXT,
			dst_host TEXT,
			dst_port TEXT,
			uid TEXT,
			pid TEXT,
			process TEXT,
			process_args TEXT,
			process_cwd TEXT,
			rule TEXT,
			UNIQUE(node, action, protocol, src_ip, src_port, dst_ip, dst_port, uid, pid, process, process_args)
		)`,
		`CREATE INDEX IF NOT EXISTS time_index ON connections (time)`,
		`CREATE INDEX IF NOT EXISTS action_index ON connections (action)`,
		`CREATE INDEX IF NOT EXISTS protocol_index ON connections (protocol)`,
		`CREATE INDEX IF NOT EXISTS dst_host_index ON connections (dst_host)`,
		`CREATE INDEX IF NOT EXISTS process_index ON connections (process)`,
		`CREATE INDEX IF NOT EXISTS dst_ip_index ON connections (dst_ip)`,
		`CREATE INDEX IF NOT EXISTS dst_port_index ON connections (dst_port)`,
		`CREATE INDEX IF NOT EXISTS rule_index ON connections (rule)`,
		`CREATE INDEX IF NOT EXISTS node_index ON connections (node)`,
		`CREATE INDEX IF NOT EXISTS details_query_index ON connections (process, process_args, uid, pid, dst_ip, dst_host, dst_port, action, node, protocol)`,

		`CREATE TABLE IF NOT EXISTS nodes (
			addr TEXT PRIMARY KEY,
			hostname TEXT,
			daemon_version TEXT,
			daemon_uptime TEXT,
			daemon_rules TEXT,
			cons TEXT,
			cons_dropped TEXT,
			version TEXT,
			status TEXT,
			last_connection TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS rules (
			time TEXT,
			node TEXT,
			name TEXT,
			enabled TEXT,
			precedence TEXT,
			action TEXT,
			duration TEXT,
			operator_type TEXT,
			operator_sensitive TEXT,
			operator_operand TEXT,
			operator_data TEXT,
			description TEXT,
			nolog TEXT,
			created TEXT,
			UNIQUE(node, name)
		)`,
		`CREATE INDEX IF NOT EXISTS rules_index ON rules (time)`,

		`CREATE TABLE IF NOT EXISTS alerts (
			time TEXT,
			node TEXT,
			type TEXT,
			action TEXT,
			priority TEXT,
			what TEXT,
			body TEXT,
			status INT
		)`,

		`CREATE TABLE IF NOT EXISTS hosts (what TEXT PRIMARY KEY, hits INTEGER)`,
		`CREATE TABLE IF NOT EXISTS procs (what TEXT PRIMARY KEY, hits INTEGER)`,
		`CREATE TABLE IF NOT EXISTS addrs (what TEXT PRIMARY KEY, hits INTEGER)`,
		`CREATE TABLE IF NOT EXISTS ports (what TEXT PRIMARY KEY, hits INTEGER)`,
		`CREATE TABLE IF NOT EXISTS users (what TEXT PRIMARY KEY, hits INTEGER)`,
	}

	for _, s := range stmts {
		if _, err := d.conn.Exec(s); err != nil {
			log.Printf("db schema error: %s: %v", s[:40], err)
			return err
		}
	}
	return nil
}
