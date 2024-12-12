package tests

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	FaaS "umbilical-choir-core/internal/app/faas"
	MetricAggregator "umbilical-choir-core/internal/app/metric_aggregator"
)

func (t *TestMeta) releaseTestSetup() (*MetricAggregator.MetricAggregator, chan struct{}, error) {
	log.Info("Starting metric aggregator")
	aggregator := &MetricAggregator.MetricAggregator{
		Program:   fmt.Sprintf("test-%s", t.FuncName),
		StageName: t.StageName,
	}
	shutdownChan := make(chan struct{})
	go MetricAggregator.StartMetricServer(aggregator, shutdownChan)

// if withoutDeployingFunctions is set to true, the function will not be deployed, and f1UriAdd and f2UriAdd must be provided
func (t *TestMeta) releaseTestSetup(withoutDeployingFunctions bool, f1UriAdd, f2UriAdd string) (*MetricAggregator.MetricAggregator, chan struct{}, string, string, error) {
	log.Info("Setting up release test and proxy functions")
	// duplicate the function with a new name
	log.Infof("duplicating the base function '%s' from '%s'", t.AVersionName, t.AVersionPath)
	f1Uri, err := t.FaaS.Update(t.AVersionName, t.AVersionPath, t.AVersionRuntime, "http", true, []string{})
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", t.FuncName, t.AVersionName, err)
		return nil, nil, err
	}
	var f1Uri, f2Uri string
	var err error

	if !withoutDeployingFunctions {
		// duplicate the function with a new name
		log.Infof("duplicating the base function '%s' from '%s'", t.AVersionName, t.AVersionPath)
		f1Uri, err = t.FaaS.Update(t.AVersionName, t.AVersionPath, t.AVersionRuntime, "http", true, []string{})
		if err != nil {
			log.Errorf("error when duplicating the '%s' funcion as '%s': %v", t.FuncName, t.AVersionName, err)
			return nil, nil, f1Uri, f2Uri, err
		}

	// deploy the new version
	log.Infof("now, deploying the new version as '%s' from '%s'", t.BVersionName, t.BVersionPath)
	f2Uri, err := t.FaaS.Update(t.BVersionName, t.BVersionPath, t.BVersionRuntime, "http", true, []string{})
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", t.FuncName, t.BVersionName, err)
		return nil, nil, err
	}
		// deploy the new version
		log.Infof("now, deploying the new version as '%s' from '%s'", t.BVersionName, t.BVersionPath)
		f2Uri, err = t.FaaS.Update(t.BVersionName, t.BVersionPath, t.BVersionRuntime, "http", true, []string{})
		if err != nil {
			log.Errorf("error when deploying the new '%s' funcion as '%s': %v", t.FuncName, t.BVersionName, err)
			return nil, nil, f1Uri, f2Uri, err
		}
	} else {
		if f1UriAdd == "" || f2UriAdd == "" { // guard clause
			log.Fatalf("Error: withoutDeployingFunctions is set to true, but f1UriAdd or f2UriAdd is empty")
		}
		f1Uri = f1UriAdd // re-register the previously deployed function
		f2Uri = f2UriAdd
		log.Infof("Skipped func deployment. Re-using the previously deployed functions: f1Uri: %s, f2Uri: %s", f1Uri, f2Uri)
	}

	log.Info("Starting metric aggregator")
	aggregator := &MetricAggregator.MetricAggregator{
		Program:   programName,
		StageName: t.StageName,
	}
	shutdownChan := make(chan struct{})
	go MetricAggregator.StartMetricServer(aggregator, shutdownChan)

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
		return nil, nil, f1Uri, f2Uri, fmt.Errorf("unknown FaaS type: %T", t.FaaS)
	}

	log.Infof("now, uploading proxy function as '%s' from '%s'", t.FuncName, proxyPath)
	_, err = t.FaaS.Update(t.FuncName, proxyPath, "python", "http", true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", t.FuncName, err)
		return nil, nil, f1Uri, f2Uri, err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", t.FuncName)

	log.Info("Successfully completed releaseTestSetup")
	return aggregator, shutdownChan, f1Uri, f2Uri, nil
}

// releaseTestCleanup clean up the program after the test
func (t *TestMeta) releaseTestCleanup(metricShutdownChan chan struct{}) {

	close(metricShutdownChan) // shutdown the metric aggregator

	//return nil
}
