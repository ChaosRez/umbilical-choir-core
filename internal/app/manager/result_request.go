package manager

import MetricAgg "umbilical-choir-core/internal/app/metric_aggregator"

type ResultRequest struct {
	ID               string                    `json:"id"`
	ReleaseID        string                    `json:"release_id"`
	ReleaseSummaries []MetricAgg.ResultSummary `json:"release_summaries"`
}
