package handler

import (
	"net/http"
	jwtAuth "partlist/src/app/auth/jwt"
	database "partlist/src/db"
	config "partlist/src/utilis"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username  string `json:"username" gorm:"column:username"`
	Password  string `json:"password" gorm:"column:password"`
	FirstName string `json:"firstname" gorm:"column:firstname"`
	SurName   string `json:"surname" gorm:"column:surname"`
}

// Register user handler
func Register(c *gin.Context) {
	appConfig := config.GetConfig()

	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	cost, err := strconv.Atoi(appConfig.BryptCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid Bcryptcost" + err.Error()})
		return
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), cost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.Password = string(hashedPassword)

	db := database.GetDB()
	// Save user with hashed password to DB
	if err = db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
}

// Login user handler
func Login(c *gin.Context) {
	var reqUser User
	if err := c.ShouldBindJSON(&reqUser); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	db := database.GetDB()

	var user User

	// Retrieve user from DB by username
	if err := db.Where("userName = ?", reqUser.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// Compare stored hash with password from request
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(reqUser.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// Generate JWT token
	token, err := jwtAuth.GenerateToken(user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}
