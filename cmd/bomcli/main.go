package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"partlist/internal/bomconverter"
	"partlist/internal/parts"
	"partlist/internal/platform/config"
	"partlist/internal/platform/db"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

// run imports each given .xlsm/.xlsx file directly through the service layer (no HTTP, no login).
// Columns are found by the saved mapping for the file's header row, else by detection; a file whose
// required columns cannot be found is refused, so use the web page to map it by hand.
func run(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("usage: bomcli <file.xlsm> [more files...]")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := db.Open(cfg.DBURL)
	if err != nil {
		return err
	}
	svc := bomconverter.New(parts.New(conn, parts.Options{}), conn)

	for _, path := range paths {
		if err := importFile(svc, path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func importFile(svc bomconverter.Service, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	res, err := svc.Import(context.Background(), f, bomconverter.ImportOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("%s: total=%d inserted=%d skipped=%d duplicates=%d\n", path, res.Total, res.Inserted, res.Skipped, res.Duplicates)
	return nil
}
