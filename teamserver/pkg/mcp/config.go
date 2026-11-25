package mcp

// Config holds the configuration for the MCP server.
type Config struct {
	// ReadOnly disables all tasking tools
	ReadOnly bool
	// AllowedTools is a list of tool names that are allowed (empty = all)
	AllowedTools []string
	// AuditLog is the audit log destination ("stderr" or file path)
	AuditLog string
	// MaxOutputBytes is the maximum output bytes per call
	MaxOutputBytes int
	// LootRoot is the teamserver loot directory containing run artifacts
	LootRoot string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxOutputBytes: 262144,
		LootRoot:       "data/loot",
	}
}
