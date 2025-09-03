package utilis

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port      string
	JWTSecret string
	DBUrl     string
	BryptCost string
}

var AppConfig Config

func Init() {
	err := godotenv.Load()
	if err != nil {
		fmt.Print("No .env file found, using environment variables")
	}

	AppConfig = Config{
		Port:      os.Getenv("PORT"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		DBUrl:     os.Getenv("DB_URL"),
		BryptCost: os.Getenv("BCRYPT_COST"),
	}
}

func GetConfig() Config {
	return AppConfig
}
