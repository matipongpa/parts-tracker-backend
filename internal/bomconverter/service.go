package bomconverter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"partlist/internal/bomconverter/internal/parser"
	"partlist/internal/parts"
)

var ErrInvalidFile error = errors.New("bomconverter: invalid BOM file")

// ImportResult counts rows of the uploaded BOM.
// Skipped rows have an unknown designator or lack a required part field.
type ImportResult struct {
	Total    int `json:"total"`
	Inserted int `json:"inserted"`
	Skipped  int `json:"skipped"`
}

// Service is the public surface of the bomconverter module.
type Service interface {
	// Import parses an .xlsx/.xlsm BOM and stores its valid rows in one transaction.
	Import(ctx context.Context, file io.Reader) (ImportResult, error)
}

type service struct {
	parts parts.Service
}

func New(partsSvc parts.Service) Service {
	return &service{parts: partsSvc}
}

func (s *service) Import(ctx context.Context, file io.Reader) (ImportResult, error) {
	rows, err := parser.ReadFirstSheet(file)
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	lines, err := parser.Parse(rows)
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}

	valid := make([]parts.NewPart, 0, len(lines))
	for _, line := range lines {
		if p, ok := s.toNewPart(line); ok {
			valid = append(valid, p)
		}
	}

	inserted, err := s.parts.BulkCreate(ctx, valid)
	if err != nil {
		return ImportResult{}, fmt.Errorf("store parts: %w", err)
	}
	return ImportResult{Total: len(lines), Inserted: inserted, Skipped: len(lines) - len(valid)}, nil
}

func (s *service) toNewPart(line parser.Line) (parts.NewPart, bool) {
	partType := s.parts.ClassifyDesignator(line.Designator)
	if partType == parts.PartTypeUnknown {
		return parts.NewPart{}, false
	}
	p := parts.NewPart{
		PartNumber:   line.PartNumber,
		PcbRef:       line.PcbRef,
		Description:  line.Description,
		Footprint:    line.Footprint,
		Fit:          line.Fit,
		Manufacturer: line.Manufacturer,
		Type:         strings.ToLower(string(partType)),
		ProjectName:  line.ProjectName,
		Value:        line.Value,
	}
	return p, p.Validate() == nil
}
