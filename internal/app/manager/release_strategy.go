package manager

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	MetricAgg "umbilical-choir-core/internal/app/metric_aggregator"
	Strategy "umbilical-choir-core/internal/app/strategy"
	Tests "umbilical-choir-core/internal/app/tests"
)

// handleAfterTestInstructions determines the next stage, handles rollback (if needed), and rollout/rollback actions
func (m *Manager) handleAfterTestInstructions(stage Strategy.Stage, testMeta *Tests.TestMeta, fMeta *Strategy.Function, strategy *Strategy.ReleaseStrategy, agg *MetricAgg.MetricAggregator, rollbackRequired bool, success bool, rollbackFuncVer *Strategy.Version) (*Strategy.Stage, error) {
	if rollbackRequired {
		log.Warn("Rollback is required. Replacing the rollback func... dump:", rollbackFuncVer)
		testMeta.ReplaceChosenFunction(*rollbackFuncVer)
		return nil, nil
	} else {
		if success {
			log.Infof("All '%s' requirements met. Proceeding with OnSuccess action", stage.Name)
			nextStage, err := handleEndAction(stage.EndAction.OnSuccess, testMeta, fMeta, strategy)
			if err != nil {
				return nil, fmt.Errorf("failed to handle end action: %v", err)
			}
			return nextStage, nil
		} else {
			log.Warnf("'%s' requirements Not met. Proceeding with OnFailure action", stage.Name)
			nextStage, err := handleEndAction(stage.EndAction.OnFailure, testMeta, fMeta, strategy)
			if err != nil {
				return nil, fmt.Errorf("failed to handle end action: %v", err)
			}
			if (int(agg.F1ErrCounts)) != 0 {
				log.Warnf("however, f1 had errors during test: %v/%v.", agg.F1ErrCounts, agg.F1Counts)
			}
			return nextStage, nil
		}
	}
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
