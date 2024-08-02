package tests

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
	MetricAggregator "umbilical-choir-core/internal/app/metric_aggregator"
	Strategy "umbilical-choir-core/internal/app/strategy"
)

type ABMeta struct {
	FuncName           string
	AVersionName       string
	BVersionName       string
	AVersionPath       string
	BVersionPath       string
	AVersionRuntime    string
	BVersionRuntime    string
	AVersionThreads    int
	BVersionThreads    int
	AVersionIsFullPath bool
	BVersionIsFullPath bool
	ATrafficPercentage int
	BTrafficPercentage int
	Program            string
	tf                 *TinyFaaS.TinyFaaS
}

// ABTest
// the test runs at least for 'minDuration' seconds and at least 'minCalls' are made to the function
func ABTest(stageData Strategy.Stage, funcMeta *Strategy.Function, tf *TinyFaaS.TinyFaaS) (*ABMeta, *MetricAggregator.MetricAggregator, error) {
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
	abEndConditions := stageData.EndConditions
	minDurationStr := "0s"
	minCalls := 0
	for _, req := range abEndConditions {
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
	log.Infof("Called ABTest for '%s' function. Minimum end conditions: %v calls and %v run time", funcName, minCalls, minDurationStr)

	// Create an instance of ABMeta
	abMeta := &ABMeta{
		FuncName:           funcName,
		AVersionName:       funcName + "01",
		BVersionName:       funcName + "02",
		AVersionPath:       a.Path,
		BVersionPath:       b.Path,
		AVersionRuntime:    a.Env,
		BVersionRuntime:    b.Env,
		AVersionThreads:    a.Threads,
		BVersionThreads:    b.Threads,
		ATrafficPercentage: aTrafficPercentage,
		BTrafficPercentage: bTrafficPercentage,
		Program:            fmt.Sprintf("ab-%s", funcName),
		tf:                 tf,
	}

	minDuration, err := time.ParseDuration(minDurationStr)
	if err != nil {
		return abMeta, nil, fmt.Errorf("error parsing duration '%v': %v", minDurationStr, err)
	}

	// set up functions, and run Metric Aggregator before starting the test
	agg, metricShutdownChan, err := abMeta.aBTestSetup()
	if err != nil {
		log.Errorf("Error in ABTestSetup for '%s' function: %v", funcName, err)
		return abMeta, agg, err
	}
	// Clean up the test after a clean finish or an error
	defer abMeta.aBTestCleanup(metricShutdownChan)

	log.Info("Starting to poll the call count from Metric Aggregator")
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
					log.Infof("ABTest successful. The minimum call count and duration satisfied. time: %v, calls: %v, last response time: %v",
						elapse, callCount, lastResponseTime)
					return abMeta, agg, nil
				} else {
					log.Infof("min call count is done(%v), but min duration not satisfied (%vs/%vs). last response time: %vms. Continuing to poll...", callCount, elapse, minDuration, lastResponseTime)
				}
			} else if elapse > minDuration {
				log.Infof("min duration is done, but min call count not satisfied (%v/%v). last response time: %vms. Continuing to poll after %v...",
					callCount, minCalls, lastResponseTime, elapse)
			} else {
				log.Infof("A/B test In progress... %v calls | last took %vms | %v elapsed", callCount, lastResponseTime, elapse)
			}
		}
		// Wait before polling again
		time.Sleep(1 * time.Second)
	}
}

// replaces the proxy function with the given (winner) function, and cleanups A/B functions
func (t *ABMeta) ABTestReplaceChosenFunction(fVersion Strategy.Version) {
	_, err := t.tf.UploadLocal(t.FuncName, fVersion.Path, fVersion.Env, fVersion.Threads, fVersion.IsFullPath, []string{})
	if err != nil {
		log.Errorf("error replacing proxy function with %s's selected version: %v", t.FuncName, err)
	}
	// Clean up the functions
	err = t.tf.Delete(t.AVersionName)
	if err != nil {
		log.Errorf("Error cleaning up function %v: %v", t.AVersionName, err)
	}
	err = t.tf.Delete(t.BVersionName)
	if err != nil {
		log.Errorf("Error cleaning up function %v: %v", t.BVersionName, err)
	}
}

func (t *ABMeta) aBTestSetup() (*MetricAggregator.MetricAggregator, chan struct{}, error) {
	log.Info("Starting metric aggregator")
	aggregator := &MetricAggregator.MetricAggregator{
		Program: "ab-sieve",
	}
	shutdownChan := make(chan struct{})
	go MetricAggregator.StartMetricServer(aggregator, shutdownChan)

	log.Info("Setting up A/B and proxy functions")
	// duplicate the function with a new name
	log.Info("duplicating the base function: ", t.AVersionName)
	log.Debugf("from oldPath: '%s'", t.AVersionPath)
	_, err := t.tf.UploadLocal(t.AVersionName, t.AVersionPath, t.AVersionRuntime, t.AVersionThreads, t.BVersionIsFullPath, []string{})
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", t.FuncName, t.AVersionName, err)
		return nil, nil, err
	}

	// deploy the new version
	log.Infof("deploying new version as %v", t.BVersionName)
	log.Debugf("from newPath: '%s'", t.BVersionPath)
	_, err = t.tf.UploadLocal(t.BVersionName, t.BVersionPath, t.BVersionRuntime, t.BVersionThreads, t.BVersionIsFullPath, []string{})
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", t.FuncName, t.BVersionName, err)
		return nil, nil, err
	}

	// deploy the proxy/metric function with the func name
	args := []string{"PORT=8000",
		//"HOST=172.17.0.1",
		"HOST=host.docker.internal", // docker desktop
		fmt.Sprintf("F1NAME=%s", t.AVersionName),
		fmt.Sprintf("F2NAME=%s", t.BVersionName),
		fmt.Sprintf("PROGRAM=ab-%s", t.FuncName),
		fmt.Sprintf("BCHANCE=%v", t.BTrafficPercentage),
	}

	proxyPath := "../umbilical-choir-proxy/binary/new-python-m2"
	//proxyPath := "../umbilical-choir-proxy/python3"
	//proxyPath := "../umbilical-choir-proxy/binary/python-arm-linux"
	log.Infof("uploading proxy function as '%s'", t.FuncName)
	log.Debugf("from proxyPath: '%s'", proxyPath)
	_, err = t.tf.UploadLocal(t.FuncName, proxyPath, "python3", 1, true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", t.FuncName, err)
		return nil, nil, err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", t.FuncName)

	log.Info("Successfully completed ABTestSetup")
	return aggregator, shutdownChan, nil
}

// aBTestCleanup clean up the program after the test
func (t *ABMeta) aBTestCleanup(metricShutdownChan chan struct{}) {

	close(metricShutdownChan) // shutdown the metric aggregator

	//return nil
}
