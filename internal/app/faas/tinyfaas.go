package faas

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
)

var tfRuntimes = map[string]string{
	"python": "python3",
	"nodejs": "nodejs",
}

const tfEndpoint = "http://172.17.0.1:8000"

//const tfEndpoint = "http://host.docker.internal:8000" // docker desktop

type TinyFaaSAdapter struct {
	TF *TinyFaaS.TinyFaaS
}

func (t *TinyFaaSAdapter) WipeFunctions() error {
	return t.TF.WipeFunctions()
}

func (t *TinyFaaSAdapter) Functions() (string, error) {
	return t.TF.Functions()
}

func (t *TinyFaaSAdapter) Close() error {
	return nil // no need to close anything
}

func (t *TinyFaaSAdapter) Log() (string, error) {
	return t.TF.ResultsLog()
}

func (t *TinyFaaSAdapter) UploadFunction(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error) {
	tfRuntime, exists := tfRuntimes[runtime]
	if !exists {
		return "", fmt.Errorf("runtime '%s' not supported", runtime)
	}
	_, err := t.TF.UploadLocal(funcName, path, tfRuntime, 1, isFullPath, args)
	return fmt.Sprintf("%s/%s", tfEndpoint, funcName), err
}

func (t *TinyFaaSAdapter) Update(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error) {
	tfRuntime, exists := tfRuntimes[runtime]
	if !exists {
		return "", fmt.Errorf("runtime '%s' not supported", runtime)
	}
	_, err := t.TF.UploadLocal(funcName, path, tfRuntime, 1, isFullPath, args) // same as upload
	return fmt.Sprintf("%s/%s", tfEndpoint, funcName), err
}

func (t *TinyFaaSAdapter) Delete(funcName string) error {
	return t.TF.Delete(funcName)
}
