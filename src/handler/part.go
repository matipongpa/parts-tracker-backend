package handler

import (
	"net/http"
	database "partlist/src/db"
	"partlist/src/utilis"

	"github.com/gin-gonic/gin"
)

type PartWithCount struct {
	List  []Part `json:"list"`
	Count int64  `json:"count"`
}
type PartQueryParam struct {
	PcbRef      string `form:"pcb_ref"`
	PartNumber  string `form:"part_number"`
	Value       string `form:"value"`
	Description string `form:"description"`
	Type        string `form:"type"`
	Page        string `form:"page"`
	PageSize    string `form:"page_size"`
}

type Part struct {
	Id           *string `json:"id" gorm:"primaryKey;autoIncrement"`
	PartNumber   string  `json:"part_number" gorm:"column:partnumber"`
	Pcbref       string  `json:"pcb_ref" gorm:"column:pcbref"`
	Description  string  `json:"description" gorm:"column:description"`
	Footprint    string  `json:"footprint" gorm:"column:footprint"`
	Fit          string  `json:"fit" gorm:"column:fit"`
	Manufacturer string  `json:"manufacturer" gorm:"column:manufacturer"`
	ParType      string  `json:"type" gorm:"column:type"`
	ProjectName  string  `json:"project_name" gorm:"column:projectname"`
	Value        string  `json:"value" gorm:"column:value"`
}

func GetPartById(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusInternalServerError, "Id should not be empty")
	}

	db := database.GetDB()

	var part Part

	if err := db.Where("id = ?", id).First(&part).Error; err != nil {
		c.JSON(http.StatusNotFound, "content not found")
		return
	}

	c.JSON(http.StatusOK, part)
}

func GetPart(c *gin.Context) {
	var queryParam PartQueryParam

	if err := c.ShouldBindQuery(&queryParam); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	dbQuery := db.Model(&Part{})

	if queryParam.PcbRef != "" {
		var queryParamWildcard = "%" + queryParam.PcbRef + "%"
		dbQuery = dbQuery.Where("pcbref LIKE ?", queryParamWildcard)
	}

	if queryParam.PartNumber != "" {
		var queryParamWildcard = "%" + queryParam.PartNumber + "%"
		dbQuery = dbQuery.Where("partnumber LIKE ?", queryParamWildcard)
	}

	if queryParam.Value != "" {
		var queryParamWildcard = "%" + queryParam.Value + "%"
		dbQuery = dbQuery.Where("value LIKE ?", queryParamWildcard)
	}

	if queryParam.Description != "" {
		var queryParamWildcard = "%" + queryParam.Description + "%"
		dbQuery = dbQuery.Where("description LIKE ?", queryParamWildcard)
	}

	if queryParam.Type != "" {
		var queryParamWildcard = "%" + queryParam.Type + "%"
		dbQuery = dbQuery.Where("type LIKE ?", queryParamWildcard)
	}
	var count int64
	dbQuery.Count(&count)

	var part []Part
	dbQuery.Scopes(utilis.Paginate(queryParam.Page, queryParam.PageSize)).Find(&part)

	response := PartWithCount{
		List:  part,
		Count: count,
	}

	c.JSON(http.StatusOK, response)
}

func PutUpdatePart(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusInternalServerError, "Id should not be empty")
	}

	var updatePart Part
	if err := c.ShouldBindJSON(&updatePart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	if err := db.Model(&Part{}).Where("id = ?", id).Updates(updatePart).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Save to DB or process as needed
	c.JSON(http.StatusOK, updatePart)
}

func PostCreatePart(c *gin.Context) {
	var part Part

	if err := c.ShouldBindJSON(&part); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error ": err.Error()})
		return
	}

	if part.Value == "" ||
		part.PartNumber == "" ||
		part.Pcbref == "" ||
		part.Fit == "" ||
		part.Manufacturer == "" ||
		part.ParType == "" ||
		part.ProjectName == "" {
		c.JSON(http.StatusInternalServerError, "require field should not empty")
		return
	}

	db := database.GetDB()

	if err := db.Save(&part).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"save capacitor fail error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, "Create capacitor success")

}

func DeletePart(c *gin.Context) {
	id := c.Param("id")

	db := database.GetDB()

	if err := db.Delete(&Part{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Delete capacitor fail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, "Delete capacitor success")
}
