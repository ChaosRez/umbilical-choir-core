package main

import (
	"context"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	log "github.com/sirupsen/logrus"
	"umbilical-choir-core/internal/app/config"
	FaaS "umbilical-choir-core/internal/app/faas"
	Manager "umbilical-choir-core/internal/app/manager"
	Poller "umbilical-choir-core/internal/app/poller"
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
		log.Fatalf("Unsupported FaaS type: %s", cfg.FaaS.Type)
	}
	servArea, err := cfg.StrAreaToPolygon()
	if err != nil {
		log.Fatalf("Failed to parse service area: %v", err)
	}
	manager := Manager.New(faasAdapter, cfg.Agent.Host, servArea)

	if cfg.StrategyPath == "" { // default behavior
		pollRes := Poller.PollParent(cfg.Parent.Host, cfg.Parent.Port, "", manager.ServiceAreaPolygon)
		manager.ID = pollRes.ID
		if pollRes.NewRelease == "" {
			// TODO: call PollParent with the manager.ID
			log.Fatalf("TODO: call PollParent with the manager.ID")
		} else {
			log.Infof("New release available at '%s'", pollRes.NewRelease)
			err := Poller.DownloadRelease(cfg, pollRes.NewRelease)
			if err != nil {
				log.Fatalf("Failed to download release: %v", err)
			}
			strategy, err := Strategy.LoadStrategy(strategyPath)
			if err != nil {
				log.Fatalf("Failed to load strategy: %v", err)
			}
			manager.RunReleaseStrategy(strategy)
			// TODO sent result to parent
		}
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
