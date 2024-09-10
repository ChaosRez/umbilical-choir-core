package poller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"umbilical-choir-core/internal/app/config"
)

type PollRequest struct {
	ID               string      `json:"id"`
	GeographicArea   orb.Polygon `json:"geographic_area"`
	NumberOfChildren int         `json:"number_of_children"`
}

type PollResponse struct {
	ID         string `json:"id"`
	NewRelease string `json:"new_release"`
}

const PollInterval = 5 * time.Second

func PollParent(host, port, id string, serviceArea orb.Polygon) PollResponse {
	url := fmt.Sprintf("http://%s:%s/poll", host, port)

	request := map[string]interface{}{
		"id":                 id,
		"number_of_children": 0,
		"geographic_area":    geojson.NewGeometry(serviceArea),
	}

	for { //retry
		jsonData, err := json.Marshal(request)
		if err != nil {
			log.Errorf("Failed to marshal request: %v", err)
			continue
		}
		//log.Debugf("HTTP request payload: %s", string(jsonData))

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Errorf("Failed to poll parent: %v", err)
			time.Sleep(PollInterval)
			continue
		}
		defer resp.Body.Close()

		var response PollResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			log.Errorf("Failed to decode response (%v): %v", resp.StatusCode, err)
			log.Errorf("Response: %v", resp)
			time.Sleep(PollInterval)
			continue
		}

		return response
		//time.Sleep(PollInterval) // Poll periodically
	}
}

func DownloadRelease(cfg *config.Config, endpoint string) error {
	url := fmt.Sprintf("http://%s:%s%s", cfg.Parent.Host, cfg.Parent.Port, endpoint)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download release: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download release: received status code %d", resp.StatusCode)
	}

	// Create the directory if it doesn't exist
	dir := "releases"
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Save the file with a timestamp
	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(dir, fmt.Sprintf("release_%s.yml", timestamp))
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}

	log.Infof("Release downloaded and saved to %s", filePath)
	return nil
}
