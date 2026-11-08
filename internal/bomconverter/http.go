package bomconverter

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	uploadField    string = "file"
	maxUploadBytes int64  = 10 << 20
)

// RegisterRoutes mounts POST /bom/import; the caller supplies the group (and its auth middleware).
func RegisterRoutes(group *gin.RouterGroup, svc Service) {
	group.POST("/bom/import", importHandler(svc))
}

func importHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
		header, err := c.FormFile(uploadField)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "File too large"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing file field \"file\""})
			return
		}
		if ext := strings.ToLower(filepath.Ext(header.Filename)); ext != ".xlsm" && ext != ".xlsx" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only .xlsm and .xlsx files are accepted"})
			return
		}
		file, err := header.Open()
		if err != nil {
			log.Printf("bomconverter: open upload: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
			return
		}
		defer file.Close()

		result, err := svc.Import(c.Request.Context(), file)
		switch {
		case err == nil:
			c.JSON(http.StatusOK, result)
		case errors.Is(err, ErrInvalidFile):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "File is not a valid BOM workbook"})
		default:
			log.Printf("bomconverter: import failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
		}
	}
}
