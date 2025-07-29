package csv

import (
	"fmt"
	part "partlist/bomConverter/lib/part"
	"partlist/bomConverter/lib/request"
	"strings"

	"github.com/xuri/excelize/v2" // Using excelize for XLSX/XLSM files
)

// CsvHandler handles CSV and Excel file processing.
type CsvHandler struct {
	ExcelFile      string
	ExcelFileList  []string
	CSVExtension   string
	DataStarted    bool
	ProjectNameKey string
	ProjectCodeKey string
	ProjectName    string
	ProjectCode    string
	PartList       []part.Part
	HeaderIndex    map[part.HeaderIndex]int
}

type App struct {
	URLRequest string
	PartList   []part.Part
	Client     *request.ClientService
	CsvHandler *CsvHandler
}

// NewCsvHandler creates a new CsvHandler instance.
func NewCsvHandler(excelFileName string, excelDirList []string) *CsvHandler {
	return &CsvHandler{
		ExcelFile:      excelFileName,
		ExcelFileList:  excelDirList,
		CSVExtension:   ".csv",
		DataStarted:    false,
		ProjectNameKey: "Project Name :",
		ProjectCodeKey: "Project Code :",
		PartList:       []part.Part{},
		HeaderIndex:    make(map[part.HeaderIndex]int),
	}
}

// XLSXSaveCSVConverter converts XLSX/XLSM files to CSV and processes them.
func (ch *CsvHandler) XLSXSaveCSVConverter(urlRequest string) ([]part.Part, error) {

	if ch.ExcelFile != "" && len(ch.ExcelFileList) == 0 {
		return ch.processSingleExcelFile(ch.ExcelFile)
	} else if ch.ExcelFile == "" && len(ch.ExcelFileList) > 0 {
		ch.processMultipleExcelFiles(urlRequest)
		return []part.Part{}, nil
	}
	return []part.Part{}, nil
}

// CleanText replaces newline characters with spaces in a string.
func CleanText(description string) string {
	return strings.ReplaceAll(description, "\n", " ")
}

func (ch *CsvHandler) processSingleExcelFile(excelFileName string) ([]part.Part, error) {
	f, err := excelize.OpenFile(excelFileName)
	if err != nil {
		return nil, fmt.Errorf("error opening Excel file '%s': %w", excelFileName, err)
	}
	defer f.Close()

	sheetName := f.GetSheetName(0) // Get the first sheet name

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("error reading rows from Excel file '%s': %w", excelFileName, err)
	}

	for _, row := range rows {
		if len(row) > 1 {
			switch row[0] {
			case ch.ProjectNameKey:
				ch.ProjectName = row[1]
			case ch.ProjectCodeKey:
				ch.ProjectCode = row[1]
			}
		}
		if len(row) != 0 {
			ch.updateBOM(row)
		}
	}

	return ch.PartList, nil
}

func (ch *CsvHandler) processMultipleExcelFiles(urlRequest string) error {
	fileNumber := len(ch.ExcelFileList)
	client := request.NewClientService(urlRequest)
	for i, file := range ch.ExcelFileList {
		parts, err := ch.processSingleExcelFile(file)
		if err != nil {
			return err // Or handle the error differently if you want to continue
		}
		ch.PartList = append(ch.PartList, parts...)
		if len(ch.PartList) > 0 {
			for _, part := range ch.PartList {
				_, err := client.PostPartJSON(part)
				if err != nil {
					continue
					// fmt.Printf("Error posting part: %v\n", err)
				}
			}
		} else {
			fmt.Println("No parts found to process.")
		}
		ch.PartList = []part.Part{}
		fmt.Printf("Progress: %d/%d\r", i+1, fileNumber)
	}
	fmt.Println() // New line after progress
	return nil
}

func (ch *CsvHandler) updateBOM(csvRow []string) {
	if ch.DataStarted && !contains(csvRow, "Designator") {
		designator := csvRow[0]
		if designator == "" {
			return
		}

		partTypeFound := part.GetPartType(designator)

		if partTypeFound != part.PartTypeUnknown {
			partInfo := part.Part{
				Type:        strings.ToLower(string(partTypeFound)),
				ProjectName: ch.ProjectName,
				ProjectCode: ch.ProjectCode,
			}

			// Safely access header indices
			if idx, ok := ch.HeaderIndex[part.HeaderIndexDescription]; ok && len(csvRow) > idx {
				partInfo.Description = CleanText(csvRow[idx])
			}
			if idx, ok := ch.HeaderIndex[part.HeaderIndexFit]; ok && len(csvRow) > idx {
				if csvRow[idx] == "" {
					partInfo.Fit = ""
				} else {
					partInfo.Fit = CleanText(csvRow[idx])
				}
			}
			if idx, ok := ch.HeaderIndex[part.HeaderIndexFootprint]; ok && len(csvRow) > idx {
				if csvRow[idx] == "" {
					partInfo.Footprint = ""
				} else {
					partInfo.Footprint = CleanText(csvRow[idx])
				}
			}
			if idx, ok := ch.HeaderIndex[part.HeaderIndexManufacturer]; ok && len(csvRow) > idx {
				if csvRow[idx] == "" {
					partInfo.Manufacturer = ""
				} else {
					partInfo.Manufacturer = CleanText(csvRow[idx])
				}
			}
			if idx, ok := ch.HeaderIndex[part.HeaderIndexPartNumber]; ok && len(csvRow) > idx {
				if csvRow[idx] == "" {
					partInfo.PartNumber = ""
				} else {
					partInfo.PartNumber = CleanText(csvRow[idx])
				}
			}
			if idx, ok := ch.HeaderIndex[part.HeaderIndexValue]; ok && len(csvRow) > idx {
				if csvRow[idx] == "" {
					partInfo.Value = ""
				} else {
					partInfo.Value = CleanText(csvRow[idx])
				}
			}
			ch.PartList = append(ch.PartList, partInfo)
		}
	}

	if !ch.DataStarted && len(csvRow) > 0 && csvRow[0] == "Designator" {
		ch.DataStarted = true
		for idx, header := range csvRow {
			switch header {
			case string(part.HeaderIndexType):
				ch.HeaderIndex[part.HeaderIndexType] = idx
			case string(part.HeaderIndexDescription):
				ch.HeaderIndex[part.HeaderIndexDescription] = idx
			case string(part.HeaderIndexFit):
				ch.HeaderIndex[part.HeaderIndexFit] = idx
			case string(part.HeaderIndexFootprint):
				ch.HeaderIndex[part.HeaderIndexFootprint] = idx
			case string(part.HeaderIndexManufacturer):
				ch.HeaderIndex[part.HeaderIndexManufacturer] = idx
			case string(part.HeaderIndexPartNumber):
				ch.HeaderIndex[part.HeaderIndexPartNumber] = idx
			case string(part.HeaderIndexValue):
				ch.HeaderIndex[part.HeaderIndexValue] = idx
				// Project Name and Code are read separately, not from header row
			}
		}
	}
}

// contains checks if a string exists in a string slice.
func contains(slice []string, item string) bool {
	for _, a := range slice {
		if a == item {
			return true
		}
	}
	return false
}
