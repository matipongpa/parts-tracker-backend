package identity

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// UsernameContextKey is where Middleware stores the caller's username.
// The value matches the key the pre-refactor middleware used.
const UsernameContextKey string = "userName"

const bearerPrefix string = "Bearer "

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

// RegisterRoutes mounts the public auth routes (/register, /login) on the given group.
func RegisterRoutes(public *gin.RouterGroup, svc Service) {
	public.POST("/register", registerHandler(svc))
	public.POST("/login", loginHandler(svc))
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

func loginHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}

		tokenString, err := svc.Login(c.Request.Context(), req.Username, req.Password)
		switch {
		case err == nil:
			c.JSON(http.StatusOK, gin.H{"token": tokenString})
		case errors.Is(err, ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		default:
			log.Printf("identity: login failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		}
	}
}
