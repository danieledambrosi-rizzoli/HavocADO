package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"Havoc/pkg/logger"
	"Havoc/pkg/mcp"
	"Havoc/pkg/operator"

	"github.com/spf13/cobra"
)

var CobraMcp = &cobra.Command{
	Use:   "mcp",
	Short: "start the Havoc MCP stdio server",
	RunE:  mcpFunc,
}

type McpFlags struct {
	Teamserver     string
	User           string
	Password       string
	TLSCA          string
	Insecure       bool
	ReadOnly       bool
	AllowTools     string
	AuditLog       string
	MaxOutputBytes int
	NoReconnect    bool
}

var mcpFlags McpFlags

func init() {
	CobraMcp.Flags().SortFlags = false

	CobraMcp.Flags().StringVar(&mcpFlags.Teamserver, "teamserver", "wss://127.0.0.1:40056", "teamserver websocket URL")
	CobraMcp.Flags().StringVar(&mcpFlags.User, "user", "mcp", "operator username")
	CobraMcp.Flags().StringVar(&mcpFlags.Password, "password", "", "operator password (required)")
	CobraMcp.Flags().StringVar(&mcpFlags.TLSCA, "tls-ca", "", "path to TLS CA certificate")
	CobraMcp.Flags().BoolVar(&mcpFlags.Insecure, "insecure", false, "skip TLS verification")
	CobraMcp.Flags().BoolVar(&mcpFlags.ReadOnly, "read-only", false, "disable tasking tools")
	CobraMcp.Flags().StringVar(&mcpFlags.AllowTools, "allow-tools", "", "comma-separated list of allowed tools (default: all)")
	CobraMcp.Flags().StringVar(&mcpFlags.AuditLog, "audit-log", "stderr", "audit log destination (stderr or file path)")
	CobraMcp.Flags().IntVar(&mcpFlags.MaxOutputBytes, "max-output-bytes", 262144, "max output bytes per call")
	CobraMcp.Flags().BoolVar(&mcpFlags.NoReconnect, "no-reconnect", false, "disable automatic reconnection")

	HavocCli.AddCommand(CobraMcp)
}

func mcpFunc(cmd *cobra.Command, args []string) error {
	// Must not call startMenu() - banner would corrupt stdout
	// Redirect logger to stderr before anything else
	logger.SetStdOut(os.Stderr)

	// Apply environment fallbacks for flags that were not set explicitly.
	applyEnv := func(flag, env string, target *string) {
		if cmd.Flags().Changed(flag) {
			return
		}
		if value := os.Getenv(env); value != "" {
			*target = value
		}
	}
	applyEnv("teamserver", "HAVOC_MCP_TEAMSERVER", &mcpFlags.Teamserver)
	applyEnv("user", "HAVOC_MCP_USER", &mcpFlags.User)
	applyEnv("password", "HAVOC_MCP_PASSWORD", &mcpFlags.Password)
	applyEnv("tls-ca", "HAVOC_MCP_TLS_CA", &mcpFlags.TLSCA)

	// Validate required flags
	if mcpFlags.Password == "" {
		return fmt.Errorf("password is required (--password or HAVOC_MCP_PASSWORD)")
	}

	// Parse allow-tools (comma-separated)
	var allowedTools []string
	if mcpFlags.AllowTools != "" {
		for _, tool := range strings.Split(mcpFlags.AllowTools, ",") {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				allowedTools = append(allowedTools, tool)
			}
		}
	}

	// Create operator config
	opCfg := operator.Config{
		URL:         mcpFlags.Teamserver,
		User:        mcpFlags.User,
		Password:    mcpFlags.Password,
		Insecure:    mcpFlags.Insecure,
		TLSCA:       mcpFlags.TLSCA,
		NoReconnect: mcpFlags.NoReconnect,
	}

	// Create MCP config
	mcpCfg := mcp.Config{
		ReadOnly:       mcpFlags.ReadOnly,
		AllowedTools:   allowedTools,
		AuditLog:       mcpFlags.AuditLog,
		MaxOutputBytes: mcpFlags.MaxOutputBytes,
	}

	// Create operator client
	ctx := context.Background()
	opClient, err := operator.Dial(ctx, opCfg)
	if err != nil {
		return fmt.Errorf("failed to dial teamserver: %w", err)
	}
	defer opClient.Close()

	// Start operator client in background to build state
	go func() {
		if err := opClient.Run(ctx); err != nil {
			logger.Error("Operator client error: " + err.Error())
		}
	}()

	// Create MCP server
	server := mcp.NewServer(opClient, mcpCfg)

	// Serve over stdio (this blocks)
	return server.Serve(ctx, os.Stdin, os.Stdout)
}
