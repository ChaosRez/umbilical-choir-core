package faas

type FaaS interface {
	WipeFunctions() error
	Functions() (string, error)
	Close() error
	Log() (string, error)
	// Call(funcName string, data string) (string, error)
	Upload(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error)
	Update(funcName, path, runtime string, entryPoint string, isFullPath bool, args []string) (string, error)
	Delete(funcName string) error
}
