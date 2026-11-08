package bomconverter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"gorm.io/gorm"

	"partlist/internal/bomconverter/internal/layout"
	"partlist/internal/bomconverter/internal/parser"
	"partlist/internal/bomconverter/internal/store"
	"partlist/internal/parts"
)

var (
	ErrInvalidFile  error = errors.New("bomconverter: invalid BOM file")
	ErrUnknownSheet error = errors.New("bomconverter: no such sheet")
)

// Mapping, Source and MappingError are the layout types clients send and receive.
type (
	Mapping      = layout.Mapping
	Source       = layout.Source
	MappingError = layout.MappingError
)

const (
	previewMinRows     int = 25
	previewMaxRows     int = 100
	previewMaxColumns  int = 40
	previewMaxCellRune int = 100
	// headerContextRows is how many data rows the preview shows under the header row.
	headerContextRows int = 10
)

// Where a Preview's mapping came from.
const (
	SourceSaved    string = "saved"
	SourceDetected string = "detected"
	SourceGiven    string = "given" // the client's own mapping, being dry-run
)

// ImportResult counts rows of the uploaded BOM.
// Skipped rows have an unknown designator or lack a required part field.
// Duplicates are valid rows that already exist (or repeat earlier in the file) and are not stored again.
// MappingSaved says the confirmed mapping was stored for the next file with the same columns.
type ImportResult struct {
	Total        int  `json:"total"`
	Inserted     int  `json:"inserted"`
	Skipped      int  `json:"skipped"`
	Duplicates   int  `json:"duplicates"`
	MappingSaved bool `json:"mapping_saved"`
}

// Summary says what importing the sheet with the previewed mapping would do, before duplicates are checked.
type Summary struct {
	Rows    int `json:"rows"`
	Valid   int `json:"valid"`
	Skipped int `json:"skipped"`
}

// Preview shows a sheet and the mapping the app proposes for it. Rows is a grid of the top of the sheet,
// padded to Columns, so the user can check the proposal against what is really in the file.
type Preview struct {
	Sheets         []string          `json:"sheets"`
	Sheet          string            `json:"sheet"`
	HeaderRow      int               `json:"header_row"`
	HeaderDetected bool              `json:"header_detected"`
	TotalRows      int               `json:"total_rows"`
	Columns        int               `json:"columns"`
	Rows           [][]string        `json:"rows"`
	Mapping        Mapping           `json:"mapping"`
	MappingSource  string            `json:"mapping_source"`
	Confidence     map[string]string `json:"confidence"`
	Missing        []string          `json:"missing"`
	Resolved       map[string]string `json:"resolved"`
	Summary        Summary           `json:"summary"`
}

// PreviewOptions picks what to preview. A blank Sheet means the first sheet that looks like a BOM.
// HeaderRow overrides header detection when non-nil (zero-based). A non-nil Mapping is used as is instead
// of a saved or detected one, so the client can see what its edits would do; it may be incomplete.
type PreviewOptions struct {
	Sheet     string
	HeaderRow *int
	Mapping   *Mapping
}

// ImportOptions control one import. A nil Mapping means "use the saved mapping for these columns, else the
// detected one", which keeps plain uploads (and the CLI) working; it fails when a required field is unmapped.
// SaveMapping stores a client-supplied Mapping for later files; User is recorded with it.
type ImportOptions struct {
	Sheet       string
	Mapping     *Mapping
	SaveMapping bool
	User        string
}

// Service is the public surface of the bomconverter module.
type Service interface {
	// Preview reads an .xlsx/.xlsm BOM and proposes how its columns map to part fields. Nothing is stored.
	Preview(ctx context.Context, file io.Reader, opts PreviewOptions) (Preview, error)
	// Import reads the BOM with the mapping and stores its valid rows in one transaction.
	Import(ctx context.Context, file io.Reader, opts ImportOptions) (ImportResult, error)
}

// mappingStore remembers confirmed mappings by header fingerprint.
type mappingStore interface {
	Find(ctx context.Context, fingerprint string) (layout.Mapping, bool, error)
	Save(ctx context.Context, fingerprint string, m layout.Mapping, user string) error
}

type service struct {
	parts parts.Service
	store mappingStore
}

func New(partsSvc parts.Service, db *gorm.DB) Service {
	return newService(partsSvc, store.New(db))
}

func newService(partsSvc parts.Service, mappings mappingStore) *service {
	return &service{parts: partsSvc, store: mappings}
}

// sheetData is the chosen sheet's rows.
type sheetData struct {
	names []string
	name  string
	rows  [][]string
}

func (s *service) Preview(ctx context.Context, file io.Reader, opts PreviewOptions) (Preview, error) {
	wb, err := parser.Open(file)
	if err != nil {
		return Preview{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	defer wb.Close()
	sheet, err := chooseSheet(wb, opts.Sheet)
	if err != nil {
		return Preview{}, err
	}

	if opts.Mapping != nil {
		return s.previewGiven(sheet, *opts.Mapping)
	}

	headerRow, detected := 0, false
	if opts.HeaderRow != nil {
		headerRow, detected = *opts.HeaderRow, *opts.HeaderRow >= 0 && *opts.HeaderRow < len(sheet.rows)
	} else {
		headerRow, detected = layout.DetectHeader(sheet.rows)
	}

	mapping, confidence := layout.Detect(sheet.rows, headerRow)
	source := SourceDetected
	if detected {
		saved, found, err := s.savedMapping(ctx, sheet.rows, headerRow)
		if err != nil {
			return Preview{}, err
		}
		if found {
			mapping, confidence, source = saved, markSaved(saved), SourceSaved
		}
	}

	return s.describe(sheet, mapping, source, confidence, detected), nil
}

// previewGiven describes a sheet under the client's mapping. The mapping must be well formed, not complete.
func (s *service) previewGiven(sheet sheetData, mapping Mapping) (Preview, error) {
	if err := layout.ValidateShape(mapping); err != nil {
		return Preview{}, err
	}
	return s.describe(sheet, mapping, SourceGiven, map[string]string{}, mapping.HeaderRow < len(sheet.rows)), nil
}

func (s *service) describe(sheet sheetData, mapping Mapping, source string, confidence map[string]string, detected bool) Preview {
	missing := layout.Missing(mapping)
	summary := Summary{}
	if detected && len(missing) == 0 {
		summary = s.summarise(layout.Apply(sheet.rows, mapping))
	}
	grid, columns := previewGrid(sheet.rows, mapping.HeaderRow)
	return Preview{
		Sheets:         sheet.names,
		Sheet:          sheet.name,
		HeaderRow:      mapping.HeaderRow,
		HeaderDetected: detected,
		TotalRows:      len(sheet.rows),
		Columns:        columns,
		Rows:           grid,
		Mapping:        mapping,
		MappingSource:  source,
		Confidence:     confidence,
		Missing:        missing,
		Resolved: map[string]string{
			layout.FieldProjectName: layout.Resolve(sheet.rows, mapping, layout.FieldProjectName),
			layout.FieldPcbRef:      layout.Resolve(sheet.rows, mapping, layout.FieldPcbRef),
		},
		Summary: summary,
	}
}

func (s *service) Import(ctx context.Context, file io.Reader, opts ImportOptions) (ImportResult, error) {
	wb, err := parser.Open(file)
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	defer wb.Close()
	sheet, err := chooseSheet(wb, opts.Sheet)
	if err != nil {
		return ImportResult{}, err
	}

	mapping, err := s.resolveMapping(ctx, sheet.rows, opts.Mapping)
	if err != nil {
		return ImportResult{}, err
	}
	lines := layout.Apply(sheet.rows, mapping)
	valid := s.validParts(lines)

	inserted, err := s.parts.BulkCreate(ctx, valid)
	if err != nil {
		return ImportResult{}, fmt.Errorf("store parts: %w", err)
	}
	result := ImportResult{
		Total:      len(lines),
		Inserted:   inserted,
		Skipped:    len(lines) - len(valid),
		Duplicates: len(valid) - inserted,
	}
	if opts.Mapping != nil && opts.SaveMapping {
		result.MappingSaved = s.saveMapping(ctx, sheet.rows, mapping, opts.User)
	}
	return result, nil
}

// resolveMapping validates the client's mapping, or falls back to the saved one and then the detected one.
func (s *service) resolveMapping(ctx context.Context, rows [][]string, given *Mapping) (Mapping, error) {
	if given != nil {
		if err := layout.Validate(*given); err != nil {
			return Mapping{}, err
		}
		return *given, nil
	}
	headerRow, ok := layout.DetectHeader(rows)
	if !ok {
		return Mapping{}, &MappingError{Problem: "no header row found"}
	}
	mapping, _ := layout.Detect(rows, headerRow)
	if saved, found, err := s.savedMapping(ctx, rows, headerRow); err != nil {
		return Mapping{}, err
	} else if found {
		mapping = saved
	}
	if err := layout.Validate(mapping); err != nil {
		return Mapping{}, err
	}
	return mapping, nil
}

// savedMapping looks up the team's mapping for this header row. Its HeaderRow is set to the row it was found for.
func (s *service) savedMapping(ctx context.Context, rows [][]string, headerRow int) (Mapping, bool, error) {
	fingerprint := fingerprintAt(rows, headerRow)
	if fingerprint == "" {
		return Mapping{}, false, nil
	}
	saved, found, err := s.store.Find(ctx, fingerprint)
	if err != nil {
		return Mapping{}, false, err
	}
	if !found {
		return Mapping{}, false, nil
	}
	saved.HeaderRow = headerRow
	return saved, true, nil
}

// saveMapping failing must not undo an import that already committed, so it only reports whether it worked.
func (s *service) saveMapping(ctx context.Context, rows [][]string, m Mapping, user string) bool {
	fingerprint := fingerprintAt(rows, m.HeaderRow)
	if fingerprint == "" {
		return false
	}
	if err := s.store.Save(ctx, fingerprint, layout.Reusable(m), user); err != nil {
		log.Printf("bomconverter: %v", err)
		return false
	}
	return true
}

func fingerprintAt(rows [][]string, headerRow int) string {
	if headerRow < 0 || headerRow >= len(rows) {
		return ""
	}
	return layout.Fingerprint(rows[headerRow])
}

func markSaved(m Mapping) map[string]string {
	confidence := make(map[string]string, len(m.Fields))
	for field := range m.Fields {
		confidence[field] = layout.ConfidenceSaved
	}
	return confidence
}

func chooseSheet(wb *parser.Workbook, requested string) (sheetData, error) {
	names := wb.SheetNames()
	if len(names) == 0 {
		return sheetData{}, fmt.Errorf("%w: workbook has no sheets", ErrInvalidFile)
	}
	if requested != "" {
		for _, name := range names {
			if name != requested {
				continue
			}
			rows, err := wb.Rows(name)
			if err != nil {
				return sheetData{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
			}
			return sheetData{names: names, name: name, rows: rows}, nil
		}
		return sheetData{}, ErrUnknownSheet
	}

	// No sheet asked for: the first one that looks like a BOM, else the first sheet.
	var first sheetData
	for i, name := range names {
		rows, err := wb.Rows(name)
		if err != nil {
			return sheetData{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
		}
		candidate := sheetData{names: names, name: name, rows: rows}
		if i == 0 {
			first = candidate
		}
		if _, ok := layout.DetectHeader(rows); ok {
			return candidate, nil
		}
	}
	return first, nil
}

// previewGrid returns the top of the sheet as a rectangle, with long cells shortened.
func previewGrid(rows [][]string, headerRow int) ([][]string, int) {
	count := min(len(rows), previewMaxRows, max(previewMinRows, headerRow+headerContextRows+1))
	columns := 0
	for _, row := range rows[:count] {
		columns = max(columns, len(row))
	}
	columns = min(columns, previewMaxColumns)

	grid := make([][]string, count)
	for i := range grid {
		grid[i] = make([]string, columns)
		for j := 0; j < columns && j < len(rows[i]); j++ {
			grid[i][j] = shorten(layout.Clean(rows[i][j]))
		}
	}
	return grid, columns
}

func shorten(s string) string {
	runes := []rune(s)
	if len(runes) <= previewMaxCellRune {
		return s
	}
	return string(runes[:previewMaxCellRune]) + "…"
}

func (s *service) summarise(lines []layout.Line) Summary {
	valid := len(s.validParts(lines))
	return Summary{Rows: len(lines), Valid: valid, Skipped: len(lines) - valid}
}

// validParts keeps the lines with a known designator and every required field.
func (s *service) validParts(lines []layout.Line) []parts.NewPart {
	valid := make([]parts.NewPart, 0, len(lines))
	for _, line := range lines {
		if p, ok := s.toNewPart(line); ok {
			valid = append(valid, p)
		}
	}
	return valid
}

func (s *service) toNewPart(line layout.Line) (parts.NewPart, bool) {
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
