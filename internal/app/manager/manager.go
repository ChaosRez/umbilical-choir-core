package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/paulmach/orb"
	log "github.com/sirupsen/logrus"
	"net/http"
	"umbilical-choir-core/internal/app/config"
	FaaS "umbilical-choir-core/internal/app/faas"
	MetricAgg "umbilical-choir-core/internal/app/metric_aggregator"
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

func (m *Manager) RunReleaseStrategy(strategy *Strategy.ReleaseStrategy) {
	agentHost := m.Host
	for _, stage := range strategy.Stages {
		log.Infof("%s: starting a '%s' stage for '%s' function", stage.Name, stage.Type, stage.FuncName)
		var nextStage *Strategy.Stage = nil
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

			// Process the results of the release test
			success, rollbackRequired := m.processStageResult(stage, summary) // TODO if rollbackRequired, then break? what to report to parent?

			log.Infof("Running after test instructions. Checking if rollback is required...")
			nextStage, err = m.handleAfterTestInstructions(stage, testMeta, fMeta, strategy, agg, rollbackRequired, success, rollbackFuncVer, summary)
			if err != nil {
				log.Errorf("Failed to handle after test instructions: %v", err)
				return
			}

			log.Infof("f1 response time: Min %vms, Max %vms", summary.F1TimesSummary.Minimum, summary.F1TimesSummary.Maximum)
			log.Infof("f2 response time: Min %vms, Max %vms", summary.F2TimesSummary.Minimum, summary.F2TimesSummary.Maximum)

			// Send result summary to parent
			err = m.sendResultSummary(m.ID, strategy.ID, summary)
			if err != nil {
				log.Errorf("Failed to send result summary: %v", err)
			}
		case "WaitForSignal":
			// TODO: combine with normal releasetest. The only difference is the polling for signal + extera parameters needed
			testMeta, agg, err := Tests.ReleaseTestWithSignal(stage, fMeta, agentHost, m.FaaS, m.ParentHost, m.ParentPort, m.ID, strategy.ID)
			if err != nil {
				log.Errorf("Error in ReleaseTestWithSignal for '%s' function: %v", stage.FuncName, err)
				return
			}

			// Summarize metrics
			fmt.Printf(agg.SummarizeString())
			summary := agg.SummarizeResult()

			// Process the results of the release test
			success, rollbackRequired := m.processStageResult(stage, summary)

			log.Infof("Running after test instructions. Checking if rollback is required...")
			nextStage, err = m.handleAfterTestInstructions(stage, testMeta, fMeta, strategy, agg, rollbackRequired, success, rollbackFuncVer, summary)
			if err != nil {
				log.Errorf("Failed to handle after test instructions: %v", err)
				return
			}

			log.Infof("f1 response time: Min %vms, Max %vms", summary.F1TimesSummary.Minimum, summary.F1TimesSummary.Maximum)
			log.Infof("f2 response time: Min %vms, Max %vms", summary.F2TimesSummary.Minimum, summary.F2TimesSummary.Maximum)

			// Send result summary to parent
			err = m.sendResultSummary(m.ID, strategy.ID, summary)
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

// private
// handles rollback (if needed), success/failure, and determining the next stage
func (m *Manager) handleAfterTestInstructions(stage Strategy.Stage, testMeta *Tests.TestMeta, fMeta *Strategy.Function, strategy *Strategy.ReleaseStrategy, agg *MetricAgg.MetricAggregator, rollbackRequired bool, success bool, rollbackFuncVer *Strategy.Version, summary *MetricAgg.ResultSummary) (*Strategy.Stage, error) {
	if rollbackRequired {
		log.Warn("Rollback is required. Replacing the rollback func... dump:", rollbackFuncVer)
		testMeta.ReplaceChosenFunction(*rollbackFuncVer)
		summary.Status = "error"
		return nil, nil
	} else {
		if success {
			log.Infof("All '%s' requirements met. Proceeding with OnSuccess action", stage.Name)
			nextStage, err := handleEndAction(stage.EndAction.OnSuccess, testMeta, fMeta, strategy)
			summary.Status = "success"
			if err != nil {
				return nil, fmt.Errorf("Failed to handle end action: %v", err)
			}
			return nextStage, nil
		} else {
			log.Warnf("'%s' requirements Not met. Proceeding with OnFailure action", stage.Name)
			nextStage, err := handleEndAction(stage.EndAction.OnFailure, testMeta, fMeta, strategy)
			summary.Status = "failure"
			if err != nil {
				return nil, fmt.Errorf("Failed to handle end action: %v", err)
			}
			if (int(agg.F1ErrCounts)) != 0 {
				log.Warnf("however, f1 had errors during test: %v/%v.", agg.F1ErrCounts, agg.F1Counts)
			}
			return nextStage, nil
		}
	}
}

func (m *Manager) processStageResult(stage Strategy.Stage, summary *MetricAgg.ResultSummary) (bool, bool) {
	success := true
	rollbackRequired := false

	for _, metricCondition := range stage.MetricsConditions {
		switch metricCondition.Name {
		case "responseTime":
			switch metricCondition.CompareWith {
			case "Median":
				if metricCondition.IsThresholdMet(summary.F2TimesSummary.Median) {
					log.Infof("Median response time (%v) requirement for f2 met: %v", summary.F2TimesSummary.Median, metricCondition.Threshold)
				} else {
					log.Warnf("Median response time (%v) requirement for f2 Not met: %v", summary.F2TimesSummary.Median, metricCondition.Threshold)
					success = false
				}
			case "Minimum":
				if metricCondition.IsThresholdMet(summary.F2TimesSummary.Minimum) {
					log.Infof("Minimum response time (%v) requirement for f2 met: %v", summary.F2TimesSummary.Minimum, metricCondition.Threshold)
				} else {
					log.Warnf("Minimum response time (%v) requirement for f2 Not met: %v", summary.F2TimesSummary.Minimum, metricCondition.Threshold)
					success = false
				}
			case "Maximum":
				if metricCondition.IsThresholdMet(summary.F2TimesSummary.Maximum) {
					log.Infof("Maximum response time (%v) requirement for f2 met: %v", summary.F2TimesSummary.Maximum, metricCondition.Threshold)
				} else {
					log.Warnf("Maximum response time (%v) requirement for f2 Not met: %v", summary.F2TimesSummary.Maximum, metricCondition.Threshold)
					success = false
				}
			default:
				rollbackRequired = true
				log.Errorf("Unknown compareWith parameter: %s", metricCondition.CompareWith)
			}

		case "errorRate":
			if metricCondition.IsThresholdMet(summary.F2ErrRate) {
				log.Infof("Error rate (%v) requirement for f2 met: %v", summary.F2ErrRate, metricCondition.Threshold)
			} else {
				log.Warnf("Error rate (%v) requirement for f2 Not met: %v", summary.F2ErrRate, metricCondition.Threshold)
				success = false
			}
		default:
			rollbackRequired = true
			log.Warnf("Unknown metric condition: %s. Ignoring it", metricCondition.Name)
		}
	}

	return success, rollbackRequired
}

// handleEndAction either runs rollout/rollback or returns the next stage
func handleEndAction(endAction string, testMeta *Tests.TestMeta, fMeta *Strategy.Function, strategy *Strategy.ReleaseStrategy) (*Strategy.Stage, error) {
	log.Infof("Processing end action '%s'", endAction)
	switch endAction {
	case "rollout":
		log.Info("(rollout) Replacing new func version (f2)...")
		testMeta.ReplaceChosenFunction(fMeta.NewVersion)
	case "rollback":
		log.Info("(rollback) Replacing the base func version (f1)...")
		testMeta.ReplaceChosenFunction(fMeta.BaseVersion)

	default:
		nextStage, err := strategy.GetStageByName(endAction)
		if err != nil {
			return nil, fmt.Errorf("EndAction is not defined: %v", err)
		}
		return nextStage, nil
	}
	return nil, nil
}

func (m *Manager) sendResultSummary(id, releaseID string, summary *MetricAgg.ResultSummary) error {
	log.Infof("Sending the result summary to parent for release '%s'", releaseID)
	resultRequest := ResultRequest{
		ID:               id,
		ReleaseID:        releaseID,
		ReleaseSummaries: []MetricAgg.ResultSummary{*summary},
	}

	data, err := json.Marshal(resultRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal result request: %v", err)
	}

	url := fmt.Sprintf("http://%s:%s/result", m.ParentHost, m.ParentPort)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to send result request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response: %v", resp.Status)
	}

	return nil
}
