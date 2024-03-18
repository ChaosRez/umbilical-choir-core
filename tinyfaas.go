package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"os"
	"os/exec"
)

type TinyFaaS struct {
	Host string
	Port string
	Path string
}

func NewTinyFaaS(host, port string) *TinyFaaS {
	return &TinyFaaS{
		Host: host,
		Port: port,
		Path: "../faas/tinyfaas/",
	}
}

func (tf *TinyFaaS) uploadLocal(funcName string, subPath string, env string, threads int) (*resty.Response, error) {
	//wiki: curl http://localhost:8080/upload --data "{\"name\": \"$2\", \"env\": \"$3\", \"threads\": $4, \"zip\": \"$(zip -r - ./* | base64 | tr -d '\n')\"}"
	//wiki: ./scripts/upload.sh "test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1

	// switch to function directory in tinyfaas
	err := os.Chdir(tf.Path + subPath)
	if err != nil {
		return nil, err
	}

	// parse the function source code to base64
	cmdStr := "zip -r - ./* | base64 | tr -d '\n'"
	cmd := exec.Command("bash", "-c", cmdStr)
	var zip bytes.Buffer
	cmd.Stdout = &zip
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	// make a resty client
	client := resty.New()

	// make json parameters
	params := map[string]interface{}{
		"name":    funcName,
		"env":     env,
		"threads": threads,
		"zip":     zip.String(),
	}
	jsonBody, err := json.Marshal(params)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return nil, err
	}

	// call and return the result
	return client.R().
		EnableTrace().
		SetBody(jsonBody).
		Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "upload"))
}

func (tf *TinyFaaS) uploadURL(funcName string, subPath string, env string, threads int, url string) (*resty.Response, error) {
	//wiki: curl http://localhost:8080/uploadURL --data "{\"name\": \"$3\", \"env\": \"$4\",\"threads\": $5,\"url\": \"$1\",\"subfolder_path\": \"$2\"}"
	//wiki: uploadURL.sh "https://github.com/OpenFogStack/tinyFaas/archive/main.zip" "tinyFaaS-main/test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1

	// make a resty client
	client := resty.New()

	// make json parameters
	params := map[string]interface{}{
		"name":           funcName,
		"env":            env,
		"threads":        threads,
		"url":            url,
		"subfolder_path": subPath,
	}
	jsonBody, err := json.Marshal(params)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return nil, err
	}

	// call and return the result
	return client.R().
		EnableTrace().
		SetBody(jsonBody).
		Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "uploadURL"))
}

func (tf *TinyFaaS) delete(funcName string) (*resty.Response, error) {
	//wiki: curl http://localhost:8080/delete --data "{\"name\": \"$1\"}"

	// make a resty client
	client := resty.New()

	// make json parameters
	params := map[string]interface{}{
		"name": funcName,
	}
	jsonBody, err := json.Marshal(params)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return nil, err
	}

	// call and return the result
	return client.R().
		EnableTrace().
		SetBody(jsonBody).
		Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "delete"))
}

func (tf *TinyFaaS) resultsLog() (*resty.Response, error) {
	// make a resty client
	client := resty.New()
	// call and return the result
	return client.R().
		EnableTrace().
		Get(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "logs"))
}

func (tf *TinyFaaS) wipeFunctions() (*resty.Response, error) {
	// make a resty client
	client := resty.New()
	// call and return the result
	return client.R().
		EnableTrace().
		Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "wipe"))
}

// lists functions
func (tf *TinyFaaS) functions() (*resty.Response, error) {
	// make a resty client
	client := resty.New()
	// call and return the result
	return client.R().
		EnableTrace().
		Get(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "list"))
}
