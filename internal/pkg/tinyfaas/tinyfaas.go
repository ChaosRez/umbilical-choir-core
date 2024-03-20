package tinyfaas

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
	"os/exec"
)

type TinyFaaS struct {
	Host string
	Port string
	Path string
}

func New(host, port string) *TinyFaaS {
	return &TinyFaaS{
		Host: host,
		Port: port,
		Path: "../faas/tinyfaas/",
	}
}

func (tf *TinyFaaS) UploadLocal(funcName string, subPath string, env string, threads int) (string, error) {
	//wiki: curl http://localhost:8080/upload --data "{\"name\": \"$2\", \"env\": \"$3\", \"threads\": $4, \"zip\": \"$(zip -r - ./* | base64 | tr -d '\n')\"}"
	//wiki: ./scripts/upload.sh "test/fns/sieve-of-eratosthenes" "sieve" "nodejs" 1

	// parse the function source code to base64
	cmdStr := "zip -r - ./* | base64 | tr -d '\n'"
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = tf.Path + subPath
	var zip bytes.Buffer
	cmd.Stdout = &zip
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	// make a resty client
	client := resty.New()

	// make json parameters
	params := map[string]interface{}{
		"name":    funcName,
		"env":     env,
		"threads": threads,
		"zip":     zip.String(),
		"envs":    []string{"HOST=172.17.0.1"},
	}
	jsonBody, err := json.Marshal(params)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return "", err
	}

	// call and get the response
	callResponse := func() (*resty.Response, error) {
		resp, err := client.R().
			EnableTrace().
			SetBody(jsonBody).
			Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "upload"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// validate the response
	resp, err := checkResponse(callResponse)
	if err != nil {
		log.Fatalf("Error uploading '%s' function via local func: %v ", funcName, err)
	}
	return resp, nil
}

func (tf *TinyFaaS) UploadURL(funcName string, subPath string, env string, threads int, url string) (string, error) {
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
		"envs":           []string{"HOST=172.17.0.1"},
	}
	jsonBody, err := json.Marshal(params)
	if err != nil {
		log.Errorf("Error marshaling JSON: %v", err)
		return "", err
	}

	// call and get the response
	callResponse := func() (*resty.Response, error) {
		resp, err := client.R().
			EnableTrace().
			SetBody(jsonBody).
			Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "uploadURL"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// validate the response
	resp, err := checkResponse(callResponse)
	if err != nil {
		log.Fatalf("Error uploading '%s' function via URL: %v", funcName, err)
	}
	log.Infof("'%s' deployed successfully \n", funcName)
	return resp, nil
}

func (tf *TinyFaaS) Delete(funcName string) error {
	//wiki: curl http://localhost:8080/delete --data "{\"name\": \"$1\"}"

	// make a resty client
	client := resty.New()

	// make json parameters
	params := map[string]interface{}{
		"name": funcName,
	}
	jsonBody, err := json.Marshal(params)
	if err != nil {
		log.Errorf("Error marshaling JSON: %v", err)
		return err
	}

	// call and get the response
	callResponse := func() (*resty.Response, error) {
		resp, err := client.R().
			EnableTrace().
			SetBody(jsonBody).
			Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "delete"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// validate the response
	_, err = checkResponse(callResponse)
	if err != nil {
		log.Fatalf("Error when deleting '%s' function: %v", funcName, err)
	}
	log.Infof("deleting '%s' function success \n", funcName)
	return nil
}

func (tf *TinyFaaS) ResultsLog() (string, error) {
	// make a resty client
	client := resty.New()

	// call and get the response
	callResponse := func() (*resty.Response, error) {
		resp, err := client.R().
			EnableTrace().
			Get(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "logs"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// validate the response
	resp, err := checkResponse(callResponse)
	if err != nil {
		log.Fatalf("Error when getting results log: %v", err)
	}
	return resp, err
}

func (tf *TinyFaaS) WipeFunctions() {
	// make a resty client
	client := resty.New()

	// call and get the response
	callResponse := func() (*resty.Response, error) {
		resp, err := client.R().
			EnableTrace().
			Post(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "wipe"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// validate the response
	_, err := checkResponse(callResponse)
	if err != nil {
		log.Fatalf("Error when wiping functions: %v", err)
	}
	log.Info("wiping functions success")
	return

}

// Functions lists available functions
func (tf *TinyFaaS) Functions() string {
	// make a resty client
	client := resty.New()

	// call and get the response
	callResponse := func() (*resty.Response, error) {
		resp, err := client.R().
			EnableTrace().
			Get(fmt.Sprintf("http://%s:%s/%s", tf.Host, tf.Port, "list"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// validate the response
	resp, err := checkResponse(callResponse)
	if err != nil {
		log.Fatalf("Error when getting functions list: %v", err)
	}
	return resp
}

// Private
func checkResponse(fn func() (*resty.Response, error)) (string, error) {
	resp, err := fn()
	if err != nil {
		return "", err
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("non-successful response (%d)", resp.StatusCode())
		return "", errors.New(msg)
	}
	return string(resp.Body()), nil
}

/** Sample

// initialize tinyFaaS manager instance
	tf := TinyFaaS.New("localhost", "8080")

// upload a function
respU, err := tf.UploadLocal("sieve", "test/fns/sieve-of-eratosthenes", "nodejs", 1)
//respU, err := tf.UploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")

// Check tinyFaaS response. (inc. timeout, ok, other errors?)
if err != nil {
log.Fatalln("error when calling tinyFaaS: ", err)
}
// Print the response
log.Infof("upload success:\n%s", respU) // 200 success, 400 bad request

// delete a function
errD := tf.Delete("sieve")

// Check tinyFaaS response. (inc. timeout, ok, other errors?)
if errD != nil {
log.Fatalln("error when calling tinyFaaS: ", errD)
}

// get results log
respL, errL := tf.ResultsLog()

// Check tinyFaaS response. (inc. timeout, ok, other errors?)
if errL != nil {
log.Fatalln("error when calling tinyFaaS: ", errL)
}
// Print the response
fmt.Printf("results log: %s\n", respL) // 200 success

// list functions
respF := tf.Functions()
// Print the response
fmt.Printf("functions: %s\n", respF)

// wipe functions
tf.WipeFunctions()

*/
