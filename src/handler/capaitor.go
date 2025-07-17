package handler

import (
	"net/http"
	database "partlist/src/db"

	"github.com/gin-gonic/gin"
)

type CapacitorQueryParam struct {
	Voltage string `form:"voltage"`
	Current string `form:"current"`
}

type Capacitor struct {
	Id         *string `json:"id" gorm:"column:id"`
	ProjectId  string  `json:"projectId" gorm:"column:projectId"`
	PartNumber string  `json:"partNumber" gorm:"column:partNumber"`
	Package    string  `json:"package" gorm:"column:package"`
	Value      float64 `json:"value" gorm:"column:value"`
	Voltage    float64 `json:"voltage" gorm:"column:voltage"`
}

func GetCapacitorById(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusInternalServerError, "Id should not be empty")
	}

	db := database.GetDB()

	var capacitor Capacitor

	if err := db.Where("id = ?", id).First(&capacitor).Error; err != nil {
		c.JSON(http.StatusNotFound, "content not found")
		return
	}

	c.JSON(http.StatusOK, capacitor)
}

func GetCapacitor(c *gin.Context) {
	var queryParam CapacitorQueryParam

	if err := c.ShouldBindQuery(&queryParam); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	dbQuery := db.Model(&Capacitor{}).Debug()
	if queryParam.Current != "" {
		dbQuery = dbQuery.Where("voltage = ?", queryParam.Voltage)
	}

	if queryParam.Current != "" {
		dbQuery = dbQuery.Where("current = ?", queryParam.Current)
	}

	var capacitors []Capacitor
	dbQuery.Find(&capacitors)

	c.JSON(http.StatusOK, capacitors)
}

func PutUpdateCapacitor(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusInternalServerError, "Id should not be empty")
	}

	var updateCapacitor Capacitor
	if err := c.ShouldBindJSON(&updateCapacitor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	if err := db.Model(&Capacitor{}).Where("id = ?", id).Updates(updateCapacitor).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Save to DB or process as needed
	c.JSON(http.StatusOK, updateCapacitor)
}

func PostCreateCapacitor(c *gin.Context) {
	var capacitor Capacitor

	if err := c.ShouldBindJSON(&capacitor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error ": err.Error()})
		return
	}

	if capacitor.Package == "" || capacitor.PartNumber == "" || capacitor.ProjectId == "" {
		c.JSON(http.StatusInternalServerError, "require package partnumber and project Id should not be empty")
		return
	}

	db := database.GetDB()

	if err := db.Save(&capacitor).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"save capacitor fail error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, "Create capacitor success")

}
