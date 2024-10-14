package tests

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
	FaaS "umbilical-choir-core/internal/app/faas"
	MetricAggregator "umbilical-choir-core/internal/app/metric_aggregator"
	Strategy "umbilical-choir-core/internal/app/strategy"
)

type TestMeta struct {
	FuncName           string
	AVersionName       string
	BVersionName       string
	AVersionPath       string
	BVersionPath       string
	AVersionRuntime    string
	BVersionRuntime    string
	ATrafficPercentage int
	BTrafficPercentage int
	Program            string
	StageName          string
	AgentHost          string
	FaaS               FaaS.FaaS
}

// TODO: replace hard-coded entrypoint from input strategy

// ReleaseTest
// the test runs at least for 'minDuration' seconds and at least 'minCalls' are made to the function + collect metrics
func ReleaseTest(stageData Strategy.Stage, funcMeta *Strategy.Function, agentHost string, faas FaaS.FaaS) (*TestMeta, *MetricAggregator.MetricAggregator, error) {
	funcName := stageData.FuncName
	a := funcMeta.BaseVersion
	b := funcMeta.NewVersion
	variants := stageData.Variants
	aTrafficPercentage := 100
	bTrafficPercentage := 0
	for _, variant := range variants {
		switch variant.Name {
		case "base_version":
			aTrafficPercentage = variant.TrafficPercentage
		case "new_version":
			bTrafficPercentage = variant.TrafficPercentage
		default:
			log.Warnf("Unknown variant: '%v'. more than two versions is not yet supported. Ignoring it", variant.Name)
		}
	}
	if aTrafficPercentage+bTrafficPercentage != 100 {
		log.Fatalf("Unexpected! Traffic percentage for A and B versions should sum up to 100. Got %v and %v", aTrafficPercentage, bTrafficPercentage)
	}
	testEndConditions := stageData.EndConditions
	minDurationStr := "0s"
	minCalls := 0
	for _, req := range testEndConditions {
		switch req.Name {
		case "minDuration":
			minDurationStr = req.Threshold
		case "minCalls":
			num, err := strconv.Atoi(req.Threshold)
			if err != nil {
				log.Fatal("Error converting string 'minCalls' to int in 'EndConditions':", err)
			}
			minCalls = num
		default:
			log.Warnf("Unknown requirement: %v. Ignoring it", req.Name)
		}
	}
	log.Infof("Running ReleaseTest for '%s' function. Minimum end conditions: %v calls and %v run time", funcName, minCalls, minDurationStr)

	// Create an instance of TestMeta
	testMeta := &TestMeta{
		FuncName:           funcName,
		AVersionName:       funcName + "01",
		BVersionName:       funcName + "02",
		AVersionPath:       a.Path,
		BVersionPath:       b.Path,
		AVersionRuntime:    a.Env,
		BVersionRuntime:    b.Env,
		ATrafficPercentage: aTrafficPercentage,
		BTrafficPercentage: bTrafficPercentage,
		Program:            fmt.Sprintf("test-%s", funcName),
		StageName:          stageData.Name,
		AgentHost:          agentHost,
		FaaS:               faas,
	}

	minDuration, err := time.ParseDuration(minDurationStr)
	if err != nil {
		return testMeta, nil, fmt.Errorf("error parsing duration '%v': %v", minDurationStr, err)
	}

	// set up functions, and run Metric Aggregator before starting the test
	agg, metricShutdownChan, err := testMeta.releaseTestSetup()
	if err != nil {
		log.Errorf("Error in releaseTestSetup for '%s' function: %v", funcName, err)
		return testMeta, agg, err
	}
	// Clean up the test after a clean finish or an error
	defer testMeta.releaseTestCleanup(metricShutdownChan)

	log.Info("now polling Metric Aggregator for test result")
	beginning := time.Now()
	for {
		elapse := time.Since(beginning)
		// Query the count of proxyTime call metric
		callCount := int(agg.CallCounts)

		// If no calls were made, log and wait
		if callCount == 0 {
			log.Debugf("no '%v()' calls after %v, waiting...", funcName, elapse)
		} else {
			responseTimes := agg.ProxyTimes
			var lastResponseTime float64

			if len(responseTimes) == 0 {
				lastResponseTime = -1 // no value
				log.Errorf("Unexpected! no response time found in Metric Aggregator while call_count exist! Continuing...")
			} else {
				lastResponseTime = responseTimes[len(responseTimes)-1]
			}

			// If the count is at least minCalls, and minDuration passed, return true
			if callCount >= minCalls {
				if elapse > minDuration {
					log.Infof("ReleaseTest successful. The minimum call count and duration satisfied. time: %v, calls: %v, last response time: %v",
						elapse, callCount, lastResponseTime)
					return testMeta, agg, nil
				} else {
					log.Infof("min call count is done(%v), but min duration not satisfied (%vs/%vs). last response time: %vms. Continuing to poll...", callCount, elapse, minDuration, lastResponseTime)
				}
			} else if elapse > minDuration {
				log.Infof("min duration is done, but min calls not satisfied (%v/%v). last response time: %vms. Continuing to poll after %v...",
					callCount, minCalls, lastResponseTime, elapse)
			} else {
				log.Infof("Release Test in progress... %v calls | last took %vms | %v elapsed", callCount, lastResponseTime, elapse)
			}
		}
		// Wait before polling again
		time.Sleep(1 * time.Second)
	}
}

// replaces the proxy function with the given (winner) function, and cleanups release test functions
func (t *TestMeta) ReplaceChosenFunction(fVersion Strategy.Version) {
	_, err := t.FaaS.Update(t.FuncName, fVersion.Path, fVersion.Env, "http", true, []string{})
	if err != nil {
		log.Errorf("error replacing proxy function with %s's selected version: %v", t.FuncName, err)
	}
	// Clean up the functions
	err = t.FaaS.Delete(t.AVersionName)
	if err != nil {
		log.Errorf("Error cleaning up function %v: %v", t.AVersionName, err)
	}
	err = t.FaaS.Delete(t.BVersionName)
	if err != nil {
		log.Errorf("Error cleaning up function %v: %v", t.BVersionName, err)
	}
}

func (t *TestMeta) releaseTestSetup() (*MetricAggregator.MetricAggregator, chan struct{}, error) {
	log.Info("Starting metric aggregator")
	aggregator := &MetricAggregator.MetricAggregator{
		Program:   fmt.Sprintf("test-%s", t.FuncName),
		StageName: t.StageName,
	}
	shutdownChan := make(chan struct{})
	go MetricAggregator.StartMetricServer(aggregator, shutdownChan)

	log.Info("Setting up release test and proxy functions")
	// duplicate the function with a new name
	log.Infof("duplicating the base function '%s' from '%s'", t.AVersionName, t.AVersionPath)
	f1Uri, err := t.FaaS.Update(t.AVersionName, t.AVersionPath, t.AVersionRuntime, "http", true, []string{})
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", t.FuncName, t.AVersionName, err)
		return nil, nil, err
	}

	// deploy the new version
	log.Infof("now, deploying the new version as '%s' from '%s'", t.BVersionName, t.BVersionPath)
	f2Uri, err := t.FaaS.Update(t.BVersionName, t.BVersionPath, t.BVersionRuntime, "http", true, []string{})
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", t.FuncName, t.BVersionName, err)
		return nil, nil, err
	}

	// deploy the proxy/metric function with the func name
	args := []string{
		fmt.Sprintf("F1ENDPOINT=%s", f1Uri),
		fmt.Sprintf("F2ENDPOINT=%s", f2Uri),
		fmt.Sprintf("AGENTHOST=%s", t.AgentHost),
		fmt.Sprintf("F1NAME=%s", t.AVersionName),
		fmt.Sprintf("F2NAME=%s", t.BVersionName),
		fmt.Sprintf("PROGRAM=test-%s", t.FuncName),
		fmt.Sprintf("BCHANCE=%v", t.BTrafficPercentage),
	}

	var proxyPath string
	switch t.FaaS.(type) {
	case *FaaS.TinyFaaSAdapter:
		proxyPath = "../umbilical-choir-proxy/binary/_tinyfaas-arm64"
	case *FaaS.GCPAdapter:
		proxyPath = "../umbilical-choir-proxy/binary/_gcp-amd64"
	default:
		return nil, nil, fmt.Errorf("unknown FaaS type: %T", t.FaaS)
	}

	log.Infof("now, uploading proxy function as '%s' from '%s'", t.FuncName, proxyPath)
	_, err = t.FaaS.Update(t.FuncName, proxyPath, "python", "http", true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", t.FuncName, err)
		return nil, nil, err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", t.FuncName)

	log.Info("Successfully completed releaseTestSetup")
	return aggregator, shutdownChan, nil
}

// releaseTestCleanup clean up the program after the test
func (t *TestMeta) releaseTestCleanup(metricShutdownChan chan struct{}) {

	close(metricShutdownChan) // shutdown the metric aggregator

	//return nil
}
