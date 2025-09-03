package utilis

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DescriptionUrl string
}

var AppConfig Config

func Init() {
	err := godotenv.Load()
	if err != nil {
		fmt.Print("No .env file found, using environment variables")
	}

	AppConfig = Config{
		DescriptionUrl: os.Getenv("DESCRIPTION_URL"),
	}

	fmt.Printf("Appconfig %v", AppConfig)
}

func GetConfig() Config {
	return AppConfig
}
