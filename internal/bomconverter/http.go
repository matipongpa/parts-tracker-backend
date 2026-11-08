package bomconverter

import (
	"encoding/json"
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	uploadField    string = "file"
	sheetField     string = "sheet"
	headerRowField string = "header_row"
	mappingField   string = "mapping"
	saveField      string = "save_mapping"
	maxUploadBytes int64  = 10 << 20
)

// RegisterRoutes mounts POST /bom/preview and POST /bom/import; the caller supplies the group (and its
// auth middleware). username names the caller, for recording who confirmed a saved mapping.
func RegisterRoutes(group *gin.RouterGroup, svc Service, username func(*gin.Context) string) {
	group.POST("/bom/preview", previewHandler(svc))
	group.POST("/bom/import", importHandler(svc, username))
}

func previewHandler(svc Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, ok := openUpload(c)
		if !ok {
			return
		}
		defer file.Close()

		opts := PreviewOptions{Sheet: c.PostForm(sheetField)}
		if raw := c.PostForm(mappingField); raw != "" {
			mapping, err := decodeMapping(raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping"})
				return
			}
			opts.Mapping = &mapping
		}
		if raw := c.PostForm(headerRowField); raw != "" {
			row, err := strconv.Atoi(raw)
			if err != nil || row < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid header row"})
				return
			}
			opts.HeaderRow = &row
		}
		preview, err := svc.Preview(c.Request.Context(), file, opts)
		if err != nil {
			respondError(c, "preview", err)
			return
		}
		c.JSON(http.StatusOK, preview)
	}
}

func importHandler(svc Service, username func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, ok := openUpload(c)
		if !ok {
			return
		}
		defer file.Close()

		opts := ImportOptions{Sheet: c.PostForm(sheetField), User: username(c)}
		if raw := c.PostForm(mappingField); raw != "" {
			mapping, err := decodeMapping(raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping"})
				return
			}
			opts.Mapping = &mapping
			opts.SaveMapping = c.PostForm(saveField) == "true"
		}
		result, err := svc.Import(c.Request.Context(), file, opts)
		if err != nil {
			respondError(c, "import", err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// decodeMapping rejects unknown keys, so a typo in a field name is an error and not a silently ignored mapping.
func decodeMapping(raw string) (Mapping, error) {
	var mapping Mapping
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mapping); err != nil {
		return Mapping{}, err
	}
	return mapping, nil
}

// openUpload reads the "file" part, enforcing the size limit and extension. It has already answered
// the client when ok is false.
func openUpload(c *gin.Context) (multipart.File, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	header, err := c.FormFile(uploadField)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "File too large"})
			return nil, false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing file field \"file\""})
		return nil, false
	}
	if ext := strings.ToLower(filepath.Ext(header.Filename)); ext != ".xlsm" && ext != ".xlsx" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only .xlsm and .xlsx files are accepted"})
		return nil, false
	}
	file, err := header.Open()
	if err != nil {
		log.Printf("bomconverter: open upload: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
		return nil, false
	}
	return file, true
}

func respondError(c *gin.Context, op string, err error) {
	var mappingErr *MappingError
	switch {
	case errors.As(err, &mappingErr):
		// The text names fields and limits only, so it is safe to show.
		body := gin.H{"error": mappingErr.Error()}
		if len(mappingErr.Missing) > 0 {
			body["missing"] = mappingErr.Missing
		}
		c.JSON(http.StatusUnprocessableEntity, body)
	case errors.Is(err, ErrUnknownSheet):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown sheet"})
	case errors.Is(err, ErrInvalidFile):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "File is not a valid BOM workbook"})
	default:
		log.Printf("bomconverter: %s failed: %v", op, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
	}
}
