package assistant

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"partlist/internal/assistant/internal/llm"
)

const (
	maxRequestBytes int64         = 8 << 10
	askTimeout      time.Duration = 90 * time.Second
	// saveTimeout bounds the history write, which must not depend on the (possibly expired) ask context.
	saveTimeout time.Duration = 5 * time.Second
)

// UserFunc returns the authenticated caller's username, or "" if there is none. It is injected by the
// composition root so this module does not import identity.
type UserFunc func(c *gin.Context) string

type askRequest struct {
	Question string `json:"question"`
}

// RegisterAskRoutes mounts POST /ask; the caller supplies the group (and its auth middleware).
// Every answered question is saved to history under the caller's username.
func RegisterAskRoutes(group *gin.RouterGroup, svc Service, hist History, user UserFunc) {
	group.POST("/ask", askHandler(svc, hist, user))
}

// RegisterHistoryRoutes mounts the saved-conversation routes. They work without a model, so
// old answers stay readable even if the assistant is later switched off.
func RegisterHistoryRoutes(group *gin.RouterGroup, hist History, user UserFunc) {
	group.GET("/ask/history", listHistoryHandler(hist, user))
	group.GET("/ask/history/:id", getHistoryHandler(hist, user))
	group.DELETE("/ask/history/:id", deleteHistoryHandler(hist, user))
}

func askHandler(svc Service, hist History, user UserFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := user(c)
		if username == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
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
		c.JSON(http.StatusOK, saveAnswer(c.Request.Context(), hist, username, strings.TrimSpace(in.Question), answer))
	}
}

// saveAnswer stores the answer and returns it as an Entry. A failed save must not cost the user a
// minute-long answer, so the error is logged and the entry comes back without an ID.
func saveAnswer(reqCtx context.Context, hist History, username string, question string, answer Answer) Entry {
	// Detached from the request: if the client gave up while the model was thinking, the answer is still kept.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), saveTimeout)
	defer cancel()

	entry, err := hist.Save(ctx, username, question, answer)
	if err != nil {
		log.Printf("assistant: save history failed: %v", err)
		return Entry{
			Question: question, Answer: answer.Answer, Parts: answer.Parts,
			PartsTruncated: answer.PartsTruncated, ToolsUsed: answer.ToolsUsed, CreatedAt: time.Now().UTC(),
		}
	}
	return entry
}

func listHistoryHandler(hist History, user UserFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := user(c)
		if username == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		items, err := hist.List(c.Request.Context(), username)
		if err != nil {
			respondHistoryError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"list": items})
	}
}

func getHistoryHandler(hist History, user UserFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := user(c)
		if username == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		entry, err := hist.Get(c.Request.Context(), username, c.Param("id"))
		if err != nil {
			respondHistoryError(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}

func deleteHistoryHandler(hist History, user UserFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := user(c)
		if username == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		if err := hist.Delete(c.Request.Context(), username, c.Param("id")); err != nil {
			respondHistoryError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func respondHistoryError(c *gin.Context, err error) {
	if errors.Is(err, ErrHistoryNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	log.Printf("assistant: history failed: %v", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load history"})
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
