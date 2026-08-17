package auditlogs

// Collector receives MCP audit entries.
type Collector interface {
	CollectMCPAuditEntry(entry MCPAuditLog)
	Close()
}
