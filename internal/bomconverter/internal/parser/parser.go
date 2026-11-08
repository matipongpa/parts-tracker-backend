// Package parser reads the cells of an .xlsx/.xlsm workbook.
package parser

import (
	"errors"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

var (
	ErrUnreadable   error = errors.New("parser: file is not a readable Excel workbook")
	ErrUnknownSheet error = errors.New("parser: no such sheet")
)

// Limits on what a workbook may unpack to. The upload is capped at 10 MB, but a crafted
// zip can expand far beyond that.
const (
	maxUnzipBytes    int64 = 256 << 20
	maxUnzipXMLBytes int64 = 128 << 20
)

// Workbook is an opened workbook. Close it when done.
type Workbook struct {
	file *excelize.File
}

func Open(r io.Reader) (*Workbook, error) {
	f, err := excelize.OpenReader(r, excelize.Options{UnzipSizeLimit: maxUnzipBytes, UnzipXMLSizeLimit: maxUnzipXMLBytes})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return &Workbook{file: f}, nil
}

func (w *Workbook) Close() error { return w.file.Close() }

// SheetNames lists the visible sheets in workbook order (all sheets if none is visible).
func (w *Workbook) SheetNames() []string {
	all := w.file.GetSheetList()
	visible := make([]string, 0, len(all))
	for _, name := range all {
		if shown, err := w.file.GetSheetVisible(name); err == nil && shown {
			visible = append(visible, name)
		}
	}
	if len(visible) == 0 {
		return all
	}
	return visible
}

// Rows returns every row of a sheet. Trailing empty cells of a row are omitted.
func (w *Workbook) Rows(sheet string) ([][]string, error) {
	rows, err := w.file.GetRows(sheet)
	if err != nil {
		// excelize reports a missing sheet by name in the error text; any other failure is a bad file.
		for _, name := range w.file.GetSheetList() {
			if name == sheet {
				return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
			}
		}
		return nil, fmt.Errorf("%w: %q", ErrUnknownSheet, sheet)
	}
	return rows, nil
}
