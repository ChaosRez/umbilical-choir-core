package manager

import MetricAgg "umbilical-choir-core/internal/app/metric_aggregator"

type ResultRequest struct {
	ID             string                    `json:"id"`
	ReleaseID      string                    `json:"release_id"`
	StageSummaries []MetricAgg.ResultSummary `json:"stage_summaries"`
	NextStage      string                    `json:"next_stage"` // NOTE: in case of a "rollback" or "rollout", nextStage will be nil
}
