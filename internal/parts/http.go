package parts

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type searchParams struct {
	PcbRef      string `form:"pcb_ref"`
	PartNumber  string `form:"part_number"`
	Value       string `form:"value"`
	Description string `form:"description"`
	Type        string `form:"type"`
	Page        string `form:"page"`
	PageSize    string `form:"page_size"`
}

// RegisterRoutes mounts the part routes; the caller supplies the group (and its auth middleware).
func RegisterRoutes(group *gin.RouterGroup, svc Service) {
	group.GET("/part", searchHandler(svc))
	group.GET("/part/:id", getHandler(svc))
	group.POST("/part", createHandler(svc))
	group.PUT("/part/:id", updateHandler(svc))
	group.DELETE("/part/:id", deleteHandler(svc))
}

func searchHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var params searchParams
		if err := c.ShouldBindQuery(&params); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query"})
			return
		}
		result, err := svc.Search(c.Request.Context(), SearchQuery{
			PcbRef:      params.PcbRef,
			PartNumber:  params.PartNumber,
			Value:       params.Value,
			Description: params.Description,
			Type:        params.Type,
			Page:        atoiOrZero(params.Page),
			PageSize:    atoiOrZero(params.PageSize),
		})
		if err != nil {
			respondError(c, "search", err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func getHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		part, err := svc.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			respondError(c, "get", err)
			return
		}
		c.JSON(http.StatusOK, part)
	}
}

func createHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in NewPart
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}
		part, err := svc.Create(c.Request.Context(), in)
		if err != nil {
			respondError(c, "create", err)
			return
		}
		c.JSON(http.StatusOK, part)
	}
}

func updateHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in PartPatch
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}
		part, err := svc.Update(c.Request.Context(), c.Param("id"), in)
		if err != nil {
			respondError(c, "update", err)
			return
		}
		c.JSON(http.StatusOK, part)
	}
}

func deleteHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
			respondError(c, "delete", err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Part deleted"})
	}
}

func respondError(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Part not found"})
	default:
		log.Printf("parts: %s failed: %v", op, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
	}
}

// atoiOrZero keeps the legacy leniency: an unparsable page or page_size falls back to the default.
func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
