package identity

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// UsernameContextKey is where Middleware stores the caller's username.
// The value matches the key the pre-refactor middleware used.
const UsernameContextKey string = "userName"

const bearerPrefix string = "Bearer "

// RefreshCookieName is the httpOnly cookie that carries the refresh token.
const RefreshCookieName string = "refresh_token"

// CookieConfig sets the attributes of the refresh cookie. HttpOnly is always on and not configurable.
type CookieConfig struct {
	// Secure restricts the cookie to HTTPS. Turn it off only for plain-HTTP local development.
	Secure bool
	// SameSite is Lax or Strict when the UI and API share a site; None (which requires Secure) when they do not.
	SameSite http.SameSite
}

type registerRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	FirstName string `json:"firstname"`
	Surname   string `json:"surname"`
	// InviteCode is the shared code that authorises registration.
	InviteCode string `json:"invitecode"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterRoutes mounts the public auth routes on the given group: /register, /login,
// /auth/refresh and /auth/logout. The refresh cookie is scoped to /auth so the browser
// sends it to those two endpoints only, not on every API call.
func RegisterRoutes(public *gin.RouterGroup, svc Service, cookies CookieConfig) {
	cookiePath := public.BasePath() + "/auth"
	public.POST("/register", registerHandler(svc))
	public.POST("/login", loginHandler(svc, cookies, cookiePath))
	public.POST("/auth/refresh", refreshHandler(svc, cookies, cookiePath))
	public.POST("/auth/logout", logoutHandler(cookies, cookiePath))
}

func refreshCookie(cfg CookieConfig, path string, value string, maxAge time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     RefreshCookieName,
		Value:    value,
		Path:     path,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	}
}

// clearRefreshCookie expires the cookie. Path, Secure and SameSite must match how it was set.
func clearRefreshCookie(c *gin.Context, cfg CookieConfig, path string) {
	cookie := refreshCookie(cfg, path, "", 0)
	cookie.MaxAge = -1
	http.SetCookie(c.Writer, cookie)
}

// Middleware rejects requests without a valid Bearer token and stores the username on the context.
func Middleware(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid token"})
			return
		}

		claims, err := svc.Verify(c.Request.Context(), strings.TrimPrefix(header, bearerPrefix))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		c.Set(UsernameContextKey, claims.Username)
		c.Next()
	}
}

func registerHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req registerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}

		err := svc.Register(c.Request.Context(), RegisterInput{
			Username:   req.Username,
			Password:   req.Password,
			FirstName:  req.FirstName,
			Surname:    req.Surname,
			InviteCode: req.InviteCode,
		})
		switch {
		case err == nil:
			c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
		case errors.Is(err, ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		case errors.Is(err, ErrRegistrationClosed):
			c.JSON(http.StatusForbidden, gin.H{"error": "Registration is closed"})
		case errors.Is(err, ErrInvalidInviteCode):
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid invite code"})
		case errors.Is(err, ErrUsernameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": "Username already taken"})
		default:
			log.Printf("identity: register failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		}
	}
}

func loginHandler(svc Service, cookies CookieConfig, cookiePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}

		session, err := svc.Login(c.Request.Context(), req.Username, req.Password)
		switch {
		case err == nil:
			http.SetCookie(c.Writer, refreshCookie(cookies, cookiePath, session.RefreshToken, session.RefreshMaxAge))
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusOK, gin.H{"token": session.AccessToken})
		case errors.Is(err, ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		default:
			log.Printf("identity: login failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		}
	}
}

// refreshHandler trades the refresh cookie for a new access token. The browser attaches the
// cookie itself, so the page's JavaScript never sees the refresh token.
func refreshHandler(svc Service, cookies CookieConfig, cookiePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		refreshToken, err := c.Cookie(RefreshCookieName)
		if err != nil || refreshToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing refresh token"})
			return
		}

		accessToken, err := svc.Refresh(c.Request.Context(), refreshToken)
		switch {
		case err == nil:
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusOK, gin.H{"token": accessToken})
		case errors.Is(err, ErrInvalidToken):
			clearRefreshCookie(c, cookies, cookiePath)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		default:
			log.Printf("identity: refresh failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh token"})
		}
	}
}

// logoutHandler only drops the cookie. Refresh tokens are stateless, so one that was copied
// elsewhere stays valid until it expires; see the README.
func logoutHandler(cookies CookieConfig, cookiePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clearRefreshCookie(c, cookies, cookiePath)
		c.Status(http.StatusNoContent)
	}
}
