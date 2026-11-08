package bomconverter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"partlist/internal/bomconverter/internal/layout"
	"partlist/internal/parts"
)

type fakeParts struct {
	parts.Service
	stored []parts.NewPart
	err    error
	// existing makes BulkCreate report fewer inserts, as the real store does for duplicates.
	existing int
}

func (f *fakeParts) ClassifyDesignator(d string) parts.PartType {
	switch {
	case strings.HasPrefix(d, "C"):
		return parts.PartTypeCapacitor
	case strings.HasPrefix(d, "R"):
		return parts.PartTypeResistor
	}
	return parts.PartTypeUnknown
}

func (f *fakeParts) BulkCreate(_ context.Context, ps []parts.NewPart) (int, error) {
	f.stored = ps
	return len(ps) - f.existing, f.err
}

type fakeStore struct {
	saved     map[string]layout.Mapping
	savedBy   string
	findErr   error
	saveErr   error
	saveCalls int
}

func newFakeStore() *fakeStore { return &fakeStore{saved: map[string]layout.Mapping{}} }

func (f *fakeStore) Find(_ context.Context, fp string) (layout.Mapping, bool, error) {
	m, ok := f.saved[fp]
	return m, ok, f.findErr
}

func (f *fakeStore) Save(_ context.Context, fp string, m layout.Mapping, user string) error {
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved[fp], f.savedBy = m, user
	return nil
}

// sheets maps a sheet name to its rows; the first listed name is the first sheet.
type sheet struct {
	name string
	rows [][]string
}

func workbook(t *testing.T, sheets ...sheet) []byte {
	t.Helper()
	f := excelize.NewFile()
	for i, s := range sheets {
		if i == 0 {
			if err := f.SetSheetName("Sheet1", s.name); err != nil {
				t.Fatal(err)
			}
		} else if _, err := f.NewSheet(s.name); err != nil {
			t.Fatal(err)
		}
		for r, row := range s.rows {
			for c, v := range row {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
				if err := f.SetCellValue(s.name, cell, v); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sampleRows() [][]string {
	return [][]string{
		{"Project Name :", "Widget"},
		{"PCB Ref:", "PCB-1"},
		{"Designator", "Description", "Value", "FIT/NFIT", "Manufacturer", "Manufacturer part no."},
		{"C1", "cap", "100n", "FIT", "Murata", "PN-C"},
		{"R1", "res", "10k", "", "Yageo", "PN-R"},   // missing Fit -> skipped
		{"ZZ1", "mystery", "1", "FIT", "X", "PN-Z"}, // unknown designator -> skipped
	}
}

func sampleBOM(t *testing.T) []byte { return workbook(t, sheet{"Sheet1", sampleRows()}) }

func TestImportStoresOnlyValidRows(t *testing.T) {
	fake := &fakeParts{}
	res, err := newService(fake, newFakeStore()).Import(context.Background(), bytes.NewReader(sampleBOM(t)), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ImportResult{Total: 3, Inserted: 1, Skipped: 2}) {
		t.Errorf("result = %+v", res)
	}
	if len(fake.stored) != 1 || fake.stored[0].Type != "capacitor" || fake.stored[0].ProjectName != "Widget" || fake.stored[0].PcbRef != "PCB-1" {
		t.Errorf("stored = %+v", fake.stored)
	}
}

func TestImportInvalidFile(t *testing.T) {
	_, err := newService(&fakeParts{}, newFakeStore()).Import(context.Background(), strings.NewReader("junk"), ImportOptions{})
	if !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("err = %v", err)
	}
}

func TestImportReportsDuplicates(t *testing.T) {
	res, err := newService(&fakeParts{existing: 1}, newFakeStore()).Import(context.Background(), bytes.NewReader(sampleBOM(t)), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ImportResult{Total: 3, Inserted: 0, Skipped: 2, Duplicates: 1}) {
		t.Errorf("result = %+v", res)
	}
}

// changedRows is the same data in a different layout: other headers, other order, project and PCB as columns.
func changedRows() [][]string {
	return [][]string{
		{"Part No", "Ref Des", "Project", "PCB", "MFR", "Fitted", "Comment"},
		{"PN-C", "C1", "Gadget", "PCB-2", "Murata", "FIT", "100n"},
	}
}

func TestImportReadsAChangedFormatWithoutAnyMapping(t *testing.T) {
	fake := &fakeParts{}
	_, err := newService(fake, newFakeStore()).Import(context.Background(), bytes.NewReader(workbook(t, sheet{"Data", changedRows()})), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.stored) != 1 || fake.stored[0].PartNumber != "PN-C" || fake.stored[0].ProjectName != "Gadget" || fake.stored[0].Value != "100n" {
		t.Errorf("stored = %+v", fake.stored)
	}
}

func TestImportRefusesAnIncompleteMapping(t *testing.T) {
	file := workbook(t, sheet{"Sheet1", [][]string{{"Designator", "Part No"}, {"C1", "PN"}}})
	fake := &fakeParts{}
	_, err := newService(fake, newFakeStore()).Import(context.Background(), bytes.NewReader(file), ImportOptions{})
	var me *MappingError
	if !errors.As(err, &me) || len(me.Missing) == 0 {
		t.Fatalf("err = %v", err)
	}
	if fake.stored != nil {
		t.Errorf("stored %+v despite the error", fake.stored)
	}
}

func completeMapping() Mapping {
	col := func(c int) Source { return Source{Kind: layout.KindColumn, Column: c} }
	return Mapping{HeaderRow: 0, Fields: map[string]Source{
		layout.FieldPartNumber: col(0), layout.FieldDesignator: col(1), layout.FieldProjectName: {Kind: layout.KindText, Text: "Typed"},
		layout.FieldPcbRef: col(3), layout.FieldManufacturer: col(4), layout.FieldFit: col(5), layout.FieldValue: col(6),
	}}
}

func TestImportUsesTheClientMappingAndSavesItForTheNextFile(t *testing.T) {
	store := newFakeStore()
	svc := newService(&fakeParts{}, store)
	file := workbook(t, sheet{"Data", changedRows()})
	mapping := completeMapping()

	res, err := svc.Import(context.Background(), bytes.NewReader(file), ImportOptions{Mapping: &mapping, SaveMapping: true, User: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.MappingSaved || store.savedBy != "alice" || len(store.saved) != 1 {
		t.Fatalf("result = %+v, saved = %v by %q", res, store.saved, store.savedBy)
	}
	for _, saved := range store.saved {
		// "Typed" was this file's project name; it must not be stamped on the next file.
		if _, ok := saved.Fields[layout.FieldProjectName]; ok {
			t.Errorf("a typed project name was saved: %+v", saved.Fields[layout.FieldProjectName])
		}
	}

	// The next upload with the same header row is pre-filled with what alice confirmed, not with detection.
	preview, err := svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.MappingSource != SourceSaved || preview.Mapping.Fields[layout.FieldPartNumber].Column != 0 {
		t.Errorf("preview mapping = %s %+v", preview.MappingSource, preview.Mapping.Fields[layout.FieldPartNumber])
	}
	// The project name was not saved, so it is asked for again.
	if len(preview.Missing) != 1 || preview.Missing[0] != layout.FieldProjectName {
		t.Errorf("missing = %v", preview.Missing)
	}
	if preview.Confidence[layout.FieldFit] != layout.ConfidenceSaved {
		t.Errorf("confidence = %v", preview.Confidence)
	}
}

func TestImportDoesNotSaveUnlessAsked(t *testing.T) {
	store := newFakeStore()
	mapping := completeMapping()
	res, err := newService(&fakeParts{}, store).Import(context.Background(), bytes.NewReader(workbook(t, sheet{"Data", changedRows()})),
		ImportOptions{Mapping: &mapping})
	if err != nil || res.MappingSaved || store.saveCalls != 0 {
		t.Fatalf("res = %+v, err = %v, saves = %d", res, err, store.saveCalls)
	}
}

func TestImportSucceedsEvenWhenSavingTheMappingFails(t *testing.T) {
	store := newFakeStore()
	store.saveErr = errors.New("db down")
	mapping := completeMapping()
	res, err := newService(&fakeParts{}, store).Import(context.Background(), bytes.NewReader(workbook(t, sheet{"Data", changedRows()})),
		ImportOptions{Mapping: &mapping, SaveMapping: true})
	if err != nil || res.Inserted != 1 || res.MappingSaved {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestImportRejectsAMalformedMapping(t *testing.T) {
	mapping := completeMapping()
	mapping.Fields[layout.FieldFit] = Source{Kind: layout.KindColumn, Column: 100000}
	_, err := newService(&fakeParts{}, newFakeStore()).Import(context.Background(), bytes.NewReader(workbook(t, sheet{"Data", changedRows()})),
		ImportOptions{Mapping: &mapping})
	var me *MappingError
	if !errors.As(err, &me) || me.Problem == "" {
		t.Fatalf("err = %v", err)
	}
}

func TestPreviewProposesAMappingAndCountsRows(t *testing.T) {
	p, err := newService(&fakeParts{}, newFakeStore()).Preview(context.Background(), bytes.NewReader(sampleBOM(t)), PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.HeaderDetected || p.HeaderRow != 2 || p.MappingSource != SourceDetected || p.TotalRows != 6 {
		t.Fatalf("preview = %+v", p)
	}
	if len(p.Missing) != 0 || p.Resolved[layout.FieldProjectName] != "Widget" || p.Resolved[layout.FieldPcbRef] != "PCB-1" {
		t.Errorf("missing = %v resolved = %v", p.Missing, p.Resolved)
	}
	if p.Summary != (Summary{Rows: 3, Valid: 1, Skipped: 2}) {
		t.Errorf("summary = %+v", p.Summary)
	}
	if len(p.Rows) != 6 || p.Columns != 6 || len(p.Rows[0]) != 6 || p.Rows[0][0] != "Project Name :" {
		t.Errorf("grid = %d rows x %d, first = %v", len(p.Rows), p.Columns, p.Rows[0])
	}
}

func TestPreviewHonoursAHeaderRowOverride(t *testing.T) {
	row := 0
	p, err := newService(&fakeParts{}, newFakeStore()).Preview(context.Background(), bytes.NewReader(sampleBOM(t)), PreviewOptions{HeaderRow: &row})
	if err != nil {
		t.Fatal(err)
	}
	// Row 0 is "Project Name :, Widget": it names a project column and no part columns, so the rest is missing.
	if p.HeaderRow != 0 || len(p.Missing) != len(layout.Required)-1 || p.Summary != (Summary{}) {
		t.Errorf("preview = header %d, missing %v, summary %+v", p.HeaderRow, p.Missing, p.Summary)
	}
}

func TestPreviewDryRunsTheClientMapping(t *testing.T) {
	svc := newService(&fakeParts{}, newFakeStore())
	file := workbook(t, sheet{"Data", changedRows()})
	mapping := completeMapping()

	p, err := svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{Mapping: &mapping})
	if err != nil {
		t.Fatal(err)
	}
	if p.MappingSource != SourceGiven || len(p.Missing) != 0 || p.Summary != (Summary{Rows: 1, Valid: 1}) || p.Resolved[layout.FieldProjectName] != "Typed" {
		t.Errorf("preview = %+v", p)
	}

	delete(mapping.Fields, layout.FieldValue)
	p, err = svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{Mapping: &mapping})
	if err != nil || len(p.Missing) != 1 || p.Missing[0] != layout.FieldValue || p.Summary != (Summary{}) {
		t.Errorf("unfinished: %+v, %v", p, err)
	}

	mapping.Fields[layout.FieldValue] = Source{Kind: layout.KindColumn, Column: -3}
	var me *MappingError
	if _, err := svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{Mapping: &mapping}); !errors.As(err, &me) {
		t.Errorf("malformed: err = %v", err)
	}
}

func TestPreviewPicksTheSheetThatLooksLikeABOMAndListsAll(t *testing.T) {
	file := workbook(t, sheet{"Notes", [][]string{{"nothing here"}}}, sheet{"BOM", sampleRows()})
	svc := newService(&fakeParts{}, newFakeStore())

	p, err := svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Sheet != "BOM" || len(p.Sheets) != 2 {
		t.Errorf("sheet = %q of %v", p.Sheet, p.Sheets)
	}

	p, err = svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{Sheet: "Notes"})
	if err != nil || p.Sheet != "Notes" || p.HeaderDetected {
		t.Errorf("explicit sheet: %+v, %v", p, err)
	}
	if _, err := svc.Preview(context.Background(), bytes.NewReader(file), PreviewOptions{Sheet: "Nope"}); !errors.Is(err, ErrUnknownSheet) {
		t.Errorf("unknown sheet err = %v", err)
	}
}

func TestPreviewShortensLongCells(t *testing.T) {
	long := strings.Repeat("x", 500)
	file := workbook(t, sheet{"Sheet1", [][]string{{"Designator", "Part No"}, {"C1", long}}})
	p, err := newService(&fakeParts{}, newFakeStore()).Preview(context.Background(), bytes.NewReader(file), PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := []rune(p.Rows[1][1]); len(got) != previewMaxCellRune+1 {
		t.Errorf("cell has %d runes", len(got))
	}
}

func post(t *testing.T, svc Service, path string, fields map[string]string, name string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), svc, func(*gin.Context) string { return "tester" })

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if name != "" {
		part, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(content)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHTTPUploadValidation(t *testing.T) {
	svc := newService(&fakeParts{}, newFakeStore())
	cases := []struct {
		name    string
		fields  map[string]string
		file    string
		content []byte
		want    int
	}{
		{"ok", nil, "bom.xlsm", sampleBOM(t), http.StatusOK},
		{"no file", nil, "", nil, http.StatusBadRequest},
		{"wrong extension", nil, "bom.pdf", sampleBOM(t), http.StatusBadRequest},
		{"not a workbook", nil, "bom.xlsm", []byte("junk"), http.StatusUnprocessableEntity},
		{"too large", nil, "bom.xlsm", make([]byte, maxUploadBytes+1), http.StatusRequestEntityTooLarge},
		{"mapping is not json", map[string]string{"mapping": "{"}, "bom.xlsm", sampleBOM(t), http.StatusBadRequest},
		{"mapping with an unknown key", map[string]string{"mapping": `{"header_row":0,"fields":{},"extra":1}`}, "bom.xlsm", sampleBOM(t), http.StatusBadRequest},
		{"mapping missing fields", map[string]string{"mapping": `{"header_row":2,"fields":{}}`}, "bom.xlsm", sampleBOM(t), http.StatusUnprocessableEntity},
		{"unknown sheet", map[string]string{"sheet": "Nope"}, "bom.xlsm", sampleBOM(t), http.StatusBadRequest},
	}
	for _, path := range []string{"/api/bom/import", "/api/bom/preview"} {
		for _, tc := range cases {
			want := tc.want
			// A preview may be shown an unfinished mapping; only an import insists it is complete.
			if path == "/api/bom/preview" && tc.name == "mapping missing fields" {
				want = http.StatusOK
			}
			t.Run(path+" "+tc.name, func(t *testing.T) {
				rec := post(t, svc, path, tc.fields, tc.file, tc.content)
				if rec.Code != want {
					t.Fatalf("status = %d, want %d, body = %s", rec.Code, want, rec.Body)
				}
			})
		}
	}
}

func TestHTTPPreviewReturnsTheProposal(t *testing.T) {
	rec := post(t, newService(&fakeParts{}, newFakeStore()), "/api/bom/preview", map[string]string{"header_row": "2"}, "bom.xlsx", sampleBOM(t))
	var got Preview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status = %d, err = %v, body = %s", rec.Code, err, rec.Body)
	}
	if got.HeaderRow != 2 || got.Mapping.Fields[layout.FieldPartNumber].Column != 5 {
		t.Errorf("preview = %+v", got)
	}
	for _, bad := range []string{"-1", "abc"} {
		if rec := post(t, newService(&fakeParts{}, newFakeStore()), "/api/bom/preview", map[string]string{"header_row": bad}, "bom.xlsx", sampleBOM(t)); rec.Code != http.StatusBadRequest {
			t.Errorf("header_row %q: status = %d", bad, rec.Code)
		}
	}
}

func TestHTTPImportWithMappingSavesAsTheCaller(t *testing.T) {
	store := newFakeStore()
	mapping, _ := json.Marshal(completeMapping())
	rec := post(t, newService(&fakeParts{}, store), "/api/bom/import",
		map[string]string{"mapping": string(mapping), "save_mapping": "true"}, "bom.xlsx", workbook(t, sheet{"Data", changedRows()}))
	if rec.Code != http.StatusOK || store.savedBy != "tester" {
		t.Fatalf("status = %d, savedBy = %q, body = %s", rec.Code, store.savedBy, rec.Body)
	}
	var res ImportResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.MappingSaved || res.Inserted != 1 {
		t.Errorf("result = %+v, err = %v", res, err)
	}
}

func TestHTTPMissingFieldsAreListedForTheClient(t *testing.T) {
	file := workbook(t, sheet{"Sheet1", [][]string{{"Designator", "Part No"}, {"C1", "PN"}}})
	rec := post(t, newService(&fakeParts{}, newFakeStore()), "/api/bom/import", nil, "bom.xlsx", file)
	var body struct {
		Error   string   `json:"error"`
		Missing []string `json:"missing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, err = %v", rec.Code, err)
	}
	if len(body.Missing) == 0 || !strings.Contains(body.Error, "missing") {
		t.Errorf("body = %+v", body)
	}
}

func TestHTTPStoreFailureIs500AndHidesTheCause(t *testing.T) {
	rec := post(t, newService(&fakeParts{err: errors.New("db down")}, newFakeStore()), "/api/bom/import", nil, "bom.xlsm", sampleBOM(t))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
}

func TestPreviewStoreFailureIs500(t *testing.T) {
	store := newFakeStore()
	store.findErr = errors.New("db down")
	rec := post(t, newService(&fakeParts{}, store), "/api/bom/preview", nil, "bom.xlsm", sampleBOM(t))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
}
