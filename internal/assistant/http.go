package assistant

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"partlist/internal/assistant/internal/llm"
)

const (
	maxRequestBytes int64         = 8 << 10
	askTimeout      time.Duration = 90 * time.Second
)

type askRequest struct {
	Question string `json:"question"`
}

// RegisterRoutes mounts POST /ask; the caller supplies the group (and its auth middleware).
func RegisterRoutes(group *gin.RouterGroup, svc Service) {
	group.POST("/ask", askHandler(svc))
}

func askHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
		var in askRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), askTimeout)
		defer cancel()

		answer, err := svc.Ask(ctx, in.Question)
		if err != nil {
			respondError(c, err)
			return
		}
		c.JSON(http.StatusOK, answer)
	}
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidQuestion):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Question must be 1-1000 characters"})
	case errors.Is(err, ErrRefused):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "The assistant declined this question"})
	case errors.Is(err, context.DeadlineExceeded):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "The assistant took too long"})
	case errors.Is(err, context.Canceled):
		// The client went away; nobody is listening for a body.
		c.Status(http.StatusRequestTimeout)
	case errors.Is(err, ErrTooManyRounds), errors.Is(err, ErrNoAnswer), errors.Is(err, llm.ErrTruncated):
		log.Printf("assistant: no usable answer: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "The assistant could not produce an answer"})
	default:
		log.Printf("assistant: ask failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "The assistant is unavailable"})
	}
}
