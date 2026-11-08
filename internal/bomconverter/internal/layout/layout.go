// Package layout describes where each part field lives in a BOM sheet (a Mapping), guesses that
// from the sheet's content (Detect) and reads the parts out with it (Apply).
package layout

import (
	"fmt"
	"strings"
	"unicode"
)

// Field names are the keys of Mapping.Fields. They match the JSON names of parts.NewPart.
const (
	FieldDesignator    string  = "designator"
	FieldPartNumber    string  = "part_number"
	FieldDescription   string  = "description"
	FieldFootprint     string  = "footprint"
	FieldFit           string  = "fit"
	FieldManufacturer  string  = "manufacturer"
	FieldValue         string  = "value"
	FieldProjectName   string  = "project_name"
	FieldPcbRef        string  = "pcb_ref"
	maxColumns         int     = 256
	maxCellRow         int     = 1 << 20
	maxConstantRunes   int     = 200
	headerSearchRows   int     = 60
	contentSampleRows  int     = 50
	minDesignatorShare float64 = 0.7
)

// Fields lists every field in the order it is matched.
var Fields []string = []string{
	FieldDesignator, FieldPartNumber, FieldDescription, FieldFootprint, FieldFit,
	FieldManufacturer, FieldValue, FieldProjectName, FieldPcbRef,
}

// Required fields must be mapped: parts.Service refuses a part without them.
// Description and footprint are optional.
var Required []string = []string{
	FieldDesignator, FieldPartNumber, FieldFit, FieldManufacturer, FieldValue, FieldProjectName, FieldPcbRef,
}

// Kind says where a field's value comes from.
type Kind string

const (
	// KindColumn reads the field from a column of every data row.
	KindColumn Kind = "column"
	// KindCell reads one fixed cell (typically the project name above the table) for every row.
	KindCell Kind = "cell"
	// KindText uses a constant the user typed.
	KindText Kind = "text"
)

// Source is one field's origin. Row and Column are zero-based.
type Source struct {
	Kind   Kind   `json:"kind"`
	Column int    `json:"column"`
	Row    int    `json:"row"`
	Text   string `json:"text"`
}

// Mapping tells Apply how to read a sheet. HeaderRow is zero-based; data rows follow it.
type Mapping struct {
	HeaderRow int               `json:"header_row"`
	Fields    map[string]Source `json:"fields"`
}

// Line is one BOM row before it is classified. Designator is raw ("C12", "R3,R4").
type Line struct {
	Designator   string
	PartNumber   string
	Description  string
	Footprint    string
	Fit          string
	Manufacturer string
	Value        string
	ProjectName  string
	PcbRef       string
}

// MappingError is a mapping that cannot be applied. Problem describes a malformed mapping;
// Missing lists required fields that have no source.
type MappingError struct {
	Problem string
	Missing []string
}

func (e *MappingError) Error() string {
	if e.Problem != "" {
		return "invalid mapping: " + e.Problem
	}
	return "invalid mapping: missing " + strings.Join(e.Missing, ", ")
}

// Missing returns the required fields the mapping has no source for, in Required order.
func Missing(m Mapping) []string {
	missing := []string{}
	for _, field := range Required {
		if _, ok := m.Fields[field]; !ok {
			missing = append(missing, field)
		}
	}
	return missing
}

// Validate checks a mapping that came from a client. Nothing in it may point outside sane limits,
// and every required field must have a source.
func Validate(m Mapping) error {
	if err := ValidateShape(m); err != nil {
		return err
	}
	if missing := Missing(m); len(missing) > 0 {
		return &MappingError{Missing: missing}
	}
	return nil
}

// ValidateShape checks everything Validate does except that required fields are present, so a
// half-finished mapping can still be previewed.
func ValidateShape(m Mapping) error {
	if m.HeaderRow < 0 || m.HeaderRow >= maxCellRow {
		return &MappingError{Problem: "header row out of range"}
	}
	for field, src := range m.Fields {
		if !isField(field) {
			return &MappingError{Problem: fmt.Sprintf("unknown field %q", field)}
		}
		if err := validateSource(field, src); err != nil {
			return err
		}
	}
	return nil
}

func validateSource(field string, src Source) error {
	switch src.Kind {
	case KindColumn:
		if src.Column < 0 || src.Column >= maxColumns {
			return &MappingError{Problem: fmt.Sprintf("%s: column out of range", field)}
		}
	case KindCell:
		if src.Column < 0 || src.Column >= maxColumns || src.Row < 0 || src.Row >= maxCellRow {
			return &MappingError{Problem: fmt.Sprintf("%s: cell out of range", field)}
		}
	case KindText:
		if strings.TrimSpace(src.Text) == "" || len([]rune(src.Text)) > maxConstantRunes {
			return &MappingError{Problem: fmt.Sprintf("%s: text must be 1-%d characters", field, maxConstantRunes)}
		}
	default:
		return &MappingError{Problem: fmt.Sprintf("%s: unknown kind %q", field, src.Kind)}
	}
	if field == FieldDesignator && src.Kind != KindColumn {
		return &MappingError{Problem: "designator must come from a column"}
	}
	return nil
}

// Reusable returns the part of a mapping worth remembering for the next file with the same columns.
// A project name or PCB ref the user typed belongs to this one file, and saving it would stamp every
// later file with it, so those are dropped; the next user is asked for them again.
func Reusable(m Mapping) Mapping {
	fields := make(map[string]Source, len(m.Fields))
	for field, src := range m.Fields {
		if src.Kind == KindText && (field == FieldProjectName || field == FieldPcbRef) {
			continue
		}
		fields[field] = src
	}
	return Mapping{HeaderRow: m.HeaderRow, Fields: fields}
}

func isField(name string) bool {
	for _, field := range Fields {
		if field == name {
			return true
		}
	}
	return false
}

// Apply reads every data row under the header row. Fields without a source are empty. A row with
// no designator, or one that repeats the header, is not a part. The mapping must be valid.
func Apply(rows [][]string, m Mapping) []Line {
	if m.HeaderRow >= len(rows) {
		return nil
	}
	designatorHeader := ""
	if src, ok := m.Fields[FieldDesignator]; ok {
		designatorHeader = Clean(cellAt(rows, m.HeaderRow, src.Column))
	}
	read := func(field string, rowIdx int) string {
		src, ok := m.Fields[field]
		if !ok {
			return ""
		}
		switch src.Kind {
		case KindColumn:
			return Clean(cellAt(rows, rowIdx, src.Column))
		case KindCell:
			return Clean(cellAt(rows, src.Row, src.Column))
		case KindText:
			return Clean(src.Text)
		}
		return ""
	}

	var lines []Line
	for i := m.HeaderRow + 1; i < len(rows); i++ {
		designator := read(FieldDesignator, i)
		if designator == "" || designator == designatorHeader {
			continue
		}
		lines = append(lines, Line{
			Designator:   designator,
			PartNumber:   read(FieldPartNumber, i),
			Description:  read(FieldDescription, i),
			Footprint:    read(FieldFootprint, i),
			Fit:          read(FieldFit, i),
			Manufacturer: read(FieldManufacturer, i),
			Value:        read(FieldValue, i),
			ProjectName:  read(FieldProjectName, i),
			PcbRef:       read(FieldPcbRef, i),
		})
	}
	return lines
}

// Resolve returns the value a field has on the first data row, for showing the user what a
// mapping will produce (the project name, typically). Empty when it has no source or no value.
func Resolve(rows [][]string, m Mapping, field string) string {
	src, ok := m.Fields[field]
	if !ok {
		return ""
	}
	switch src.Kind {
	case KindColumn:
		for i := m.HeaderRow + 1; i < len(rows); i++ {
			if v := Clean(cellAt(rows, i, src.Column)); v != "" {
				return v
			}
		}
		return ""
	case KindCell:
		return Clean(cellAt(rows, src.Row, src.Column))
	case KindText:
		return Clean(src.Text)
	}
	return ""
}

func cellAt(rows [][]string, row int, column int) string {
	if row < 0 || row >= len(rows) || column < 0 || column >= len(rows[row]) {
		return ""
	}
	return rows[row][column]
}

// Clean trims a cell and folds line breaks into spaces.
func Clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " "))
}

// normalise lowercases a header and drops everything but letters and digits, so
// "Manufacturer part no." and "MANUFACTURER PART NO" compare equal.
func normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
