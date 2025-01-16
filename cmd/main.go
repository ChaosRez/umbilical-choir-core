package main

import (
	"context"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	log "github.com/sirupsen/logrus"
	"time"
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
		//tf.WipeFunctions()
		faasAdapter = FaaS.NewTinyFaaSAdapter(tf, cfg.FaaS.ProxyHost)
	case "gcp":
		ctx := context.Background()
		gcp, err := GCP.NewGCP(ctx, cfg.FaaS.ProjectID, cfg.FaaS.Location, cfg.FaaS.Credentials)
		if err != nil {
			log.Fatalf("Failed to initialize GCP client: %v", err)
		}
		defer gcp.Close()
		faasAdapter = &FaaS.GCPAdapter{GCP: gcp}
	default:
		log.Fatalf("Unsupported FaaS type: %s", cfg.FaaS.Type)
	}
	manager := Manager.New(faasAdapter, cfg)

	if cfg.StrategyPath == "" { // default behavior
		pollRes := Poller.PollParent(cfg.Parent.Host, cfg.Parent.Port, "", manager.ServiceAreaPolygon)
		manager.ID = pollRes.ID
		for {
			if pollRes.NewReleaseID == "" {
				log.Debugf("No new release strategy available for me")
			} else {
				log.Infof("New release available at '%s'", pollRes.NewReleaseID)
				strategyPath, err := Poller.DownloadRelease(cfg, manager.ID, pollRes.NewReleaseID)
				if err != nil {
					log.Fatalf("Failed to download release: %v", err)
				}
				strategy, err := Strategy.LoadStrategy(strategyPath)
				if err != nil {
					log.Fatalf("Failed to load strategy: %v", err)
				}
				fnsPath, err := Poller.DownloadReleaseFunctions(cfg, strategy.ID)
				if err != nil {
					log.Fatalf("Failed to download functions: %v", err)
				}
				log.Debugf("Functions downloaded to: %s", fnsPath)
				manager.RunReleaseStrategy(strategy) // sends the result to the parent
				//break
			}
			time.Sleep(3 * time.Second)
			pollRes = Poller.PollParent(cfg.Parent.Host, cfg.Parent.Port, manager.ID, manager.ServiceAreaPolygon)
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
