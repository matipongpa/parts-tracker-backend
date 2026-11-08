package migrate

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

// Up applies all pending migrations from files. A fully migrated database is not an error.
func Up(conn *gorm.DB, files fs.FS) error {
	sqlDB, err := conn.DB()
	if err != nil {
		return fmt.Errorf("migrate: get sql.DB: %w", err)
	}
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate: init driver: %w", err)
	}
	source, err := iofs.New(files, ".")
	if err != nil {
		return fmt.Errorf("migrate: open source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate: create: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}
