package manager

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	FaaS "umbilical-choir-core/internal/app/faas"
	Strategy "umbilical-choir-core/internal/app/strategy"
	Tests "umbilical-choir-core/internal/app/tests"
)

type Manager struct {
	FaaS FaaS.FaaS
	Host string
}

func New(faas FaaS.FaaS, agentHost string) *Manager {
	return &Manager{
		FaaS: faas,
		Host: agentHost,
	}
}

func (m *Manager) RunReleaseStrategy(strategy *Strategy.ReleaseStrategy) {
	agentHost := m.Host
	//functionsMeta := strategy.Functions
	stage1 := strategy.Stages[0]
	fMeta := strategy.GetFunctionByName(stage1.FuncName)
	rollbackFunc, err := fMeta.GetVersionByName(strategy.Rollback.Action.Function)
	if err != nil {
		log.Fatalf("Error getting rollback function: %v", err)
	}
	log.Infof("starting a '%s' stage for '%s' function", stage1.Type, stage1.FuncName)
	// Run the stage
	switch stage1.Type {
	case "A/B":
		testMeta, agg, err := Tests.ABTest(stage1, fMeta, agentHost, m.FaaS)
		if err != nil {
			log.Errorf("Error in ABTest for '%s' function: %v", stage1.FuncName, err)
			return
		}

		// Summarize metrics
		fmt.Printf(agg.SummarizeString())
		summary := agg.SummarizeResult()
		// Decide which function to deploy
		success := true
		rollbackRequired := false
		metricsConditions := stage1.MetricsConditions
		for _, metricCondition := range metricsConditions { //TODO: parse thresholds and compare them and use a Success flag to decide
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
					log.Fatalf("Unknown compareWith value: %s", metricCondition.CompareWith)
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
		if !rollbackRequired {
			if success {
				log.Info("Success! Replacing new func version (f2)...")
				testMeta.ABTestReplaceChosenFunction(fMeta.NewVersion)
			} else {
				log.Warnf("f2 did not meet the requirements. Replacing the base func version (f1)...")
				if (int(agg.F1ErrCounts)) != 0 {
					log.Warnf("though f1 had errors during test: %v/%v.", agg.F1ErrCounts, agg.F1Counts)
				}
				testMeta.ABTestReplaceChosenFunction(fMeta.BaseVersion)
			}
		} else {
			log.Warn("Rollback is required. Replacing the rollback func... dump:", rollbackFunc)
			testMeta.ABTestReplaceChosenFunction(*rollbackFunc)
		}
		log.Infof("f1 response time: Min %v, Max %v", summary.F1TimesSummary.Minimum, summary.F1TimesSummary.Maximum)
		log.Infof("f2 response time: Min %v, Max %v", summary.F2TimesSummary.Minimum, summary.F2TimesSummary.Maximum)

	default:
		log.Warnf("Unknown stage type: %s. Ignoring it", stage1.Type)
	}
}

func loadStrategy(filePath string) *Strategy.ReleaseStrategy {

	releaseStrategy, err := Strategy.NewStrategy(filePath)
	if err != nil {
		log.Fatalf("Failed to parse StrategyYAML: %v", err)
	}

	log.Infof("using release strategy '%v' (%v) from '%s'. It has following stages: %v", releaseStrategy.Name, releaseStrategy.Type, filePath, mapStageNames(releaseStrategy.Stages))
	log.Debugf("dump: %v", releaseStrategy)

	return releaseStrategy
}

// Helper map function
func mapStageNames(stages []Strategy.Stage) []string {
	names := make([]string, len(stages))
	for i, stage := range stages {
		names[i] = stage.Name
	}
	return names
}
