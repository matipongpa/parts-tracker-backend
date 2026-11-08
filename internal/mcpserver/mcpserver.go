// Package mcpserver serves the read-only partstools over the Model Context Protocol, so MCP
// clients such as Claude Desktop can query the parts database with their own model.
package mcpserver

import (
	"context"
	"errors"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"partlist/internal/partstools"
)

const (
	serverName    string = "parts-tracker"
	serverVersion string = "0.1.0"
)

// New returns an MCP server exposing the given tools. Every tool is marked read-only.
func New(tools []partstools.Tool) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	openWorld := false
	for _, tool := range tools {
		server.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Schema(),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &openWorld},
		}, handler(tool))
	}
	return server
}

// handler adapts a tool to MCP. Tool failures are reported in the result with IsError set, not as
// protocol errors, so the calling model can read the message and correct its arguments.
func handler(tool partstools.Tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		out, err := tool.Call(ctx, req.Params.Arguments)
		switch {
		case errors.Is(err, partstools.ErrInvalidArguments):
			return errorResult(err.Error()), nil
		case err != nil:
			log.Printf("mcpserver: tool %s failed: %v", tool.Name, err)
			return errorResult("the tool failed with an internal error"), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Content}}}, nil
	}
}

func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}
