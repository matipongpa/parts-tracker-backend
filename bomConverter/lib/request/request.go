package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	part "partlist/bomConverter/lib/part"
	"strings"
	"time"
)

// ClientService handles HTTP requests to the server.
type ClientService struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClientService creates a new ClientService instance.
func NewClientService(baseURL string) *ClientService {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &ClientService{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second, // Set a timeout for HTTP requests
		},
	}
}

// makeRequest performs an HTTP request.
func (cs *ClientService) makeRequest(method, endpoint string, data map[string]interface{}, headers map[string]string) ([]byte, error) {
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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP Error: %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// PostPartJSON sends a POST request with part data.
func (cs *ClientService) PostPartJSON(data part.Part) ([]byte, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	// Convert Part struct to a map[string]interface{} for makeRequest
	partMap := make(map[string]any)
	partBytes, _ := json.Marshal(data)
	json.Unmarshal(partBytes, &partMap)

	return cs.makeRequest("POST", "api/part", partMap, headers)
}
