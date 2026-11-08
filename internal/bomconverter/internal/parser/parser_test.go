package parser

import (
	"errors"
	"testing"
)

func sampleRows() [][]string {
	return [][]string{
		{"Project Name :", "Widget"},
		{"PCB Ref:", "PCB-1"},
		{},
		{"Designator", "Description", "Value", "FIT/NFIT", "Manufacturer part no."},
		{"C1", "cap\n100n", "100n", "FIT", "PN-C"},
		{""},
		{"R1,R2", "res", "10k"},
	}
}

func TestParse(t *testing.T) {
	lines, err := Parse(sampleRows())
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	c := lines[0]
	if c.Designator != "C1" || c.Description != "cap 100n" || c.Value != "100n" || c.Fit != "FIT" ||
		c.PartNumber != "PN-C" || c.ProjectName != "Widget" || c.PcbRef != "PCB-1" {
		t.Errorf("line 0 = %+v", c)
	}
	// Short row: missing trailing cells become empty, not a panic.
	if r := lines[1]; r.Designator != "R1,R2" || r.Fit != "" || r.PartNumber != "" {
		t.Errorf("line 1 = %+v", r)
	}
}

func TestParseWithoutHeader(t *testing.T) {
	if _, err := Parse([][]string{{"Project Name :", "X"}, {"C1", "a"}}); !errors.Is(err, ErrNoDesignator) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadFirstSheetRejectsGarbage(t *testing.T) {
	if _, err := ReadFirstSheet(bytesReader("not a workbook")); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}
