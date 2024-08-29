package main

import (
	"context"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
	FaaS "umbilical-choir-core/internal/app/faas"
	Manager "umbilical-choir-core/internal/app/manager"
	GCP "umbilical-choir-core/internal/pkg/gcp"
)

var config *Config

type Config struct {
	StrategyPath string `yaml:"strategyPath"`
	FaaS         struct {
		Type      string `yaml:"type"`
		Host      string `yaml:"host,omitempty"`
		Port      string `yaml:"port,omitempty"`
		ProxyHost string `yaml:"proxyHost,omitempty"`
		ProjectID string `yaml:"projectID,omitempty"`
		Location  string `yaml:"location,omitempty"`
	} `yaml:"faas"`
	Agent struct {
		Host string `yaml:"host"`
	} `yaml:"agent"`
	LogLevel string `yaml:"logLevel"`
}

func main() {
	var faasAdapter FaaS.FaaS
	switch config.FaaS.Type {
	case "tinyfaas":
		tf := TinyFaaS.New(config.FaaS.Host, config.FaaS.Port, "")
		tf.WipeFunctions()
		faasAdapter = FaaS.NewTinyFaaSAdapter(tf, config.FaaS.ProxyHost)
	case "gcp":
		ctx := context.Background()
		gcp, err := GCP.NewGCP(ctx, config.FaaS.ProjectID, config.FaaS.Location)
		if err != nil {
			log.Fatalf("Failed to initialize GCP client: %v", err)
		}
		defer gcp.Close()
		faasAdapter = &FaaS.GCPAdapter{GCP: gcp}
	default:
		log.Fatalf("Unknown FaaS type: %s", config.FaaS.Type)
	}

	manager := Manager.New(faasAdapter, config.StrategyPath, config.Agent.Host)
	manager.RunReleaseStrategy()
}

func init() {
	var err error
	config, err = loadConfig("config/config_gcp.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ll, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		ll = log.DebugLevel
	}
	log.SetLevel(ll)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "15:04:05.000", FullTimestamp: true})
	//lvl, ok := os.LookupEnv("LOG_LEVEL")
	//// LOG_LEVEL not set, let's default to debug
	//if !ok {
	//	lvl = "debug"
	//}
	//// parse string, this is built-in feature of logrus
	//ll, err := log.ParseLevel(lvl)
	//if err != nil {
	//	ll = log.DebugLevel
	//}
	//// set global log level
	//log.SetLevel(ll)
	//
	//// Add timestamp to logrus
	//log.SetFormatter(&log.TextFormatter{TimestampFormat: "15:04:05.000", FullTimestamp: true})
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
