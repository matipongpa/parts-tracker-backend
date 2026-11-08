package parser

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func twoSheets(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "a")
	f.SetCellValue("Sheet1", "B2", "b")
	f.NewSheet("Hidden")
	f.SetSheetVisible("Hidden", false)
	f.NewSheet("Second")
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOpenRejectsGarbage(t *testing.T) {
	if _, err := Open(strings.NewReader("not a workbook")); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSheetNamesSkipHiddenSheets(t *testing.T) {
	wb, err := Open(bytes.NewReader(twoSheets(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if got := strings.Join(wb.SheetNames(), ","); got != "Sheet1,Second" {
		t.Errorf("names = %s", got)
	}
}

func TestRows(t *testing.T) {
	wb, err := Open(bytes.NewReader(twoSheets(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	rows, err := wb.Rows("Sheet1")
	if err != nil || len(rows) != 2 || rows[1][1] != "b" {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
	if _, err := wb.Rows("Nope"); !errors.Is(err, ErrUnknownSheet) {
		t.Errorf("err = %v", err)
	}
}
