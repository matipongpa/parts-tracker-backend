package handler

import (
	"fmt"
	"net/http"
	database "partlist/src/db"

	"github.com/gin-gonic/gin"
)

type PartQueryParam struct {
	Voltage string `form:"voltage"`
	Current string `form:"current"`
}

type Part struct {
	Id           *string `json:"id" gorm:"column:id"`
	PartNumber   string  `json:"part_number" gorm:"column:partNumber"`
	ProjectId    string  `json:"project_code" gorm:"column:projectId"`
	Description  string  `json:"description" gorm:"column:description"`
	Footprint    string  `json:"footprint" gorm:"column:footprint"`
	Fit          string  `json:"fit" gorm:"column:fit"`
	Manufacturer string  `json:"manufacturer" gorm:"column:manufacturer"`
	ParType      string  `json:"type" gorm:"column:type"`
	ProjectName  string  `json:"project_name" gorm:"column:projectName"`
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

	dbQuery := db.Model(&Part{}).Debug()
	if queryParam.Current != "" {
		dbQuery = dbQuery.Where("voltage = ?", queryParam.Voltage)
	}

	if queryParam.Current != "" {
		dbQuery = dbQuery.Where("current = ?", queryParam.Current)
	}

	var capacitors []Part
	dbQuery.Find(&capacitors)

	c.JSON(http.StatusOK, capacitors)
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

	fmt.Println("Part", part)

	if part.Value == "" ||
		part.PartNumber == "" ||
		part.ProjectId == "" ||
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
