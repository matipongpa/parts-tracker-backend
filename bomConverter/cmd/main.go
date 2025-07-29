package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	csv "partlist/bomConverter/lib/csv"
	part "partlist/bomConverter/lib/part"
	request "partlist/bomConverter/lib/request"
)

// App orchestrates the conversion and upload process.
type App struct {
	URLRequest string
	PartList   []part.Part
	Client     *request.ClientService
	CsvHandler *csv.CsvHandler
}

// NewApp creates a new App instance.
func NewApp(excelFileName, serverURL string) (*App, error) {
	app := &App{
		URLRequest: serverURL,
		PartList:   []part.Part{},
		Client:     request.NewClientService(serverURL),
	}

	if !strings.HasSuffix(excelFileName, ".xlsm") {
		var excelFileList []string
		files, err := os.ReadDir(excelFileName)
		if err != nil {
			return nil, fmt.Errorf("error reading directory '%s': %w", excelFileName, err)
		}
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".xlsm") {
				excelFileList = append(excelFileList, filepath.Join(excelFileName, file.Name()))
			}
		}

		app.CsvHandler = csv.NewCsvHandler("", excelFileList)
	} else {
		app.CsvHandler = csv.NewCsvHandler(excelFileName, []string{})
	}

	return app, nil
}

// Run executes the conversion and upload process.
func (a *App) Run() {
	parts, err := a.CsvHandler.XLSXSaveCSVConverter(a.URLRequest)
	if err != nil {
		fmt.Printf("Error during CSV conversion: %v\n", err)
		return
	}
	a.PartList = parts
	if len(a.PartList) > 0 {
		for _, part := range a.PartList {
			_, err := a.Client.PostPartJSON(part)
			if err != nil {
				fmt.Printf("Error posting part: %v\n", err)
			}
		}
	} else {
		fmt.Println("No parts found to process.")
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <excel_file_or_directory> <server_url>")
		return
	}

	excelArg := os.Args[1]
	serverURL := os.Args[2]

	app, err := NewApp(excelArg, serverURL)
	if err != nil {
		fmt.Printf("Error initializing application: %v\n", err)
		return
	}
	app.Run()
}
