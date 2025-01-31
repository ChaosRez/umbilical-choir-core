package faas

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	"strings"
)

var tfRuntimes = map[string]string{
	"python": "python3",
	"nodejs": "nodejs",
	"go":     "go",
}

type TinyFaaSAdapter struct {
	TF              *TinyFaaS.TinyFaaS
	tfProxyEndpoint string // i.e. 'host.docker.internal' or '172.17.0.1'
}

func NewTinyFaaSAdapter(tf *TinyFaaS.TinyFaaS, dockerHostAddress string) *TinyFaaSAdapter {
	return &TinyFaaSAdapter{
		TF:              tf,
		tfProxyEndpoint: fmt.Sprintf("http://%s:%s", dockerHostAddress, "8000"),
	}
}

func (t *TinyFaaSAdapter) WipeFunctions() error {
	return t.TF.WipeFunctions()
}

func (t *TinyFaaSAdapter) Functions() (string, error) {
	return t.TF.Functions()
}

func (t *TinyFaaSAdapter) FunctionExists(funcName string) (bool, error) {
	functionsList, err := t.TF.Functions()
	if err != nil {
		return false, fmt.Errorf("error retrieving functions list: %v", err)
	}

	functions := strings.Split(functionsList, "\n")
	for _, function := range functions {
		if function == funcName {
			return true, nil
		}
	}
	return false, nil
}

func (t *TinyFaaSAdapter) FunctionUri(funcName string) (string, error) {
	return fmt.Sprintf("http://%s:%s/%s", "172.17.0.1", "8000", funcName), nil // FIXME: host and port is different from the config (mangement from localhost/remote). the host should be accessible localhost from docker env and port is 8000 and not the management 8080 port
}

func (t *TinyFaaSAdapter) Close() error {
	return nil // no need to close anything
}

func (t *TinyFaaSAdapter) Log() (string, error) {
	return t.TF.ResultsLog()
}

func (t *TinyFaaSAdapter) Upload(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error) {
	tfRuntime, exists := tfRuntimes[runtime]
	if !exists {
		return "", fmt.Errorf("runtime '%s' not supported", runtime)
	}

	// Adapt the code for tinyFaaS
	adaptedCode, errf := adaptFunction(path, "tinyfaas", runtime)
	if errf != nil {
		return "", fmt.Errorf("error adapting function: %v", errf)
	}

	_, err := t.TF.UploadLocal(funcName, adaptedCode, tfRuntime, 1, isFullPath, args)
	return fmt.Sprintf("%s/%s", t.tfProxyEndpoint, funcName), err
}

func (t *TinyFaaSAdapter) Update(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error) {
	return t.Upload(funcName, path, runtime, entryPoint, isFullPath, args) // same as upload
}

func (t *TinyFaaSAdapter) Delete(funcName string) error {
	return t.TF.Delete(funcName)
}
