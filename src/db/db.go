package database

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dbUrl string) error {

	var err error
	DB, err = gorm.Open(postgres.Open(dbUrl), &gorm.Config{})
	if err != nil {
		return err
	}
	return nil
}

// SetDB injects the connection opened by internal/platform/db.
//
// Deprecated: bridge for legacy handlers; removed when internal/parts replaces src/handler.
func SetDB(conn *gorm.DB) {
	DB = conn
}

func GetDB() *gorm.DB {
	return DB
}
