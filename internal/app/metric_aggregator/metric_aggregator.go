package metric_aggregator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// the expected format of the incoming JSON payload
type Metric struct {
	MetricName string  `json:"metric_name"`
	Value      float64 `json:"value"`
}
type MetricUpdatePayload struct {
	Program string   `json:"program"`
	Metrics []Metric `json:"metrics"`
}

type MetricAggregator struct {
	Program      string // program name
	StageName    string // stage name
	Mutex        sync.Mutex
	CallCounts   float64   // "Total number of calls"
	F1Counts     float64   // "Total number of f1 function calls"
	F2Counts     float64   // "Total number of f2 function calls"
	F1ErrCounts  float64   // error count for f1 instead of f1_time
	F2ErrCounts  float64   //
	ProxyTimes   []float64 // "Total call (proxy) processing time"
	F1Times      []float64 // "Total processing time of f1 function"
	F2Times      []float64 // "Total processing time of f2 function"
	OtherMetrics map[string]float64
}

type TimeSummary struct {
	Median  float64 `json:"median"`
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
}
type StageStatus int
type ResultSummary struct { // TODO: add call counts. no calls can seen as a success + add runtime (of test)
	StageName      string      `json:"stage_name"`
	ProxyTimes     TimeSummary `json:"proxy_times"`
	F1TimesSummary TimeSummary `json:"f1_times_summary"`
	F2TimesSummary TimeSummary `json:"f2_times_summary"`
	F1ErrRate      float64     `json:"f1_err_rate"`
	F2ErrRate      float64     `json:"f2_err_rate"`
	Status         StageStatus `json:"status"` // success, failure, or error
}

const ( // NOTE, for any change, update RM source code and the readme (+ stageStatusLabels)
	Pending        StageStatus = iota // The first stage status will be initialized as InProgress and never will be Pending
	InProgress                        // the child is notified
	SuccessWaiting                    // only WaitForSignal stage type. Child received enough calls and was successful
	ShouldEnd                         // only WaitForSignal stage type. The child poll for it on /end_stage to finish a stage (set by the parent)
	Completed                         // received the stage result
	Failure                           // received the stage result as Failure
	Error                             // received the stage result as Error
)

var stageStatusLabels = []string{
	"Pending",
	"InProgress",
	"SuccessWaiting",
	"ShouldEnd",
	"Completed",
	"Failure",
	"Error",
}

// String returns the string representation of the StageStatus (you can print as %s)
func (s StageStatus) String() string {
	if s < 0 || int(s) >= len(stageStatusLabels) {
		return fmt.Sprintf("StageStatus(%d)", s)
	}
	return stageStatusLabels[s]
}

// StartMetricServer starts the metric server and listens for shutdown signals
func StartMetricServer(aggregator *MetricAggregator, shutdownChan <-chan struct{}) {
	mux := http.NewServeMux() // Create a new ServeMux for each call
	mux.HandleFunc("/push", aggregator.HandleIncomingMetrics)

	server := &http.Server{
		Addr:    ":9999",
		Handler: mux,
	}

	go func() {
		log.Info("Starting metric server on port 9999")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on :9999: %v\n", err)
		}
	}()

	go func() {
		<-shutdownChan // Wait for shutdown signal

		log.Info("Shutting down the Metric server...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("Metric server forced to shutdown: %v", err)
		}

		log.Info("Metric server exiting")
	}()
}

func (ma *MetricAggregator) HandleIncomingMetrics(w http.ResponseWriter, r *http.Request) {
	ma.Mutex.Lock()
	defer ma.Mutex.Unlock()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	// Parse the JSON payload
	var payload MetricUpdatePayload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		http.Error(w, "Error parsing JSON payload", http.StatusBadRequest)
		return
	}

	// Debug log to dump received metrics
	log.Debugf("New metric set - Program: %s, Metrics: %+v", payload.Program, payload.Metrics)

	// update metrics
	for _, metric := range payload.Metrics {
		// Update local metric maps instead of Prometheus metrics
		switch metric.MetricName {
		case "call_count":
			ma.CallCounts += metric.Value
		case "f1_count":
			ma.F1Counts += metric.Value
		case "f2_count":
			ma.F2Counts += metric.Value
		case "proxy_time":
			ma.ProxyTimes = append(ma.ProxyTimes, metric.Value)
		case "f1_time":
			ma.F1Times = append(ma.F1Times, metric.Value)
		case "f2_time":
			ma.F2Times = append(ma.F2Times, metric.Value)
		case "f1_error_count":
			ma.F1ErrCounts += metric.Value
			log.Error("Proxy reported Error calling f1")
		case "f2_error_count":
			ma.F2ErrCounts += metric.Value
			log.Error("Proxy reported Error calling f2")
		default:
			ma.OtherMetrics[metric.MetricName] = metric.Value
			log.Warnf("Unknown metric name: %s. added it to 'OtherMetrics'", metric.MetricName)
		}
	}
	fmt.Fprintf(w, "Metrics updated successfully")
}

func (ma *MetricAggregator) SummarizeResult() *ResultSummary {
	ma.Mutex.Lock()
	defer ma.Mutex.Unlock()

	// Helper function to summarize times
	summarizeTimes := func(times []float64) TimeSummary {
		if len(times) == 0 {
			return TimeSummary{Median: -1, Minimum: -1, Maximum: -1}
		}
		var minT, maxT float64
		minT = times[0]
		for _, t := range times {
			if t < minT {
				minT = t
			}
			if t > maxT {
				maxT = t
			}
		}
		sortedTimes := make([]float64, len(times))
		copy(sortedTimes, times)
		sort.Float64s(sortedTimes)
		var median float64
		n := len(sortedTimes)
		if n%2 == 0 {
			median = (sortedTimes[n/2-1] + sortedTimes[n/2]) / 2
		} else {
			median = sortedTimes[n/2]
		}
		return TimeSummary{Median: median, Minimum: minT, Maximum: maxT}
	}

	// Calculate error rates
	var f1ErrorRate, f2ErrorRate float64
	if ma.F1Counts > 0 {
		f1ErrorRate = ma.F1ErrCounts / ma.F1Counts
	}
	if ma.F2Counts > 0 {
		f2ErrorRate = ma.F2ErrCounts / ma.F2Counts
	}
	if (ma.F1Counts + ma.F2Counts) < 1 { // if there are no calls
		log.Warnf("No calls were made to f1 or f2, but we will continue to process the status regardless")
	}

	return &ResultSummary{
		StageName:      ma.StageName,
		ProxyTimes:     summarizeTimes(ma.ProxyTimes),
		F1TimesSummary: summarizeTimes(ma.F1Times),
		F2TimesSummary: summarizeTimes(ma.F2Times),
		F1ErrRate:      f1ErrorRate,
		F2ErrRate:      f2ErrorRate,
		// Status will be post-processed by the manager
	}
}

func (ma *MetricAggregator) SummarizeString() string { // TODO: add error rates
	ma.Mutex.Lock()
	defer ma.Mutex.Unlock()

	// Summarize metrics
	msg := fmt.Sprintf("f1 errors: %v/%v", ma.F1ErrCounts, ma.F1Counts)
	msg += fmt.Sprintf("\nf2 errors: %v/%v", ma.F2ErrCounts, ma.F2Counts)
	msg += fmt.Sprintf("\nTotal calls (f1:f2): %v (%v:%v)\n", ma.CallCounts, ma.F1Counts, ma.F2Counts)

	// Aggregate ProxyTimes
	if len(ma.ProxyTimes) > 0 {
		var minP, maxP float64
		minP = ma.ProxyTimes[0]
		for _, t := range ma.ProxyTimes {
			if t < minP {
				minP = t
			}
			if t > maxP {
				maxP = t
			}
		}
		// Sort ProxyTimes to find the median
		sortedTimes := make([]float64, len(ma.ProxyTimes))
		copy(sortedTimes, ma.ProxyTimes)
		sort.Float64s(sortedTimes)

		var median float64
		n := len(sortedTimes)
		if n%2 == 0 {
			median = (sortedTimes[n/2-1] + sortedTimes[n/2]) / 2
		} else {
			median = sortedTimes[n/2]
		}
		msg += fmt.Sprintf("ProxyTimes - Med: %v, Min: %v, Max: %v\n", median, minP, maxP)
	} else {
		msg += "\nProxyTimes - No data available\n"
	}
	return msg
}

func (summary *ResultSummary) SendResultSummary(releaseID, nextStage, agentID, parentHost, parentPort string) error {
	log.Infof("Sending '%s' result summary to parent for release '%s', status '%v(%d)'", summary.StageName, releaseID, summary.Status, summary.Status)
	resultRequest := ResultRequest{
		ID:             agentID,
		ReleaseID:      releaseID,
		StageSummaries: []ResultSummary{*summary},
		NextStage:      nextStage,
	}

	data, err := json.Marshal(resultRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal result request: %v", err)
	}

	url := fmt.Sprintf("http://%s:%s/result", parentHost, parentPort)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to send result request: %v (%s)", err, resp.Status)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response: %v", resp.Status)
	}

	return nil
}
