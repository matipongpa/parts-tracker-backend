package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"partlist/internal/assistant/internal/llm"
	"partlist/internal/parts"
)

type stubService struct {
	answer Answer
	err    error
	got    string
}

func (s *stubService) Ask(_ context.Context, q string) (Answer, error) {
	s.got = q
	return s.answer, s.err
}

func post(svc Service, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api"), svc)
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAskHandlerReturnsAnswerAndParts(t *testing.T) {
	svc := &stubService{answer: Answer{
		Answer: "Used in A.", Parts: []parts.Part{{ID: "1", ProjectName: "A"}}, ToolsUsed: []string{"get_part_usage"},
	}}
	rec := post(svc, `{"question":"which projects use ABC?"}`)
	if rec.Code != http.StatusOK || svc.got != "which projects use ABC?" {
		t.Fatalf("status %d, question %q", rec.Code, svc.got)
	}
	var body struct {
		Answer         string       `json:"answer"`
		Parts          []parts.Part `json:"parts"`
		PartsTruncated bool         `json:"parts_truncated"`
		ToolsUsed      []string     `json:"tools_used"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Answer != "Used in A." || len(body.Parts) != 1 || body.Parts[0].ProjectName != "A" || body.ToolsUsed[0] != "get_part_usage" {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAskHandlerStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"malformed json", `{`, nil, http.StatusBadRequest},
		{"unknown field is ignored, empty question", `{}`, ErrInvalidQuestion, http.StatusBadRequest},
		{"refused", `{"question":"q"}`, ErrRefused, http.StatusUnprocessableEntity},
		{"timeout", `{"question":"q"}`, context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"too many rounds", `{"question":"q"}`, ErrTooManyRounds, http.StatusBadGateway},
		{"truncated", `{"question":"q"}`, llm.ErrTruncated, http.StatusBadGateway},
		{"no answer", `{"question":"q"}`, ErrNoAnswer, http.StatusBadGateway},
		{"upstream failure", `{"question":"q"}`, errors.New("claude request: 500"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		rec := post(&stubService{err: tc.err}, tc.body)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "claude request") {
			t.Errorf("%s: upstream error text leaked to the client: %s", tc.name, rec.Body.String())
		}
	}
}

func TestAskHandlerRejectsOversizedBody(t *testing.T) {
	rec := post(&stubService{}, `{"question":"`+strings.Repeat("a", int(maxRequestBytes))+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}
