package tui

import "fmt"

// Common port names. Not exhaustive — just the ones you'd actually want labeled.
var portNames = map[uint32]string{
	20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp",
	53: "dns", 67: "dhcp", 68: "dhcp", 80: "http", 110: "pop3",
	119: "nntp", 123: "ntp", 143: "imap", 161: "snmp", 162: "snmp-trap",
	389: "ldap", 443: "https", 445: "smb", 465: "smtps", 514: "syslog",
	587: "submission", 636: "ldaps", 853: "dot", 993: "imaps", 995: "pop3s",
	1080: "socks", 1194: "openvpn", 1433: "mssql", 1521: "oracle",
	2049: "nfs", 3306: "mysql", 3389: "rdp", 5432: "postgres",
	5900: "vnc", 6379: "redis", 6443: "k8s-api", 8080: "http-alt",
	8443: "https-alt", 8888: "http-alt", 9090: "prometheus", 9200: "elasticsearch",
	9418: "git", 11211: "memcached", 27017: "mongodb",
}

// formatPort returns "name (port)" for known ports, or just the port number.
func formatPort(port uint32) string {
	if name, ok := portNames[port]; ok {
		return fmt.Sprintf("%s(%d)", name, port)
	}
	return fmt.Sprintf("%d", port)
}

// formatPortStr does the same for string port keys (from stats maps).
func formatPortStr(portStr string) string {
	var port uint32
	if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
		if name, ok := portNames[port]; ok {
			return fmt.Sprintf("%s(%d)", name, port)
		}
	}
	return portStr
}
