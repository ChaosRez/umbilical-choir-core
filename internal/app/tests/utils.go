package tests

import (
	log "github.com/sirupsen/logrus"
	"time"
	MetricAgg "umbilical-choir-core/internal/app/metric_aggregator"
	"umbilical-choir-core/internal/app/poller"
	Strategy "umbilical-choir-core/internal/app/strategy"
)

// used for WaitForSignal stage types. if it gets a "shouldEnd" signal, it will return
func startPollingForSignal(host, port, id, strategyID, stageName string) chan struct{} {
	doneChan := make(chan struct{})
	waitTime := 5 * time.Second
	log.Infof("Polling for signal to end the test for stage '%s' after %v", stageName, waitTime)
	go func() {
		time.Sleep(waitTime)
		for {
			select {
			case <-doneChan:
				return
			default:
				endTest, err := poller.PollForSignal(host, port, id, strategyID, stageName)
				//log.Debugf("Polled for signal: %v", endTest)
				if err != nil {
					log.Errorf("Polling error: %v. 1 sec backoff", err)
					time.Sleep(1 * time.Second)
					continue
				}
				if endTest {
					close(doneChan)
					return
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()
	return doneChan
}

// ProcessStageResult processes the result of a stage, set the summary.Status, and returns if the stage was successful and if a rollback is required
func ProcessStageResult(stage Strategy.Stage, summary *MetricAgg.ResultSummary) (bool, bool) {
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
					log.Warnf("Median response time (%v) requirement for f2 NOT met: %v", summary.F2TimesSummary.Median, metricCondition.Threshold)
					success = false
				}
			case "Minimum":
				if metricCondition.IsThresholdMet(summary.F2TimesSummary.Minimum) {
					log.Infof("Minimum response time (%v) requirement for f2 met: %v", summary.F2TimesSummary.Minimum, metricCondition.Threshold)
				} else {
					log.Warnf("Minimum response time (%v) requirement for f2 NOT met: %v", summary.F2TimesSummary.Minimum, metricCondition.Threshold)
					success = false
				}
			case "Maximum":
				if metricCondition.IsThresholdMet(summary.F2TimesSummary.Maximum) {
					log.Infof("Maximum response time (%v) requirement for f2 met: %v", summary.F2TimesSummary.Maximum, metricCondition.Threshold)
				} else {
					log.Warnf("Maximum response time (%v) requirement for f2 NOT met: %v", summary.F2TimesSummary.Maximum, metricCondition.Threshold)
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
				log.Warnf("Error rate (%v) requirement for f2 NOT met: %v", summary.F2ErrRate, metricCondition.Threshold)
				success = false
			}
		default:
			rollbackRequired = true
			log.Warnf("Unknown metric condition: %s. Ignoring it", metricCondition.Name)
		}
	}

	if success {
		summary.Status = MetricAgg.Completed
	} else {
		summary.Status = MetricAgg.Failure
	}
	if rollbackRequired {
		summary.Status = MetricAgg.Error
	}

	return success, rollbackRequired
}
