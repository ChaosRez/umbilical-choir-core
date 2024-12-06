package manager

import (
	"fmt"
	"github.com/paulmach/orb"
	log "github.com/sirupsen/logrus"
	"umbilical-choir-core/internal/app/config"
	FaaS "umbilical-choir-core/internal/app/faas"
	Strategy "umbilical-choir-core/internal/app/strategy"
	Tests "umbilical-choir-core/internal/app/tests"
)

type Manager struct {
	ID                 string
	FaaS               FaaS.FaaS
	Host               string
	ServiceAreaPolygon orb.Polygon
	ParentHost         string
	ParentPort         string
}

// New creates a new Manager instance
func New(faas FaaS.FaaS, cfg *config.Config) *Manager {
	servArea, err := cfg.StrAreaToPolygon()
	if err != nil {
		log.Fatalf("Failed to parse service area: %v", err)
	}
	return &Manager{
		FaaS:               faas,
		Host:               cfg.Agent.Host,
		ServiceAreaPolygon: servArea,
		ParentHost:         cfg.Parent.Host,
		ParentPort:         cfg.Parent.Port,
	}
}

// RunReleaseStrategy executes the given release strategy
func (m *Manager) RunReleaseStrategy(strategy *Strategy.ReleaseStrategy) {
	agentHost := m.Host
	for _, stage := range strategy.Stages {
		log.Infof("'%s': starting a '%s' stage for '%s' function", stage.Name, stage.Type, stage.FuncName)
		var nextStage *Strategy.Stage = nil
		var nextStageName = ""
		fMeta, err := strategy.GetFunctionByName(stage.FuncName)
		if err != nil {
			log.Fatalf("Error getting function: %v", err)
		}
		rollbackFuncVer, err := fMeta.GetVersionByName(strategy.Rollback.Action.Function)
		if err != nil {
			log.Fatalf("Error getting rollback function: %v", err)
		}

		switch stage.Type {
		case "A/B":
			testMeta, agg, err := Tests.ReleaseTest(stage, fMeta, agentHost, m.FaaS)
			if err != nil {
				log.Errorf("Error in ReleaseTest for '%s' function: %v", stage.FuncName, err)
				return
			}

			// Summarize metrics
			fmt.Printf(agg.SummarizeString())
			summary := agg.SummarizeResult()

			// Process the results of the release test, and set the summary.Status
			success, rollbackRequired := Tests.ProcessStageResult(stage, summary) // TODO if rollbackRequired, then break? what to report to parent?

			log.Infof("Running after test instructions. Checking if rollback is required...")
			nextStage, err = m.handleAfterTestInstructions(stage, testMeta, fMeta, strategy, agg, rollbackRequired, success, rollbackFuncVer)
			if err != nil {
				log.Errorf("Failed to handle after test instructions: %v", err)
				return
			}

			log.Infof("f1 response time: Min %vms, Max %vms", summary.F1TimesSummary.Minimum, summary.F1TimesSummary.Maximum)
			log.Infof("f2 response time: Min %vms, Max %vms", summary.F2TimesSummary.Minimum, summary.F2TimesSummary.Maximum)

			// Send result summary to parent
			if nextStage != nil {
				nextStageName = nextStage.Name
			}
			err = summary.SendResultSummary(strategy.ID, nextStageName, m.ID, m.ParentHost, m.ParentPort)
			if err != nil {
				log.Errorf("Failed to send result summary: %v", err)
			}
		case "WaitForSignal":
			// TODO: combine with normal releasetest. The only difference is the polling for signal + extera parameters needed
			testMeta, agg, err := Tests.ReleaseTestWithSignal(stage, fMeta, agentHost, m.FaaS, strategy.ID, m.ParentHost, m.ParentPort, m.ID)
			if err != nil {
				log.Errorf("Error in ReleaseTestWithSignal for '%s' function: %v", stage.FuncName, err)
				return
			}

			// Summarize metrics
			fmt.Printf(agg.SummarizeString())
			summary := agg.SummarizeResult()

			// Process the results of the release test, and set the summary.Status
			success, rollbackRequired := Tests.ProcessStageResult(stage, summary)

			log.Infof("Running after test instructions. Checking if rollback is required...")
			nextStage, err = m.handleAfterTestInstructions(stage, testMeta, fMeta, strategy, agg, rollbackRequired, success, rollbackFuncVer)
			if err != nil {
				log.Errorf("Failed to handle after test instructions: %v", err)
				return
			}

			log.Infof("f1 response time: Min %vms, Max %vms", summary.F1TimesSummary.Minimum, summary.F1TimesSummary.Maximum)
			log.Infof("f2 response time: Min %vms, Max %vms", summary.F2TimesSummary.Minimum, summary.F2TimesSummary.Maximum)

			// Send result summary to parent
			if nextStage != nil {
				nextStageName = nextStage.Name
			}
			err = summary.SendResultSummary(strategy.ID, nextStageName, m.ID, m.ParentHost, m.ParentPort)
			if err != nil {
				log.Errorf("Failed to send result summary: %v", err)
			}

		default:
			log.Warnf("Unknown stage type: %s. Ignoring it", stage.Type)
		}
		if nextStage != nil {
			log.Warnf("nextStage should be: %v", nextStage) // TODO: support specifying a specific stage to jump to
		}
		log.Warn("running the next stage in the list if any (and not nextStage)")
	}
	log.Info("Release strategy completed")
}
