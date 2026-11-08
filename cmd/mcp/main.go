// Command mcp serves the read-only parts tools over MCP on stdin/stdout, for MCP clients such as
// Claude Desktop. It connects to the database directly with the credentials in its environment,
// so whoever can launch it can read every part: run it only on a trusted machine.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"partlist/internal/mcpserver"
	"partlist/internal/parts"
	"partlist/internal/partstools"
	"partlist/internal/platform/config"
	"partlist/internal/platform/db"
)

const slowQueryThreshold time.Duration = 200 * time.Millisecond

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := db.Open(cfg.DBURL)
	if err != nil {
		return err
	}
	// stdout carries the MCP protocol, and GORM logs to stdout by default; send its logs to stderr.
	conn = conn.Session(&gorm.Session{Logger: logger.New(
		log.New(os.Stderr, "", log.LstdFlags),
		logger.Config{SlowThreshold: slowQueryThreshold, LogLevel: logger.Warn, IgnoreRecordNotFoundError: true},
	)})

	// No search cache: this is a short-lived process and nothing else would invalidate it.
	server := mcpserver.New(partstools.New(parts.New(conn, parts.Options{})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
