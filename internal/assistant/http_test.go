package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

// fakeHistory is an in-memory History keyed by username, so tests can check isolation between users.
type fakeHistory struct {
	entries map[string][]Entry
	nextID  int
	saveErr error
}

func newFakeHistory() *fakeHistory { return &fakeHistory{entries: map[string][]Entry{}} }

func (f *fakeHistory) Save(_ context.Context, username string, question string, a Answer) (Entry, error) {
	if f.saveErr != nil {
		return Entry{}, f.saveErr
	}
	f.nextID++
	e := Entry{
		ID: strconv.Itoa(f.nextID), Question: question, Answer: a.Answer, Parts: a.Parts,
		PartsTruncated: a.PartsTruncated, ToolsUsed: a.ToolsUsed, CreatedAt: time.Now().UTC(),
	}
	f.entries[username] = append(f.entries[username], e)
	return e, nil
}

func (f *fakeHistory) List(_ context.Context, username string) ([]HistoryItem, error) {
	items := []HistoryItem{}
	for _, e := range f.entries[username] {
		items = append(items, HistoryItem{ID: e.ID, Question: e.Question, CreatedAt: e.CreatedAt})
	}
	return items, nil
}

func (f *fakeHistory) Get(_ context.Context, username string, id string) (Entry, error) {
	for _, e := range f.entries[username] {
		if e.ID == id {
			return e, nil
		}
	}
	return Entry{}, ErrHistoryNotFound
}

func (f *fakeHistory) Delete(_ context.Context, username string, id string) error {
	for i, e := range f.entries[username] {
		if e.ID == id {
			f.entries[username] = append(f.entries[username][:i], f.entries[username][i+1:]...)
			return nil
		}
	}
	return ErrHistoryNotFound
}

const testUser string = "alice"

// userFrom reads the username from a test header, standing in for the auth middleware.
func userFrom(c *gin.Context) string { return c.GetHeader("X-Test-User") }

func newRouter(svc Service, hist History) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	RegisterHistoryRoutes(group, hist, userFrom)
	RegisterAskRoutes(group, svc, hist, userFrom)
	return router
}

func do(router *gin.Engine, method string, path string, body string, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func post(svc Service, body string) *httptest.ResponseRecorder {
	return do(newRouter(svc, newFakeHistory()), http.MethodPost, "/api/ask", body, testUser)
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

func TestAskSavesTheExactTrimmedQuestionAndAnswer(t *testing.T) {
	hist := newFakeHistory()
	svc := &stubService{answer: Answer{Answer: "Used in A.", Parts: []parts.Part{{ID: "1", ProjectName: "A"}}, ToolsUsed: []string{"search_parts"}}}
	router := newRouter(svc, hist)

	rec := do(router, http.MethodPost, "/api/ask", `{"question":"  which projects use ABC?  "}`, testUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}

	var got Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Question != "which projects use ABC?" || got.Answer != "Used in A." || len(got.Parts) != 1 {
		t.Errorf("response entry = %+v", got)
	}
	saved := hist.entries[testUser]
	if len(saved) != 1 || saved[0].Question != "which projects use ABC?" || saved[0].Answer != "Used in A." {
		t.Errorf("saved = %+v", saved)
	}
}

func TestAskStillAnswersWhenSavingFails(t *testing.T) {
	hist := newFakeHistory()
	hist.saveErr = errors.New("database is down")
	svc := &stubService{answer: Answer{Answer: "Used in A."}}

	rec := do(newRouter(svc, hist), http.MethodPost, "/api/ask", `{"question":"q"}`, testUser)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: the answer must not be lost over a history failure", rec.Code)
	}
	var got Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "" || got.Answer != "Used in A." {
		t.Errorf("entry = %+v, want empty id and the answer", got)
	}
	if strings.Contains(rec.Body.String(), "database is down") {
		t.Errorf("internal error leaked: %s", rec.Body)
	}
}

func TestFailedAnswersAreNotSaved(t *testing.T) {
	hist := newFakeHistory()
	do(newRouter(&stubService{err: ErrRefused}, hist), http.MethodPost, "/api/ask", `{"question":"q"}`, testUser)
	if len(hist.entries[testUser]) != 0 {
		t.Errorf("saved %d entries for a failed ask, want 0", len(hist.entries[testUser]))
	}
}

func TestAskWithoutAUserDoesNotCallTheModel(t *testing.T) {
	svc := &stubService{}
	rec := do(newRouter(svc, newFakeHistory()), http.MethodPost, "/api/ask", `{"question":"q"}`, "")
	if rec.Code != http.StatusUnauthorized || svc.got != "" {
		t.Errorf("status %d, model asked %q; want 401 and no model call", rec.Code, svc.got)
	}
}

func TestHistoryListGetDelete(t *testing.T) {
	hist := newFakeHistory()
	router := newRouter(&stubService{answer: Answer{Answer: "A1"}}, hist)
	do(router, http.MethodPost, "/api/ask", `{"question":"first"}`, testUser)
	do(router, http.MethodPost, "/api/ask", `{"question":"second"}`, testUser)

	rec := do(router, http.MethodGet, "/api/ask/history", "", testUser)
	var list struct {
		List []HistoryItem `json:"list"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.List) != 2 {
		t.Fatalf("list = %s (%v)", rec.Body, err)
	}

	rec = do(router, http.MethodGet, "/api/ask/history/1", "", testUser)
	var entry Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil || entry.Question != "first" || entry.Answer != "A1" {
		t.Fatalf("get = %s (%v)", rec.Body, err)
	}

	if rec = do(router, http.MethodDelete, "/api/ask/history/1", "", testUser); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status %d", rec.Code)
	}
	if rec = do(router, http.MethodGet, "/api/ask/history/1", "", testUser); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete status %d, want 404", rec.Code)
	}
}

func TestEmptyHistoryIsAnEmptyArrayNotNull(t *testing.T) {
	rec := do(newRouter(&stubService{}, newFakeHistory()), http.MethodGet, "/api/ask/history", "", testUser)
	if strings.TrimSpace(rec.Body.String()) != `{"list":[]}` {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestHistoryIsPrivateToEachUser(t *testing.T) {
	hist := newFakeHistory()
	router := newRouter(&stubService{answer: Answer{Answer: "secret answer"}}, hist)
	do(router, http.MethodPost, "/api/ask", `{"question":"alice only"}`, "alice")

	if rec := do(router, http.MethodGet, "/api/ask/history/1", "", "bob"); rec.Code != http.StatusNotFound {
		t.Errorf("bob reading alice's entry: status %d, want 404 (not 403, so ids are not confirmed)", rec.Code)
	}
	if rec := do(router, http.MethodDelete, "/api/ask/history/1", "", "bob"); rec.Code != http.StatusNotFound {
		t.Errorf("bob deleting alice's entry: status %d, want 404", rec.Code)
	}
	rec := do(router, http.MethodGet, "/api/ask/history", "", "bob")
	if strings.Contains(rec.Body.String(), "alice only") {
		t.Errorf("bob's list leaks alice's question: %s", rec.Body)
	}
	if len(hist.entries["alice"]) != 1 {
		t.Error("alice's entry was removed by bob")
	}
}

func TestHistoryRoutesRequireAUser(t *testing.T) {
	router := newRouter(&stubService{}, newFakeHistory())
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/ask/history"},
		{http.MethodGet, "/api/ask/history/1"},
		{http.MethodDelete, "/api/ask/history/1"},
	} {
		if rec := do(router, tc.method, tc.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a user: status %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
