package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

var (
	ErrUnreadable   error = errors.New("parser: file is not a readable Excel workbook")
	ErrNoDesignator error = errors.New("parser: no \"Designator\" header row found")
)

const (
	projectNameKey     string = "Project Name :"
	projectCodeKey     string = "PCB Ref:"
	designatorHeader   string = "Designator"
	headerDescription  string = "Description"
	headerFit          string = "FIT/NFIT"
	headerFootprint    string = "Footprint"
	headerManufacturer string = "Manufacturer"
	headerManufPartNo  string = "Manufacturer part no."
	headerValue        string = "Value"
)

// Line is one BOM row. Designator is raw ("C12", "R3,R4"); classification is the caller's job.
type Line struct {
	Designator   string
	Description  string
	Fit          string
	Footprint    string
	Manufacturer string
	PartNumber   string
	Value        string
	ProjectName  string
	PcbRef       string
}

// ReadFirstSheet returns the rows of the first sheet of an .xlsx/.xlsm workbook.
func ReadFirstSheet(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return rows, nil
}

// Parse extracts BOM lines. Project name and PCB ref come from key/value rows above the
// "Designator" header row; every non-empty row after the header is a line.
func Parse(rows [][]string) ([]Line, error) {
	var (
		project, pcbRef string
		columns         map[string]int
		lines           []Line
	)
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if columns == nil {
			if len(row) > 1 {
				switch row[0] {
				case projectNameKey:
					project = clean(row[1])
				case projectCodeKey:
					pcbRef = clean(row[1])
				}
			}
			if row[0] == designatorHeader {
				columns = indexColumns(row)
			}
			continue
		}
		designator := clean(row[0])
		if designator == "" || designator == designatorHeader {
			continue
		}
		lines = append(lines, Line{
			Designator:   designator,
			Description:  cell(row, columns, headerDescription),
			Fit:          cell(row, columns, headerFit),
			Footprint:    cell(row, columns, headerFootprint),
			Manufacturer: cell(row, columns, headerManufacturer),
			PartNumber:   cell(row, columns, headerManufPartNo),
			Value:        cell(row, columns, headerValue),
			ProjectName:  project,
			PcbRef:       pcbRef,
		})
	}
	if columns == nil {
		return nil, ErrNoDesignator
	}
	return lines, nil
}

func indexColumns(header []string) map[string]int {
	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[name] = i
	}
	return columns
}

func cell(row []string, columns map[string]int, header string) string {
	idx, ok := columns[header]
	if !ok || idx >= len(row) {
		return ""
	}
	return clean(row[idx])
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}
