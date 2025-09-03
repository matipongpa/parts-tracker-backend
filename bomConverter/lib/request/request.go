package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	part "partlist/bomConverter/lib/part"
	config "partlist/bomConverter/utilis"
	"strings"
	"time"
)

// PartService handles HTTP requests to the server.
type PartService struct {
	BaseURL    string
	HTTPClient *http.Client
}

type DescriptionService struct {
	BaseURL    string
	HTTPClient *http.Client
}

type User struct {
	UserName string
	Password string
}

type PartDescription struct {
	PartNumber  string `json:"part_number"`
	Description string `json:"description,omitempty"`
}

type DataDescription struct {
	Data []PartDescription `json:"data"`
}

var Token string

func GetToken() string {
	return Token
}

func SetToken(token string) {
	Token = token
}

// NewClientService creates a new ClientService instance.
func NewClientService(baseURL string) *PartService {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &PartService{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second, // Set a timeout for HTTP requests
		},
	}
}

// makeRequest performs an HTTP request.
func (cs *PartService) makeRequest(method, endpoint string, data map[string]interface{}, headers map[string]string) ([]byte, error) {
	url := cs.BaseURL + strings.TrimPrefix(endpoint, "/")
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshalling JSON: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := cs.HTTPClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, fmt.Errorf("connection error: Could not connect to %s. Is the Go server running? Details: %w", url, err)
		}
		return nil, fmt.Errorf("an unexpected request error occurred: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP Error: %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (cs *PartService) RequestDescription(data []part.Part) ([]part.Part, error) {

	config := config.GetConfig()
	var requestPartNumber DataDescription

	for _, part := range data {
		partNumber := PartDescription{
			PartNumber: part.PartNumber,
		}
		requestPartNumber.Data = append(requestPartNumber.Data, partNumber)
	}

	requestMap := make(map[string]any)
	partBytes, _ := json.Marshal(requestPartNumber)
	json.Unmarshal(partBytes, &requestMap)

	headers := map[string]string{
		"content-Type": "application/json",
	}

	fmt.Printf("request to url %v", requestMap)
	fmt.Printf("config.DescriptionUrl %v", config.DescriptionUrl)

	response, err := cs.makeRequest("POST", config.DescriptionUrl, requestMap, headers)
	if err != nil {
		return []part.Part{}, fmt.Errorf("error reading response body: %w", err)
	}

	var res DataDescription
	if err := json.Unmarshal(response, &res); err != nil {
		return []part.Part{}, fmt.Errorf("error parsing response: %w", err)
	}

	partDescMap := make(map[string]string)

	for _, data := range res.Data {
		partDescMap[data.PartNumber] = data.Description
	}

	var newPartDescription []part.Part

	for _, oldPart := range data {
		oldPart.Description = partDescMap[oldPart.PartNumber]
		newDescription := oldPart
		newPartDescription = append(newPartDescription, newDescription)
	}

	return newPartDescription, nil
}

// PostPartJSON sends a POST request with part data.
func (cs *PartService) PostPartJSON(data part.Part) ([]byte, error) {

	var Bearer string = "Bearer " + Token

	headers := map[string]string{
		"content-Type":  "application/json",
		"authorization": Bearer,
	}

	// Convert Part struct to a map[string]interface{} for makeRequest
	partMap := make(map[string]any)
	partBytes, _ := json.Marshal(data)
	json.Unmarshal(partBytes, &partMap)

	return cs.makeRequest("POST", "api/part", partMap, headers)
}

// PostPartJSON sends a POST request with part data.
func (cs *PartService) PostLogin(userName string, password string) ([]byte, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	// Convert Part struct to a map[string]interface{} for makeRequest

	user := User{
		UserName: userName,
		Password: password,
	}

	userMap := map[string]any{
		"userName": user.UserName,
		"password": user.Password,
	}
	return cs.makeRequest("POST", "api/login", userMap, headers)
}
