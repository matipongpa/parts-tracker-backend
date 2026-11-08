package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
)

// Confidence says how a field's source was found.
const (
	ConfidenceExact   string = "exact"   // the header is a known name for the field
	ConfidencePartial string = "partial" // the header contains a known name; worth a second look
	ConfidenceContent string = "content" // guessed from the cells below the header
	ConfidenceLabel   string = "label"   // the cell next to a label such as "Project Name :"
	ConfidenceSaved   string = "saved"   // a mapping the team confirmed for this header row
)

// synonyms are normalised header names, most specific first: when two columns match one field,
// the earlier synonym wins.
var synonyms map[string][]string = map[string][]string{
	FieldDesignator: {
		"designator", "designators", "referencedesignator", "refdes", "reference", "references", "ref", "refs",
		"partreference", "location", "locations",
	},
	FieldPartNumber: {
		"manufacturerpartno", "manufacturerpartnumber", "manufacturerpartnum", "manufacturerpn", "mfrpartno",
		"mfrpartnumber", "mfrpn", "mfgpartno", "mfgpartnumber", "mfgpn", "mpn", "partnumber", "partno", "partnum", "pn",
	},
	FieldDescription:  {"description", "partdescription", "desc"},
	FieldFootprint:    {"footprint", "footprintname", "package", "packagecase", "pcbfootprint", "pattern"},
	FieldFit:          {"fitnfit", "fitnotfit", "fittednotfitted", "fitted", "populated", "populate", "fit", "nfit"},
	FieldManufacturer: {"manufacturer", "manufacturername", "mfr", "mfg", "maker", "brand"},
	FieldValue:        {"value", "componentvalue", "val", "comment"},
	FieldProjectName:  {"projectname", "project", "projecttitle"},
	FieldPcbRef:       {"pcbref", "pcbreference", "pcbname", "pcbcode", "pcbnumber", "pcbno", "pcb", "boardref"},
}

// stems are looser patterns for the second pass, for headers like "Manufacturer part no. (alt)".
// They are long enough not to match unrelated words.
var stems map[string][]string = map[string][]string{
	FieldDesignator:   {"designator", "refdes"},
	FieldPartNumber:   {"partnumber", "partno", "mpn"},
	FieldDescription:  {"description"},
	FieldFootprint:    {"footprint"},
	FieldFit:          {"fitted", "populat"},
	FieldManufacturer: {"manufacturer"},
	FieldValue:        {"value"},
	FieldProjectName:  {"projectname"},
	FieldPcbRef:       {"pcbref"},
}

// rowFields are the fields a table row has. Project and PCB are usually above the table, so they do not make a header.
var rowFields []string = Fields[:7]

var designatorPattern *regexp.Regexp = regexp.MustCompile(`^[A-Za-z]{1,4}\d+`)

// DetectHeader finds the header row: the first row (within the top of the sheet) that names the
// most part fields. A row needs at least two of them. As a fallback, a row that only has a
// designator header is accepted, so a minimal BOM still works.
func DetectHeader(rows [][]string) (int, bool) {
	bestRow, bestScore := -1, 1
	fallback := -1
	for i := 0; i < len(rows) && i < headerSearchRows; i++ {
		matched := map[string]bool{}
		for _, cell := range rows[i] {
			n := normalise(cell)
			for _, field := range rowFields {
				if indexOf(synonyms[field], n) >= 0 {
					matched[field] = true
				}
			}
		}
		if len(matched) > bestScore {
			bestRow, bestScore = i, len(matched)
		}
		if fallback < 0 && matched[FieldDesignator] {
			fallback = i
		}
	}
	if bestRow >= 0 {
		return bestRow, true
	}
	if fallback >= 0 {
		return fallback, true
	}
	return 0, false
}

// Detect guesses a mapping for the given header row. Fields it cannot place are left out, so
// Missing(mapping) says what the user still has to supply. The confidence map has one entry per mapped field.
func Detect(rows [][]string, headerRow int) (Mapping, map[string]string) {
	m := Mapping{HeaderRow: headerRow, Fields: map[string]Source{}}
	confidence := map[string]string{}
	if headerRow < 0 || headerRow >= len(rows) {
		return m, confidence
	}
	normalised := make([]string, len(rows[headerRow]))
	for i, cell := range rows[headerRow] {
		normalised[i] = normalise(cell)
	}
	used := map[int]bool{}
	assign := func(field string, column int, how string) {
		m.Fields[field] = Source{Kind: KindColumn, Column: column}
		confidence[field] = how
		used[column] = true
	}

	for _, field := range Fields {
		if column := bestExact(normalised, used, synonyms[field]); column >= 0 {
			assign(field, column, ConfidenceExact)
		}
	}
	for _, field := range Fields {
		if _, done := m.Fields[field]; done {
			continue
		}
		if column := bestPartial(normalised, used, stems[field]); column >= 0 {
			assign(field, column, ConfidencePartial)
		}
	}
	if _, done := m.Fields[FieldDesignator]; !done {
		if column := designatorByContent(rows, headerRow, used, len(normalised)); column >= 0 {
			assign(FieldDesignator, column, ConfidenceContent)
		}
	}
	for _, field := range []string{FieldProjectName, FieldPcbRef} {
		if _, done := m.Fields[field]; done {
			continue
		}
		if src, ok := findLabelledCell(rows, headerRow, synonyms[field]); ok {
			m.Fields[field] = src
			confidence[field] = ConfidenceLabel
		}
	}
	return m, confidence
}

func bestExact(normalised []string, used map[int]bool, names []string) int {
	bestColumn, bestRank := -1, len(names)
	for column, n := range normalised {
		if used[column] || n == "" {
			continue
		}
		if rank := indexOf(names, n); rank >= 0 && rank < bestRank {
			bestColumn, bestRank = column, rank
		}
	}
	return bestColumn
}

func bestPartial(normalised []string, used map[int]bool, stems []string) int {
	for _, stem := range stems {
		for column, n := range normalised {
			if !used[column] && n != "" && strings.Contains(n, stem) {
				return column
			}
		}
	}
	return -1
}

// designatorByContent picks the column whose cells mostly look like reference designators.
func designatorByContent(rows [][]string, headerRow int, used map[int]bool, width int) int {
	bestColumn, bestShare := -1, minDesignatorShare
	for column := 0; column < width; column++ {
		if used[column] {
			continue
		}
		filled, hits := 0, 0
		for i := headerRow + 1; i < len(rows) && i <= headerRow+contentSampleRows; i++ {
			cell := Clean(cellAt(rows, i, column))
			if cell == "" {
				continue
			}
			filled++
			if designatorPattern.MatchString(cell) {
				hits++
			}
		}
		if filled < 3 {
			continue
		}
		if share := float64(hits) / float64(filled); share >= bestShare {
			bestColumn, bestShare = column, share
		}
	}
	return bestColumn
}

// findLabelledCell looks above the header for a label cell ("Project Name :") and returns the
// first non-empty cell to its right.
func findLabelledCell(rows [][]string, headerRow int, labels []string) (Source, bool) {
	for i := 0; i < headerRow && i < len(rows); i++ {
		for j, cell := range rows[i] {
			if indexOf(labels, normalise(cell)) < 0 {
				continue
			}
			for k := j + 1; k < len(rows[i]); k++ {
				if Clean(rows[i][k]) != "" {
					return Source{Kind: KindCell, Row: i, Column: k}, true
				}
			}
		}
	}
	return Source{}, false
}

func indexOf(list []string, item string) int {
	if item == "" {
		return -1
	}
	for i, candidate := range list {
		if candidate == item {
			return i
		}
	}
	return -1
}

// Fingerprint identifies a header row, so a mapping confirmed once can be found again when a file
// with the same columns arrives. Column positions are part of it: moving or renaming a column
// changes the fingerprint, and the user is asked again. Empty when the row has no text.
func Fingerprint(header []string) string {
	parts := make([]string, 0, len(header))
	for i, cell := range header {
		if n := normalise(cell); n != "" {
			parts = append(parts, strconv.Itoa(i)+":"+n)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}
