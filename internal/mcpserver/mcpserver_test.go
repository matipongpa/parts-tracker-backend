package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"partlist/internal/parts"
	"partlist/internal/partstools"
)

type fakeParts struct {
	parts.Service
	usage    parts.UsageResult
	usageErr error
}

func (f *fakeParts) Usage(context.Context, string, bool) (parts.UsageResult, error) {
	return f.usage, f.usageErr
}

// connect runs the server and a client over an in-memory transport, as a real MCP client would.
func connect(t *testing.T, svc parts.Service) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverSide, clientSide := mcp.NewInMemoryTransports()
	serverSession, err := New(partstools.New(svc)).Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("content = %+v", res.Content)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return text.Text
}

func TestListsOnlyReadOnlyTools(t *testing.T) {
	session := connect(t, &fakeParts{})
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not marked read-only", tool.Name)
		}
	}
	if len(res.Tools) != 2 || !names["search_parts"] || !names["get_part_usage"] {
		t.Errorf("tools = %v", names)
	}
}

func TestCallToolReturnsUsage(t *testing.T) {
	session := connect(t, &fakeParts{usage: parts.UsageResult{
		PartNumber: "ABC", Matches: 1,
		Projects: []parts.ProjectUsage{{ProjectName: "Rev B", Parts: []parts.Part{{ID: "1", PartNumber: "ABC", PcbRef: "PCB-1"}}}},
	}})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_part_usage", Arguments: map[string]any{"part_number": "ABC", "exact": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", textOf(t, res))
	}
	if text := textOf(t, res); !strings.Contains(text, "Rev B") || !strings.Contains(text, "PCB-1") {
		t.Errorf("text = %s", text)
	}
}

func TestToolFailuresAreResultsNotProtocolErrors(t *testing.T) {
	session := connect(t, &fakeParts{usageErr: errors.New("pq: password=hunter2")})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_part_usage", Arguments: map[string]any{"part_number": "ABC"}})
	if err != nil {
		t.Fatalf("an internal tool failure must not be a protocol error: %v", err)
	}
	if !res.IsError || strings.Contains(textOf(t, res), "hunter2") {
		t.Errorf("result = %+v (want IsError without internal details)", res)
	}
}

func TestInvalidArgumentsAreReportedToTheCaller(t *testing.T) {
	session := connect(t, &fakeParts{usageErr: parts.ErrInvalidInput})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_part_usage", Arguments: map[string]any{"part_number": " "}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(textOf(t, res), "part_number must not be empty") {
		t.Errorf("result = %+v", res)
	}
}
