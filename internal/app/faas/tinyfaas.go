package faas

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
)

var tfRuntimes = map[string]string{
	"python": "python3",
	"nodejs": "nodejs",
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
	tfRuntime, exists := tfRuntimes[runtime]
	if !exists {
		return "", fmt.Errorf("runtime '%s' not supported", runtime)
	}

	// Adapt the code for tinyFaaS
	adaptedCode, errf := adaptFunction(path, "tinyfaas", runtime)
	if errf != nil {
		return "", fmt.Errorf("error adapting function: %v", errf)
	}

	_, err := t.TF.UploadLocal(funcName, adaptedCode, tfRuntime, 1, isFullPath, args) // same as upload
	return fmt.Sprintf("%s/%s", t.tfProxyEndpoint, funcName), err
}

func (t *TinyFaaSAdapter) Delete(funcName string) error {
	return t.TF.Delete(funcName)
}
