// Package partstools exposes read-only parts queries as tools an AI model can call.
//
// The tools are defined once, as plain Go over parts.Service, and adapted two ways: the
// assistant module feeds them to the Claude API, and internal/mcpserver serves them over MCP.
// There is deliberately no tool that creates, updates or deletes a part: part descriptions
// come from uploaded spreadsheets, so a model reading them must never be able to write.
package partstools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"partlist/internal/parts"
)

// ErrInvalidArguments means the model (or MCP client) sent arguments the tool cannot use.
// Callers report it back as a tool error so the model can correct itself.
var ErrInvalidArguments error = errors.New("partstools: invalid arguments")

const (
	ToolSearchParts   string = "search_parts"
	ToolGetPartUsage  string = "get_part_usage"
	defaultSearchSize int    = 20
	maxSearchSize     int    = 50
)

// Spec describes a tool to a model: its name, what it does, and its arguments.
type Spec struct {
	Name        string
	Description string
	// Properties is the JSON-schema "properties" object of the arguments.
	Properties map[string]any
	Required   []string
}

// Schema returns the full JSON-schema object for the tool's arguments.
func (s Spec) Schema() map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           s.Properties,
		"additionalProperties": false,
	}
	if len(s.Required) > 0 {
		schema["required"] = s.Required
	}
	return schema
}

// Output is what a tool call returns. Content is JSON text for the model; Parts are the
// stored rows the call surfaced, so a caller can show them without trusting the model's retelling.
type Output struct {
	Content string
	Parts   []parts.Part
}

// Tool is a Spec plus the function that runs it. Call receives the raw JSON arguments.
type Tool struct {
	Spec
	Call func(ctx context.Context, args json.RawMessage) (Output, error)
}

// New returns the read-only tools backed by svc.
func New(svc parts.Service) []Tool {
	return []Tool{searchPartsTool(svc), getPartUsageTool(svc)}
}

type searchArgs struct {
	PcbRef      string `json:"pcb_ref"`
	PartNumber  string `json:"part_number"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
}

type searchOutput struct {
	Count    int64        `json:"count"`
	Page     int          `json:"page"`
	Returned int          `json:"returned"`
	List     []parts.Part `json:"list"`
}

func searchPartsTool(svc parts.Service) Tool {
	return Tool{
		Spec: Spec{
			Name: ToolSearchParts,
			Description: "Search stored BOM parts. Every filter is a case-insensitive substring match and filters are ANDed; " +
				"omit a filter to ignore it. Searches all projects. Returns the total match count and one page of parts " +
				"(id, part_number, pcb_ref, project_name, value, description, footprint, manufacturer, type, fit). " +
				"Parts are stored once per board per project: there are no quantities or reference designators.",
			Properties: map[string]any{
				"part_number": stringProp("Manufacturer part number, e.g. 'GRM188R71H104KA93D'."),
				"value":       stringProp("Component value, e.g. '100nF' or '10k'."),
				"description": stringProp("Words from the part description."),
				"type":        stringProp("Part category in lowercase, e.g. 'capacitor', 'resistor', 'diode', 'integrated circuit (ic)'."),
				"pcb_ref":     stringProp("Board code from the BOM sheet's 'PCB Ref:' cell (not a reference designator)."),
				"page":        intProp("Zero-based page number. Default 0."),
				"page_size":   intProp(fmt.Sprintf("Parts per page, 1-%d. Default %d.", maxSearchSize, defaultSearchSize)),
			},
		},
		Call: func(ctx context.Context, raw json.RawMessage) (Output, error) {
			var args searchArgs
			if err := decodeArgs(raw, &args); err != nil {
				return Output{}, err
			}
			size := clampPageSize(args.PageSize)
			res, err := svc.Search(ctx, parts.SearchQuery{
				PcbRef: args.PcbRef, PartNumber: args.PartNumber, Value: args.Value,
				Description: args.Description, Type: args.Type, Page: args.Page, PageSize: size,
			})
			if err != nil {
				return Output{}, err
			}
			content, err := marshal(searchOutput{Count: res.Count, Page: max(args.Page, 0), Returned: len(res.List), List: res.List})
			if err != nil {
				return Output{}, err
			}
			return Output{Content: content, Parts: res.List}, nil
		},
	}
}

type usageArgs struct {
	PartNumber string `json:"part_number"`
	Exact      bool   `json:"exact"`
}

func getPartUsageTool(svc parts.Service) Tool {
	return Tool{
		Spec: Spec{
			Name: ToolGetPartUsage,
			Description: "Find where a part number is used across ALL projects. Returns the projects that use it, and for each " +
				"project the distinct variants (value, description, footprint, manufacturer, type, fit) with the boards (pcb_ref) " +
				"they appear on. Use this for questions like 'which projects use part X?'. " +
				"Matching is case-insensitive; a substring unless exact is true. No quantities or reference designators are stored.",
			Properties: map[string]any{
				"part_number": stringProp("The part number to look up."),
				"exact":       map[string]any{"type": "boolean", "description": "Match the whole part number instead of any part number containing the text. Default false."},
			},
			Required: []string{"part_number"},
		},
		Call: func(ctx context.Context, raw json.RawMessage) (Output, error) {
			var args usageArgs
			if err := decodeArgs(raw, &args); err != nil {
				return Output{}, err
			}
			res, err := svc.Usage(ctx, args.PartNumber, args.Exact)
			if errors.Is(err, parts.ErrInvalidInput) {
				return Output{}, fmt.Errorf("%w: part_number must not be empty", ErrInvalidArguments)
			}
			if err != nil {
				return Output{}, err
			}
			content, err := marshal(toUsageView(res))
			if err != nil {
				return Output{}, err
			}
			return Output{Content: content, Parts: flattenUsage(res)}, nil
		},
	}
}

// usageView is the compact form the model sees. Boards that share every attribute are folded
// into one variant, which keeps a part used on dozens of boards from flooding the context.
type usageView struct {
	PartNumber   string        `json:"part_number"`
	ProjectCount int           `json:"project_count"`
	Matches      int           `json:"matches"`
	Truncated    bool          `json:"truncated"`
	Projects     []projectView `json:"projects"`
}

type projectView struct {
	ProjectName string        `json:"project_name"`
	Variants    []variantView `json:"variants"`
}

type variantView struct {
	PartNumber   string   `json:"part_number"`
	Value        string   `json:"value"`
	Description  string   `json:"description"`
	Footprint    string   `json:"footprint"`
	Manufacturer string   `json:"manufacturer"`
	Type         string   `json:"type"`
	Fit          string   `json:"fit"`
	Boards       []string `json:"boards"`
}

func toUsageView(res parts.UsageResult) usageView {
	view := usageView{
		PartNumber: res.PartNumber, ProjectCount: len(res.Projects), Matches: res.Matches,
		Truncated: res.Truncated, Projects: make([]projectView, 0, len(res.Projects)),
	}
	for _, project := range res.Projects {
		pv := projectView{ProjectName: project.ProjectName, Variants: []variantView{}}
		index := map[string]int{}
		for _, p := range project.Parts {
			key := strings.Join([]string{p.PartNumber, p.Value, p.Description, p.Footprint, p.Manufacturer, p.Type, p.Fit}, "\x1f")
			i, seen := index[key]
			if !seen {
				i = len(pv.Variants)
				index[key] = i
				pv.Variants = append(pv.Variants, variantView{
					PartNumber: p.PartNumber, Value: p.Value, Description: p.Description, Footprint: p.Footprint,
					Manufacturer: p.Manufacturer, Type: p.Type, Fit: p.Fit, Boards: []string{},
				})
			}
			pv.Variants[i].Boards = append(pv.Variants[i].Boards, p.PcbRef)
		}
		view.Projects = append(view.Projects, pv)
	}
	return view
}

func flattenUsage(res parts.UsageResult) []parts.Part {
	all := make([]parts.Part, 0, res.Matches)
	for _, project := range res.Projects {
		all = append(all, project.Parts...)
	}
	return all
}

func clampPageSize(n int) int {
	switch {
	case n <= 0:
		return defaultSearchSize
	case n > maxSearchSize:
		return maxSearchSize
	}
	return n
}

// decodeArgs strictly decodes the arguments; an empty or null body means "no arguments".
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	return nil
}

func marshal(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("partstools: encode result: %w", err)
	}
	return string(out), nil
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func intProp(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
