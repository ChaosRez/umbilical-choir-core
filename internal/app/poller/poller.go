package poller

import (
	"archive/zip"
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
	"strings"
	"time"
	"umbilical-choir-core/internal/app/config"
)

type PollRequest struct {
	ID               string      `json:"id"`
	GeographicArea   orb.Polygon `json:"geographic_area"`
	NumberOfChildren int         `json:"number_of_children"`
}

type PollResponse struct {
	ID           string `json:"id"`
	NewReleaseID string `json:"new_release"`
}

const PollInterval = 5 * time.Second

func PollParent(host, port, id string, serviceArea orb.Polygon) PollResponse {
	url := fmt.Sprintf("http://%s:%s/poll", host, port)
	log.Infof("Polling parent at %s", url)

	request := map[string]interface{}{
		"id":                 id,
		"number_of_children": 0, // Leaf node
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

// DownloadRelease downloads the release file from the parent, where enpoint is given by the parent
func DownloadRelease(cfg *config.Config, id, releaseID string) (string, error) {
	url := fmt.Sprintf("http://%s:%s/release?childID=%s&releaseID=%s", cfg.Parent.Host, cfg.Parent.Port, id, releaseID)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download release: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %v", err)
		}
		return "", fmt.Errorf("failed to download release: received status code %d: %s", resp.StatusCode, string(body))
	}

	// Create the directory if it doesn't exist
	dir := "releases"
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	// Save the file with a timestamp
	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(dir, fmt.Sprintf("release_%s.yml", timestamp))
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %v", err)
	}

	log.Infof("Release downloaded and saved to %s", filePath)
	return filePath, nil
}

// TODO check if function subdirectories defined in release.yml exist
// DownloadReleaseFunctions downloads the functions zip file from the parent to "fns" (name of the zip file), where id is defined in release.yml
func DownloadReleaseFunctions(cfg *config.Config, releaseID string) (string, error) {
	url := fmt.Sprintf("http://%s:%s/release/functions/%s", cfg.Parent.Host, cfg.Parent.Port, releaseID)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download release's functions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download release's functions: received status code %d", resp.StatusCode)
	}

	// Create the directory if it doesn't exist
	dir := ""
	//if err := os.MkdirAll(dir, os.ModePerm); err != nil {
	//	return "", fmt.Errorf("failed to create directory: %v", err)
	//}

	// Save the zipfile to a temporary file
	tmpFile, err := os.CreateTemp("", "functions-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary zip file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save zip file: %v", err)
	}
	tmpFile.Close() // Close the file before unzipping

	// Unzip the zipfile to fns directory
	zipReader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to open temp zip file: %v", err)
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		// Ignore macOS metadata files
		if strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		fpath := filepath.Join(dir, f.Name)

		//// Check for ZipSlip (Directory traversal)
		//if !strings.HasPrefix(fpath, filepath.Clean(dir)+string(os.PathSeparator)) {
		//	return "", fmt.Errorf("%s: illegal file path", fpath)
		//}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return "", fmt.Errorf("failed to create file %s: %v", fpath, err)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return "", fmt.Errorf("failed to open file %s in zip: %v", f.Name, err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return "", fmt.Errorf("failed to copy file %s from zip: %v", f.Name, err)
		}
	}

	return dir, nil // Return the directory where files were unzipped
}

// PollForSignal polls for a signal to end a stage test. NOTE it runs on a separate goroutine
func PollForSignal(host, port, id, strategyID, stageName string) (bool, error) {
	url := fmt.Sprintf("http://%s:%s/end_stage", host, port)
	request := map[string]interface{}{
		"id":          id,
		"strategy_id": strategyID,
		"stage_name":  stageName,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		log.Errorf("Failed to marshal request: %v", err)
		return false, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Errorf("Failed to poll for signal: %v", err)
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("(%s) received non-OK HTTP status: %v", resp.Status, string(body))
	}

	//log.Debugf("Received response: %v", resp)
	var response struct {
		EndTest bool `json:"end_stage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Errorf("Failed to decode response: %v", err)
		return false, err
	}
	log.Debugf("EndTest: %v", response.EndTest)

	return response.EndTest, nil
}
