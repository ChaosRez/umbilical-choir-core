package metric_aggregator

import (
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
	Median  float64
	Minimum float64
	Maximum float64
}
type ResultSummary struct {
	ProxyTimes     TimeSummary
	F1TimesSummary TimeSummary
	F2TimesSummary TimeSummary
	F1ErrRate      float64
	F2ErrRate      float64
}

var registerOnce sync.Once

// StartMetricServer starts the metric server and listens for shutdown signals
func StartMetricServer(aggregator *MetricAggregator, shutdownChan <-chan struct{}) {
	server := &http.Server{Addr: ":9999"}

	registerOnce.Do(func() {
		http.HandleFunc("/push", aggregator.HandleIncomingMetrics)
	})

	go func() {
		log.Info("Starting metric server on port 9999")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on :9999: %v\n", err)
		}
	}()

	<-shutdownChan // Wait for shutdown signal

	log.Info("Shutting down the Metric server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Metric server forced to shutdown: %v", err)
	}

	log.Info("Metric server exiting")
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

	return &ResultSummary{
		ProxyTimes:     summarizeTimes(ma.ProxyTimes),
		F1TimesSummary: summarizeTimes(ma.F1Times),
		F2TimesSummary: summarizeTimes(ma.F2Times),
		F1ErrRate:      f1ErrorRate,
		F2ErrRate:      f2ErrorRate,
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
		msg += fmt.Sprintf("ProxyTimes - Med: %v, Min: %v, Max: %v", median, minP, maxP)
	} else {
		msg += "\nProxyTimes - No data available"
	}
	return msg
}
