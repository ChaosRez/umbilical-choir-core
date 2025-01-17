package faas

import (
	"context"
	"fmt"
	"strings"
	GCP "umbilical-choir-core/internal/pkg/gcp"
)

var gcpRuntimes = map[string]string{
	"python": "python312",
	"nodejs": "nodejs20",
}

type GCPAdapter struct {
	GCP *GCP.GCP
}

func (g *GCPAdapter) WipeFunctions() error {
	return fmt.Errorf("WipeFunctions not implemented for GCP")
}

func (g *GCPAdapter) Functions() (string, error) {
	return "", fmt.Errorf("Functions not implemented for GCP")
}

func (g *GCPAdapter) FunctionExists(funcName string) (bool, error) {
	ctx := context.Background()
	function := &GCP.Function{
		Name:     funcName,
		Location: g.GCP.Location,
	}
	_, err := g.GCP.GetFunction(ctx, function)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("error checking if function '%s' exists: %v", funcName, err)
	}
	return true, nil
}

func (g *GCPAdapter) Close() error {
	return g.Close()
}
func (g *GCPAdapter) Log() (string, error) {
	return "", fmt.Errorf("Log not implemented for GCP")
}

func (g *GCPAdapter) Upload(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error) {
	gcpRuntime, exists := gcpRuntimes[runtime]
	if !exists {
		return "", fmt.Errorf("runtime '%s' not supported", runtime)
	}
	ctx := context.Background()

	// Adapt the code for GCP
	adaptedCode, err := adaptFunction(path, "gcp", runtime)
	if err != nil {
		return "", fmt.Errorf("error adapting function: %v", err)
	}

	function := &GCP.Function{
		Name:                 funcName,
		SourceLocalPath:      adaptedCode,
		Runtime:              gcpRuntime,
		EnvironmentVariables: map[string]string{},
		EntryPoint:           entryPoint,
		Location:             g.GCP.Location,
	}
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			function.EnvironmentVariables[parts[0]] = parts[1]
		}
	}
	return g.GCP.CreateFunction(ctx, function)
}

func (g *GCPAdapter) Update(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error) {
	gcpRuntime, exists := gcpRuntimes[runtime]
	if !exists {
		return "", fmt.Errorf("runtime '%s' not supported", runtime)
	}
	ctx := context.Background()

	// Adapt the code for GCP
	adaptedCode, err := adaptFunction(path, "gcp", runtime)
	if err != nil {
		return "", fmt.Errorf("error adapting function: %v", err)
	}

	function := &GCP.Function{
		Name:                 funcName,
		SourceLocalPath:      adaptedCode,
		Runtime:              gcpRuntime,
		EnvironmentVariables: map[string]string{},
		EntryPoint:           entryPoint,
		Location:             g.GCP.Location,
	}
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			function.EnvironmentVariables[parts[0]] = parts[1]
		}
	}
	return g.GCP.UpdateFunction(ctx, function)
}

func (g *GCPAdapter) Delete(funcName string) error {
	ctx := context.Background()
	function := &GCP.Function{
		Name:     funcName,
		Location: g.GCP.Location,
	}
	return g.GCP.DeleteFunction(ctx, function)
}
