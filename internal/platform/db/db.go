package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open connects to Postgres. TranslateError makes unique violations surface
// as gorm.ErrDuplicatedKey so modules do not depend on driver error codes.
func Open(url string) (*gorm.DB, error) {
	conn, err := gorm.Open(postgres.Open(url), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return conn, nil
}
