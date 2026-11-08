package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestRouter(t *testing.T) (*gin.Engine, Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _ := newTestService(t)

	router := gin.New()
	api := router.Group("/api")
	RegisterRoutes(api, svc)

	protected := api.Group("")
	protected.Use(Middleware(svc))
	protected.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString(UsernameContextKey))
	})
	return router, svc
}

func doJSON(router *gin.Engine, method string, path string, body string, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRegisterLoginAndProtectedRoute(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := doJSON(router, http.MethodPost, "/api/register", `{"invitecode":"team-invite","username":"alice","password":"pw"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body)
	}

	rec = doJSON(router, http.MethodPost, "/api/login", `{"username":"alice","password":"pw"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	const prefix string = `{"token":"`
	if !strings.HasPrefix(body, prefix) {
		t.Fatalf("unexpected login body: %s", body)
	}
	tokenString := strings.TrimSuffix(strings.TrimPrefix(body, prefix), `"}`)

	rec = doJSON(router, http.MethodGet, "/api/whoami", "", "Bearer "+tokenString)
	if rec.Code != http.StatusOK || rec.Body.String() != "alice" {
		t.Fatalf("whoami = %d %q, want 200 alice", rec.Code, rec.Body)
	}
}

func TestLoginWithClientKeyCasing(t *testing.T) {
	// bomConverter sends "userName"; encoding/json matches keys case-insensitively.
	router, _ := newTestRouter(t)
	doJSON(router, http.MethodPost, "/api/register", `{"invitecode":"team-invite","username":"alice","password":"pw"}`, "")

	rec := doJSON(router, http.MethodPost, "/api/login", `{"userName":"alice","password":"pw"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestRegisterStatusCodes(t *testing.T) {
	router, _ := newTestRouter(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"created", `{"invitecode":"team-invite","username":"alice","password":"pw"}`, http.StatusOK},
		{"duplicate", `{"invitecode":"team-invite","username":"alice","password":"pw"}`, http.StatusConflict},
		{"missing password", `{"invitecode":"team-invite","username":"bob"}`, http.StatusBadRequest},
		{"missing invite code", `{"username":"carol","password":"pw"}`, http.StatusForbidden},
		{"wrong invite code", `{"invitecode":"nope","username":"carol","password":"pw"}`, http.StatusForbidden},
		{"malformed json", `{`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(router, http.MethodPost, "/api/register", tc.body, "")
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestLoginBadCredentials(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := doJSON(router, http.MethodPost, "/api/login", `{"username":"ghost","password":"pw"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareRejects(t *testing.T) {
	router, _ := newTestRouter(t)
	cases := map[string]string{
		"no header":    "",
		"wrong scheme": "Basic abc",
		"bad token":    "Bearer nope",
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			rec := doJSON(router, http.MethodGet, "/api/whoami", "", header)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}
