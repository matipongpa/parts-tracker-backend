package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

var testCookies CookieConfig = CookieConfig{Secure: true, SameSite: http.SameSiteLaxMode}

func newTestRouter(t *testing.T) (*gin.Engine, Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _ := newTestService(t)

	router := gin.New()
	api := router.Group("/api")
	RegisterRoutes(api, svc, testCookies)

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

func loginAlice(t *testing.T, router *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	rec := doJSON(router, http.MethodPost, "/api/register", `{"invitecode":"team-invite","username":"alice","password":"pw"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body)
	}
	rec = doJSON(router, http.MethodPost, "/api/login", `{"username":"alice","password":"pw"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body)
	}
	return rec
}

func refreshCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in response; headers = %v", RefreshCookieName, rec.Header())
	return nil
}

func postWithCookie(router *gin.Engine, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestLoginSetsHardenedRefreshCookie(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := loginAlice(t, router)

	if strings.Contains(rec.Body.String(), "refresh") {
		t.Errorf("refresh token leaked into the JSON body: %s", rec.Body)
	}
	cookie := refreshCookieFrom(t, rec)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = httpOnly:%v secure:%v samesite:%v, want true/true/Lax", cookie.HttpOnly, cookie.Secure, cookie.SameSite)
	}
	if cookie.Path != "/api/auth" {
		t.Errorf("cookie path = %q, want /api/auth", cookie.Path)
	}
	if cookie.MaxAge != int((24 * time.Hour).Seconds()) {
		t.Errorf("cookie max-age = %d, want 24h in seconds", cookie.MaxAge)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestRefreshEndpointIssuesAccessToken(t *testing.T) {
	router, _ := newTestRouter(t)
	cookie := refreshCookieFrom(t, loginAlice(t, router))

	rec := postWithCookie(router, "/api/auth/refresh", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	const prefix string = `{"token":"`
	body := rec.Body.String()
	if !strings.HasPrefix(body, prefix) {
		t.Fatalf("unexpected body: %s", body)
	}
	access := strings.TrimSuffix(strings.TrimPrefix(body, prefix), `"}`)

	rec = doJSON(router, http.MethodGet, "/api/whoami", "", "Bearer "+access)
	if rec.Code != http.StatusOK || rec.Body.String() != "alice" {
		t.Fatalf("whoami with refreshed token = %d %q, want 200 alice", rec.Code, rec.Body)
	}
}

func TestRefreshEndpointRejections(t *testing.T) {
	router, _ := newTestRouter(t)
	loginAlice(t, router)

	t.Run("no cookie", func(t *testing.T) {
		if rec := postWithCookie(router, "/api/auth/refresh", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
	t.Run("garbage cookie clears it", func(t *testing.T) {
		rec := postWithCookie(router, "/api/auth/refresh", &http.Cookie{Name: RefreshCookieName, Value: "nope"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if cleared := refreshCookieFrom(t, rec); cleared.MaxAge >= 0 {
			t.Errorf("max-age = %d, want negative so the browser deletes the cookie", cleared.MaxAge)
		}
	})
}

func TestAccessTokenCookieCannotBeUsedToRefreshAndViceVersa(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := loginAlice(t, router)
	body := rec.Body.String()
	access := strings.TrimSuffix(strings.TrimPrefix(body, `{"token":"`), `"}`)

	// An access token presented as the refresh cookie must fail.
	rec = postWithCookie(router, "/api/auth/refresh", &http.Cookie{Name: RefreshCookieName, Value: access})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("access token as refresh cookie: status = %d, want 401", rec.Code)
	}

	// A refresh token presented as a Bearer token must fail.
	refresh := refreshCookieFrom(t, loginAlice2(t, router))
	rec = doJSON(router, http.MethodGet, "/api/whoami", "", "Bearer "+refresh.Value)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh token as bearer: status = %d, want 401", rec.Code)
	}
}

// loginAlice2 logs in the already registered alice.
func loginAlice2(t *testing.T, router *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	rec := doJSON(router, http.MethodPost, "/api/login", `{"username":"alice","password":"pw"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body)
	}
	return rec
}

func TestLogoutClearsCookie(t *testing.T) {
	router, _ := newTestRouter(t)
	cookie := refreshCookieFrom(t, loginAlice(t, router))

	rec := postWithCookie(router, "/api/auth/logout", cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	cleared := refreshCookieFrom(t, rec)
	if cleared.MaxAge >= 0 || cleared.Path != "/api/auth" || !cleared.HttpOnly {
		t.Errorf("cleared cookie = %+v, want negative max-age, path /api/auth, httpOnly", cleared)
	}
}
