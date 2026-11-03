package parts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubService struct {
	getErr    error
	createErr error
	lastQuery SearchQuery
}

func (s *stubService) Get(context.Context, string) (Part, error) { return Part{ID: "1"}, s.getErr }
func (s *stubService) Search(_ context.Context, q SearchQuery) (SearchResult, error) {
	s.lastQuery = q
	return SearchResult{List: []Part{}, Count: 0}, nil
}
func (s *stubService) Create(context.Context, NewPart) (Part, error) {
	return Part{ID: "1"}, s.createErr
}
func (s *stubService) Update(context.Context, string, PartPatch) (Part, error) {
	return Part{ID: "1"}, s.getErr
}
func (s *stubService) Delete(context.Context, string) error               { return s.getErr }
func (s *stubService) BulkCreate(context.Context, []NewPart) (int, error) { return 0, nil }
func (s *stubService) ClassifyDesignator(string) PartType                 { return PartTypeUnknown }

func do(svc Service, method string, path string, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), svc)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		svc    *stubService
		method string
		path   string
		body   string
		want   int
	}{
		{"get ok", &stubService{}, http.MethodGet, "/api/part/1", "", http.StatusOK},
		{"get not found", &stubService{getErr: ErrNotFound}, http.MethodGet, "/api/part/1", "", http.StatusNotFound},
		{"get internal hides error", &stubService{getErr: errors.New("boom")}, http.MethodGet, "/api/part/1", "", http.StatusInternalServerError},
		{"create bad json is 400", &stubService{}, http.MethodPost, "/api/part", "{", http.StatusBadRequest},
		{"create missing fields is 400", &stubService{createErr: ErrInvalidInput}, http.MethodPost, "/api/part", "{}", http.StatusBadRequest},
		{"create duplicate is 409", &stubService{createErr: ErrDuplicate}, http.MethodPost, "/api/part", "{}", http.StatusConflict},
		{"create ok", &stubService{}, http.MethodPost, "/api/part", "{}", http.StatusOK},
		{"update bad json is 400", &stubService{}, http.MethodPut, "/api/part/1", "{", http.StatusBadRequest},
		{"update not found", &stubService{getErr: ErrNotFound}, http.MethodPut, "/api/part/1", "{}", http.StatusNotFound},
		{"delete not found", &stubService{getErr: ErrNotFound}, http.MethodDelete, "/api/part/9", "", http.StatusNotFound},
		{"delete ok", &stubService{}, http.MethodDelete, "/api/part/9", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(tc.svc, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body)
			}
			if strings.Contains(rec.Body.String(), "boom") {
				t.Error("internal error text leaked")
			}
		})
	}
}

func TestSearchParsesQuery(t *testing.T) {
	svc := &stubService{}
	rec := do(svc, http.MethodGet, "/api/part?pcb_ref=C1&type=cap&page=2&page_size=abc", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	q := svc.lastQuery
	if q.PcbRef != "C1" || q.Type != "cap" || q.Page != 2 || q.PageSize != 0 {
		t.Errorf("query = %+v", q)
	}
}
