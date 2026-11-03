package bomconverter

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"partlist/internal/parts"
)

type fakeParts struct {
	parts.Service
	stored []parts.NewPart
	err    error
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
	return len(ps), f.err
}

func workbook(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	for i, row := range rows {
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellValue("Sheet1", cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sampleBOM(t *testing.T) []byte {
	return workbook(t, [][]string{
		{"Project Name :", "Widget"},
		{"PCB Ref:", "PCB-1"},
		{"Designator", "Description", "Value", "FIT/NFIT", "Manufacturer", "Manufacturer part no."},
		{"C1", "cap", "100n", "FIT", "Murata", "PN-C"},
		{"R1", "res", "10k", "", "Yageo", "PN-R"},   // missing Fit -> skipped
		{"ZZ1", "mystery", "1", "FIT", "X", "PN-Z"}, // unknown designator -> skipped
	})
}

func TestImportStoresOnlyValidRows(t *testing.T) {
	fake := &fakeParts{}
	res, err := New(fake).Import(context.Background(), bytes.NewReader(sampleBOM(t)))
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
	_, err := New(&fakeParts{}).Import(context.Background(), strings.NewReader("junk"))
	if !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("err = %v", err)
	}
}

func upload(t *testing.T, svc Service, field string, name string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), svc)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/bom/import", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHTTPImport(t *testing.T) {
	svc := New(&fakeParts{})
	cases := []struct {
		name    string
		field   string
		file    string
		content []byte
		want    int
	}{
		{"ok", "file", "bom.xlsm", sampleBOM(t), http.StatusOK},
		{"wrong field", "doc", "bom.xlsm", sampleBOM(t), http.StatusBadRequest},
		{"wrong extension", "file", "bom.pdf", sampleBOM(t), http.StatusBadRequest},
		{"not a workbook", "file", "bom.xlsm", []byte("junk"), http.StatusUnprocessableEntity},
		{"too large", "file", "bom.xlsm", make([]byte, maxUploadBytes+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := upload(t, svc, tc.field, tc.file, tc.content)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestHTTPImportStoreFailureIs500(t *testing.T) {
	rec := upload(t, New(&fakeParts{err: errors.New("db down")}), "file", "bom.xlsm", sampleBOM(t))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
}
