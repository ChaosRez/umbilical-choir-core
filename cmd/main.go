package main

import (
	"context"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	log "github.com/sirupsen/logrus"
	"umbilical-choir-core/internal/app/config"
	FaaS "umbilical-choir-core/internal/app/faas"
	Manager "umbilical-choir-core/internal/app/manager"
	Strategy "umbilical-choir-core/internal/app/strategy"
	GCP "umbilical-choir-core/internal/pkg/gcp"
)

var cfg *config.Config

func main() {
	var faasAdapter FaaS.FaaS
	switch cfg.FaaS.Type {
	case "tinyfaas":
		tf := TinyFaaS.New(cfg.FaaS.Host, cfg.FaaS.Port, "")
		tf.WipeFunctions()
		faasAdapter = FaaS.NewTinyFaaSAdapter(tf, cfg.FaaS.ProxyHost)
	case "gcp":
		ctx := context.Background()
		gcp, err := GCP.NewGCP(ctx, cfg.FaaS.ProjectID, cfg.FaaS.Location)
		if err != nil {
			log.Fatalf("Failed to initialize GCP client: %v", err)
		}
		defer gcp.Close()
		faasAdapter = &FaaS.GCPAdapter{GCP: gcp}
	default:
		log.Fatalf("Unknown FaaS type: %s", cfg.FaaS.Type)
	}
	manager := Manager.New(faasAdapter, cfg.Agent.Host)

	if cfg.StrategyPath == "" {
		log.Fatalf("no strategy")
	} else {
		log.Warnf("running the strategy from config. StrategyPath: %s", cfg.StrategyPath)
		strategy, err := Strategy.LoadStrategy(cfg.StrategyPath)
		if err != nil {
			log.Fatalf("Failed to load strategy: %v", err)
		}
		manager.RunReleaseStrategy(strategy)
	}
}

func init() {
	var err error
	cfg, err = config.LoadConfig("config/config.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	config.InitLogger(cfg.LogLevel)
}
