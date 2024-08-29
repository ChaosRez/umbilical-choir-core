package faas

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const jsFileName = "index.js"
const pyFileName = "fn.py"

func adaptFunction(path, platform, runtime string) (string, error) {
	log.Debug("Creating a temporary directory with a timestamp")
	timestamp := time.Now().Format("20060102150405")
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("adapted_function_%s_", timestamp))
	if err != nil {
		return "", fmt.Errorf("error creating temp directory: %v", err)
	}

	log.Debug("Copying all files from the provided path to the temporary directory: ", tempDir)
	err = copyDir(path, tempDir)
	if err != nil {
		return "", fmt.Errorf("error copying files to temp directory: %v", err)
	}

	if runtime == "nodejs" {
		log.Debugf("Reading the contents of the %s file from the temporary directory", jsFileName)
		tempIndexFilePath := filepath.Join(tempDir, jsFileName)
		jsCode, err := os.ReadFile(tempIndexFilePath)
		if err != nil {
			return "", fmt.Errorf("error reading file: %v", err)
		}

		log.Debug("Removing the outer block from the jsFileName file")
		re := regexp.MustCompile(`(?s)exports\.\w+\s*=\s*\(req,\s*res\)\s*=>\s*{(.*)}`)
		matches := re.FindStringSubmatch(string(jsCode))
		if len(matches) < 2 {
			return "", fmt.Errorf("invalid function format. 'req' and 'res' parameters are required for js")
		}
		innerCode := matches[1]

		log.Debug("Adapting the js code based on the platform")
		var adaptedCode string
		switch platform {
		case "tinyfaas":
			adaptedCode = fmt.Sprintf(`module.exports = (req, res) => {
%s
}`, indent(innerCode, 1))
		case "gcp":
			adaptedCode = fmt.Sprintf(`exports.http = (req, res) => {
%s
}`, indent(innerCode, 1))
		default:
			return "", fmt.Errorf("unsupported platform: %s", platform)
		}

		log.Debugf("Writing the adapted code to the %v file in the temporary directory", jsFileName)
		err = os.WriteFile(tempIndexFilePath, []byte(adaptedCode), 0644)
		if err != nil {
			return "", fmt.Errorf("error writing adapted js code to temp file: %v", err)
		}
	} else if runtime == "python" {
		if platform == "gcp" {
			log.Debug("Creating main.py for GCP")
			mainPyPath := filepath.Join(tempDir, "main.py")
			mainPyContent := `def http(request):
    request_bytes = request.data.decode("utf-8")
    request_args = request.args
    from fn import fn
    return fn(request_bytes, request_args)
`
			err = os.WriteFile(mainPyPath, []byte(mainPyContent), 0644)
			if err != nil {
				return "", fmt.Errorf("error writing main.py: %v", err)
			}
		} else if platform == "tinyfaas" {
			log.Debug("Assuming the python function is already in tinyFaaS format")
			return tempDir, nil
		}
	}

	log.Infof("Successfully adapted the source for platform: %s and runtime: %s", platform, runtime)
	return tempDir, nil
}

func indent(code string, level int) string {
	indentation := strings.Repeat("  ", level)
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = indentation + line
	}
	return strings.Join(lines, "\n")
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return err
	}

	return destinationFile.Sync()
}
