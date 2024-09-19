package config

import (
	"encoding/json"
	"errors"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
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
		Host        string `yaml:"host"`
		ServiceArea string `yaml:"service_area"`
	} `yaml:"agent"`
	Parent struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
	} `yaml:"parent"`
	LogLevel string `yaml:"logLevel"`
}

func LoadConfig(path string) (*Config, error) {
	log.Infof("Loading config from %s", path)
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

// Set logger's level (from config) and format
func InitLogger(logLevel string) {
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "15:04:05.000", FullTimestamp: true})
}

// Helper fuction to unmarshall the service area polygon
func (cfg *Config) StrAreaToPolygon() (orb.Polygon, error) {
	log.Debug("Parsing service area from config")
	var fc geojson.FeatureCollection
	err := json.Unmarshal([]byte(cfg.Agent.ServiceArea), &fc)
	if err != nil {
		return nil, err
	}

	if len(fc.Features) == 0 {
		return nil, errors.New("Service area is empty or invalid")
	}

	polygon, ok := fc.Features[0].Geometry.(orb.Polygon)
	if !ok {
		return nil, errors.New("Service area is not a valid polygon")
	}

	return polygon, nil
}
