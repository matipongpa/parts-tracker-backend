package layout

import (
	"errors"
	"reflect"
	"testing"
)

func legacyRows() [][]string {
	return [][]string{
		{"Project Name :", "Widget"},
		{"PCB Ref:", "PCB-1"},
		{},
		{"Designator", "Description", "Value", "FIT/NFIT", "Manufacturer", "Manufacturer part no."},
		{"C1", "cap\n100n", "100n", "FIT", "Murata", "PN-C"},
		{""},
		{"R1,R2", "res", "10k", "NFIT", "Yageo", "PN-R"},
	}
}

func col(c int) Source { return Source{Kind: KindColumn, Column: c} }

func TestDetectReadsTheCurrentBOMFormat(t *testing.T) {
	rows := legacyRows()
	header, ok := DetectHeader(rows)
	if !ok || header != 3 {
		t.Fatalf("header = %d, %v", header, ok)
	}
	m, conf := Detect(rows, header)
	want := map[string]Source{
		FieldDesignator:   col(0),
		FieldDescription:  col(1),
		FieldValue:        col(2),
		FieldFit:          col(3),
		FieldManufacturer: col(4),
		FieldPartNumber:   col(5),
		FieldProjectName:  {Kind: KindCell, Row: 0, Column: 1},
		FieldPcbRef:       {Kind: KindCell, Row: 1, Column: 1},
	}
	if !reflect.DeepEqual(m.Fields, want) {
		t.Errorf("fields = %+v", m.Fields)
	}
	if conf[FieldPartNumber] != ConfidenceExact || conf[FieldProjectName] != ConfidenceLabel {
		t.Errorf("confidence = %v", conf)
	}
	if missing := Missing(m); !reflect.DeepEqual(missing, []string{}) {
		t.Errorf("missing = %v", missing)
	}
}

func TestApplyReadsParts(t *testing.T) {
	rows := legacyRows()
	m, _ := Detect(rows, 3)
	lines := Apply(rows, m)
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	c := lines[0]
	if c.Designator != "C1" || c.Description != "cap 100n" || c.Value != "100n" || c.Fit != "FIT" ||
		c.PartNumber != "PN-C" || c.Manufacturer != "Murata" || c.ProjectName != "Widget" || c.PcbRef != "PCB-1" {
		t.Errorf("line 0 = %+v", c)
	}
	if lines[1].Designator != "R1,R2" {
		t.Errorf("line 1 = %+v", lines[1])
	}
}

func TestDetectFollowsAChangedFormat(t *testing.T) {
	// Columns moved, renamed and the table starts lower; project and PCB are now columns.
	rows := [][]string{
		{"BOM export"},
		{},
		{"Project", "PCB", "Part No", "Ref Des", "MFR", "Fitted", "Comment", "Package"},
		{"Widget", "PCB-9", "PN-1", "C4", "Murata", "FIT", "1u", "0402"},
	}
	header, ok := DetectHeader(rows)
	if !ok || header != 2 {
		t.Fatalf("header = %d, %v", header, ok)
	}
	m, _ := Detect(rows, header)
	want := map[string]Source{
		FieldProjectName: col(0), FieldPcbRef: col(1), FieldPartNumber: col(2), FieldDesignator: col(3),
		FieldManufacturer: col(4), FieldFit: col(5), FieldValue: col(6), FieldFootprint: col(7),
	}
	if !reflect.DeepEqual(m.Fields, want) {
		t.Errorf("fields = %+v", m.Fields)
	}
	if got := Apply(rows, m); len(got) != 1 || got[0].PartNumber != "PN-1" || got[0].ProjectName != "Widget" {
		t.Errorf("lines = %+v", got)
	}
}

func TestDetectPrefersTheSpecificPartNumberHeader(t *testing.T) {
	rows := [][]string{{"Designator", "Part No", "Manufacturer part no."}}
	m, _ := Detect(rows, 0)
	if m.Fields[FieldPartNumber] != col(2) {
		t.Errorf("part number = %+v", m.Fields[FieldPartNumber])
	}
}

func TestDetectMarksLooseMatchesAsPartial(t *testing.T) {
	rows := [][]string{{"Designator", "Manufacturer part no. (alt)", "Value"}}
	m, conf := Detect(rows, 0)
	if m.Fields[FieldPartNumber] != col(1) || conf[FieldPartNumber] != ConfidencePartial {
		t.Errorf("part number = %+v (%s)", m.Fields[FieldPartNumber], conf[FieldPartNumber])
	}
}

func TestDetectFindsDesignatorsByContent(t *testing.T) {
	rows := [][]string{
		{"Item", "Where", "Value"},
		{"1", "C1", "1u"},
		{"2", "R2", "2k"},
		{"3", "U7", "x"},
		{"4", "L3", "y"},
	}
	m, conf := Detect(rows, 0)
	if m.Fields[FieldDesignator] != col(1) || conf[FieldDesignator] != ConfidenceContent {
		t.Errorf("designator = %+v (%s)", m.Fields[FieldDesignator], conf[FieldDesignator])
	}
}

func TestDetectHeaderNeedsTwoKnownNamesOrADesignator(t *testing.T) {
	if _, ok := DetectHeader([][]string{{"foo", "bar"}, {"C1", "x"}}); ok {
		t.Error("detected a header in unrelated text")
	}
	if row, ok := DetectHeader([][]string{{"title"}, {"Designator", "x"}}); !ok || row != 1 {
		t.Errorf("designator-only header = %d, %v", row, ok)
	}
}

func TestDetectLeavesUnknownFieldsOut(t *testing.T) {
	m, _ := Detect([][]string{{"Designator", "Part No"}}, 0)
	want := []string{FieldFit, FieldManufacturer, FieldValue, FieldProjectName, FieldPcbRef}
	if got := Missing(m); !reflect.DeepEqual(got, want) {
		t.Errorf("missing = %v", got)
	}
}

func TestApplySourcesAndRepeatedHeader(t *testing.T) {
	rows := [][]string{
		{"Designator", "PN", "Fit", "Mfr", "Val"},
		{"C1", "A", "FIT", "M", "1u"},
		{"Designator", "PN", "Fit", "Mfr", "Val"}, // page break repeating the header
		{"", "B", "FIT", "M", "2u"},               // no designator: not a part
		{"R1", "C", "FIT", "M", "3u"},
	}
	m := Mapping{HeaderRow: 0, Fields: map[string]Source{
		FieldDesignator: col(0), FieldPartNumber: col(1), FieldFit: col(2), FieldManufacturer: col(3), FieldValue: col(4),
		FieldProjectName: {Kind: KindText, Text: "Typed project"},
		FieldPcbRef:      {Kind: KindCell, Row: 1, Column: 1},
	}}
	lines := Apply(rows, m)
	if len(lines) != 2 || lines[0].Designator != "C1" || lines[1].Designator != "R1" {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].ProjectName != "Typed project" || lines[1].PcbRef != "A" {
		t.Errorf("constants = %+v %+v", lines[0], lines[1])
	}
}

func TestApplyToleratesShortRowsAndBadHeaderRow(t *testing.T) {
	rows := [][]string{{"Designator", "PN"}, {"C1"}}
	m := Mapping{HeaderRow: 0, Fields: map[string]Source{FieldDesignator: col(0), FieldPartNumber: col(1), FieldValue: col(9)}}
	if got := Apply(rows, m); len(got) != 1 || got[0].PartNumber != "" || got[0].Value != "" {
		t.Errorf("lines = %+v", got)
	}
	m.HeaderRow = 50
	if got := Apply(rows, m); got != nil {
		t.Errorf("lines = %+v", got)
	}
}

func completeMapping() Mapping {
	fields := map[string]Source{}
	for i, f := range Required {
		fields[f] = col(i)
	}
	return Mapping{Fields: fields}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(*Mapping)
		missing []string
		problem bool
	}{
		{"complete", func(*Mapping) {}, nil, false},
		{"missing required", func(m *Mapping) { delete(m.Fields, FieldFit); delete(m.Fields, FieldValue) }, []string{FieldFit, FieldValue}, false},
		{"description and footprint are optional", func(m *Mapping) { m.Fields[FieldDescription] = col(20) }, nil, false},
		{"unknown field", func(m *Mapping) { m.Fields["colour"] = col(1) }, nil, true},
		{"unknown kind", func(m *Mapping) { m.Fields[FieldFit] = Source{Kind: "formula"} }, nil, true},
		{"column out of range", func(m *Mapping) { m.Fields[FieldFit] = col(9999) }, nil, true},
		{"negative column", func(m *Mapping) { m.Fields[FieldFit] = col(-1) }, nil, true},
		{"cell out of range", func(m *Mapping) { m.Fields[FieldPcbRef] = Source{Kind: KindCell, Row: -1} }, nil, true},
		{"blank text", func(m *Mapping) { m.Fields[FieldPcbRef] = Source{Kind: KindText, Text: "  "} }, nil, true},
		{"designator must be a column", func(m *Mapping) { m.Fields[FieldDesignator] = Source{Kind: KindText, Text: "C1"} }, nil, true},
		{"negative header row", func(m *Mapping) { m.HeaderRow = -1 }, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := completeMapping()
			tc.edit(&m)
			err := Validate(m)
			var me *MappingError
			switch {
			case tc.missing == nil && !tc.problem:
				if err != nil {
					t.Fatalf("err = %v", err)
				}
			case !errors.As(err, &me):
				t.Fatalf("err = %v, want MappingError", err)
			case tc.problem && me.Problem == "":
				t.Errorf("want a problem, got %+v", me)
			case !tc.problem && !reflect.DeepEqual(me.Missing, tc.missing):
				t.Errorf("missing = %v, want %v", me.Missing, tc.missing)
			}
		})
	}
}

func TestFingerprint(t *testing.T) {
	a := Fingerprint([]string{"Designator", "Manufacturer part no.", "", "Value"})
	if a == "" || a != Fingerprint([]string{" DESIGNATOR ", "manufacturer part no", "", "value", ""}) {
		t.Error("case, punctuation and trailing blanks must not change the fingerprint")
	}
	if a == Fingerprint([]string{"Manufacturer part no.", "Designator", "", "Value"}) {
		t.Error("moving a column must change the fingerprint")
	}
	if a == Fingerprint([]string{"Designator", "Part no", "", "Value"}) {
		t.Error("renaming a column must change the fingerprint")
	}
	if Fingerprint([]string{"", " "}) != "" {
		t.Error("an empty header has no fingerprint")
	}
}

func TestResolve(t *testing.T) {
	rows := [][]string{{"Project", "x"}, {"Designator", "Project"}, {"C1", ""}, {"C2", "Widget"}}
	m := Mapping{HeaderRow: 1, Fields: map[string]Source{
		FieldProjectName: col(1), FieldPcbRef: {Kind: KindCell, Row: 0, Column: 1}, FieldFit: {Kind: KindText, Text: "FIT"},
	}}
	if got := Resolve(rows, m, FieldProjectName); got != "Widget" {
		t.Errorf("column = %q (first non-empty value)", got)
	}
	if got := Resolve(rows, m, FieldPcbRef); got != "x" {
		t.Errorf("cell = %q", got)
	}
	if got := Resolve(rows, m, FieldFit); got != "FIT" {
		t.Errorf("text = %q", got)
	}
	if got := Resolve(rows, m, FieldValue); got != "" {
		t.Errorf("unmapped = %q", got)
	}
}

func TestReusableDropsTypedProjectAndPcbButKeepsTheRest(t *testing.T) {
	m := Mapping{HeaderRow: 2, Fields: map[string]Source{
		FieldDesignator:   col(0),
		FieldProjectName:  {Kind: KindText, Text: "Gamma"},
		FieldPcbRef:       {Kind: KindText, Text: "G-300"},
		FieldFit:          {Kind: KindText, Text: "FIT"},
		FieldManufacturer: {Kind: KindCell, Row: 0, Column: 1},
	}}
	got := Reusable(m)
	want := map[string]Source{FieldDesignator: col(0), FieldFit: {Kind: KindText, Text: "FIT"}, FieldManufacturer: {Kind: KindCell, Row: 0, Column: 1}}
	if got.HeaderRow != 2 || !reflect.DeepEqual(got.Fields, want) {
		t.Errorf("reusable = %+v", got)
	}
	if len(m.Fields) != 5 {
		t.Error("the original mapping was modified")
	}
	// A project name read from a column or a labelled cell is a layout fact, so it stays.
	m.Fields[FieldProjectName] = Source{Kind: KindCell, Row: 0, Column: 1}
	if _, ok := Reusable(m).Fields[FieldProjectName]; !ok {
		t.Error("a cell-based project name must be kept")
	}
}
