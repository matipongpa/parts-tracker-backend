package partstools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"partlist/internal/parts"
)

type fakeParts struct {
	parts.Service
	lastQuery parts.SearchQuery
	searchRes parts.SearchResult
	usageRes  parts.UsageResult
	usageErr  error
	lastExact bool
}

func (f *fakeParts) Search(_ context.Context, q parts.SearchQuery) (parts.SearchResult, error) {
	f.lastQuery = q
	return f.searchRes, nil
}

func (f *fakeParts) Usage(_ context.Context, _ string, exact bool) (parts.UsageResult, error) {
	f.lastExact = exact
	return f.usageRes, f.usageErr
}

func byName(t *testing.T, tools []Tool, name string) Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return Tool{}
}

func TestToolsAreReadOnly(t *testing.T) {
	for _, tool := range New(&fakeParts{}) {
		for _, banned := range []string{"create", "update", "delete", "import"} {
			if strings.Contains(tool.Name, banned) {
				t.Errorf("tool %q looks like a write tool", tool.Name)
			}
		}
	}
}

func TestSchemaIsAnObjectWithRequiredFields(t *testing.T) {
	schema := byName(t, New(&fakeParts{}), ToolGetPartUsage).Schema()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("schema = %v", schema)
	}
	if req, _ := schema["required"].([]string); len(req) != 1 || req[0] != "part_number" {
		t.Errorf("required = %v", schema["required"])
	}
	if _, has := byName(t, New(&fakeParts{}), ToolSearchParts).Schema()["required"]; has {
		t.Errorf("search_parts has no required arguments")
	}
}

func TestSearchPartsClampsPageSizeAndReturnsParts(t *testing.T) {
	svc := &fakeParts{searchRes: parts.SearchResult{Count: 42, List: []parts.Part{{ID: "1", PartNumber: "ABC"}}}}
	tool := byName(t, New(svc), ToolSearchParts)

	out, err := tool.Call(context.Background(), json.RawMessage(`{"type":"capacitor","page_size":9999}`))
	if err != nil {
		t.Fatal(err)
	}
	if svc.lastQuery.Type != "capacitor" || svc.lastQuery.PageSize != maxSearchSize {
		t.Errorf("query = %+v", svc.lastQuery)
	}
	if len(out.Parts) != 1 || out.Parts[0].ID != "1" {
		t.Errorf("parts = %+v", out.Parts)
	}
	var decoded searchOutput
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil || decoded.Count != 42 || decoded.Returned != 1 {
		t.Errorf("content = %s (%v)", out.Content, err)
	}
}

func TestSearchPartsDefaultsPageSizeAndAcceptsNoArguments(t *testing.T) {
	svc := &fakeParts{}
	tool := byName(t, New(svc), ToolSearchParts)
	for _, raw := range []string{"", "null", "{}"} {
		if _, err := tool.Call(context.Background(), json.RawMessage(raw)); err != nil {
			t.Errorf("Call(%q) = %v", raw, err)
		}
		if svc.lastQuery.PageSize != defaultSearchSize {
			t.Errorf("page size = %d, want %d", svc.lastQuery.PageSize, defaultSearchSize)
		}
	}
}

func TestUnknownOrMalformedArgumentsAreInvalid(t *testing.T) {
	tool := byName(t, New(&fakeParts{}), ToolSearchParts)
	for _, raw := range []string{`{"sql":"DROP TABLE parts"}`, `{"page":"two"}`, `[1]`, `{`} {
		if _, err := tool.Call(context.Background(), json.RawMessage(raw)); !errors.Is(err, ErrInvalidArguments) {
			t.Errorf("Call(%s) err = %v, want ErrInvalidArguments", raw, err)
		}
	}
}

func TestGetPartUsageMapsBlankPartNumberToInvalidArguments(t *testing.T) {
	svc := &fakeParts{usageErr: parts.ErrInvalidInput}
	tool := byName(t, New(svc), ToolGetPartUsage)
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"part_number":""}`)); !errors.Is(err, ErrInvalidArguments) {
		t.Errorf("err = %v, want ErrInvalidArguments", err)
	}
}

func TestGetPartUsageFoldsBoardsIntoVariantsAndReturnsEveryPart(t *testing.T) {
	cap10k := func(id string, ref string) parts.Part {
		return parts.Part{ID: id, PartNumber: "ABC", PcbRef: ref, Value: "10k", Footprint: "0603", Manufacturer: "Yageo"}
	}
	svc := &fakeParts{usageRes: parts.UsageResult{
		PartNumber: "ABC", Matches: 4,
		Projects: []parts.ProjectUsage{
			{ProjectName: "A", Parts: []parts.Part{cap10k("1", "PCB-1"), cap10k("2", "PCB-2")}},
			{ProjectName: "B", Parts: []parts.Part{
				cap10k("3", "PCB-9"),
				{ID: "4", PartNumber: "ABC", PcbRef: "PCB-9", Value: "10k", Footprint: "0805", Manufacturer: "Yageo"},
			}},
		},
	}}
	tool := byName(t, New(svc), ToolGetPartUsage)

	out, err := tool.Call(context.Background(), json.RawMessage(`{"part_number":"ABC","exact":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !svc.lastExact {
		t.Errorf("exact flag not passed through")
	}
	if len(out.Parts) != 4 {
		t.Errorf("parts = %d, want all 4 rows for the caller", len(out.Parts))
	}

	var view usageView
	if err := json.Unmarshal([]byte(out.Content), &view); err != nil {
		t.Fatal(err)
	}
	if view.ProjectCount != 2 || len(view.Projects[0].Variants) != 1 || len(view.Projects[1].Variants) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if boards := view.Projects[0].Variants[0].Boards; len(boards) != 2 || boards[0] != "PCB-1" || boards[1] != "PCB-2" {
		t.Errorf("project A boards = %v", boards)
	}
}

func TestGetPartUsageNoMatchEncodesEmptyLists(t *testing.T) {
	svc := &fakeParts{usageRes: parts.UsageResult{PartNumber: "NOPE", Projects: []parts.ProjectUsage{}}}
	out, err := byName(t, New(svc), ToolGetPartUsage).Call(context.Background(), json.RawMessage(`{"part_number":"NOPE"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, `"projects":[]`) || len(out.Parts) != 0 {
		t.Errorf("content = %s, parts = %d", out.Content, len(out.Parts))
	}
}
