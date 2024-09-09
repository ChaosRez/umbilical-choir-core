package config

import (
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	StrategyPath string `yaml:"strategyPath,omitempty"`
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

func LoadConfig(path string) (*Config, error) {
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

func InitLogger(logLevel string) {
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		ll = log.DebugLevel
	}
	log.SetLevel(ll)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "15:04:05.000", FullTimestamp: true})
}
