package controller

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	"time"
)

type ABMeta struct {
	FuncName        string
	AVersionName    string
	BVersionName    string
	AVersionPath    string
	BVersionPath    string
	AVersionRuntime string
	BVersionRuntime string
	AVersionThreads int
	BVersionThreads int
	promJob         string
	promProgram     string
	tf              *TinyFaaS.TinyFaaS
}

// ABTest returns true if the AB test was successful, false otherwise
// the test runs at least for 'minDuration' seconds and at least 'minCalls' are made to the function
func ABTest(funcName string, minDurationSec int, minCalls int, promHost string, tf *TinyFaaS.TinyFaaS) (*ABMeta, bool, error) {
	log.Infof("Called ABTest for '%s' function. Minimum Requirements: %v calls and %v seconds", funcName, minCalls, minDurationSec)
	minDuration := time.Duration(minDurationSec) * time.Second

	// Create a new Prometheus API client
	promClient, pErr := NewPrometheusClient(promHost)
	if pErr != nil {
		return nil, false, pErr
	}

	// Create an instance of ABMeta
	abMeta := &ABMeta{
		FuncName:        funcName,
		AVersionName:    funcName + "01",
		BVersionName:    funcName + "02",
		AVersionPath:    "test/fns/sieve-of-eratosthenes",     // TODO: get old function path from input
		BVersionPath:    "test/fns/sieve-of-eratosthenes-new", // TODO: get new function path from input
		AVersionRuntime: "nodejs",
		BVersionRuntime: "nodejs",
		AVersionThreads: 1,
		BVersionThreads: 1,
		promJob:         "umbilical-choir",
		promProgram:     fmt.Sprintf("ab-%s", funcName),
		tf:              tf,
	}

	// Clean up the test after a clean finish or an error
	defer abMeta.aBTestCleanup(promClient)

	// set up functions, and cleanup pushgateway before starting the test
	err := abMeta.aBTestSetup(promClient)
	if err != nil {
		log.Errorf("Error in ABTestSetup for '%s' function: %v", funcName, err)
		//// cleanup in case the setup failed
		//aBTestCleanupPushGateway(promClient, "umbilical-choir", fmt.Sprintf("ab-%s", funcName))
		return abMeta, false, err
	}

	log.Info("Starting to poll the count of proxyTime from Prometheus")
	beginning := time.Now()
	for {
		elapse := time.Since(beginning)
		// Query the count of proxyTime call metric
		resVec, errq := promClient.Query(fmt.Sprintf(`call_count{job="umbilical-choir", program="ab-%s"}`, funcName))
		if errq != nil {
			log.Errorf("Error querying proxyTime count: %v", errq)
			return abMeta, false, errq
		}
		// Convert the resVec, as a vector is expected to be returned
		countVec, ok := resVec.(model.Vector)
		if !ok {
			return abMeta, false, fmt.Errorf("resVec is not a model.Vector. dump: %countVec", resVec)
		}

		// If no calls were made, log and wait
		if len(countVec) == 0 {
			log.Debugf("no '%v()' calls after %v, waiting...", funcName, elapse)
		} else {
			callCount := countVec[0].Value // take the count value
			resVec2, errq2 := promClient.Query(fmt.Sprintf(`proxy_time{job="umbilical-choir", program="ab-%s"}`, funcName))
			if errq2 != nil {
				log.Errorf("Error querying proxyTime: %v", errq2)
				return abMeta, false, errq
			}
			//log.Debugf(resVec2.(model.Vector).String())
			responseVec, ok2 := resVec2.(model.Vector)
			if !ok2 {
				return abMeta, false, fmt.Errorf("responseVec is not a model.Vector. dump: %v", responseVec)
			}
			var lastResponseTime model.SampleValue
			if len(responseVec) == 0 {
				lastResponseTime = -1 // no value
				log.Errorf("Unexpected! no response time found in Prometheus while call_count exist! Continuing...")
			} else {
				lastResponseTime = responseVec[0].Value
			}

			// If the count is at least minCalls, and minDuration passed, return true
			if int(callCount) >= minCalls {
				if elapse > minDuration {
					log.Infof("ABTest successful. The minimum call count and duration satisfied. time: %v, calls: %v, last response time: %v",
						elapse, callCount, lastResponseTime)
					return abMeta, true, nil
				} else {
					log.Infof("min call count is done(%v), but min duration not satisfied (%vs/%vs). last response time: %vms. Continuing to poll...", callCount, elapse, minDuration, lastResponseTime)
				}
			} else if elapse > minDuration {
				log.Infof("min duration is done, but min call count not satisfied (%v/%v). last response time: %vms. Continuing to poll after %v...",
					callCount, minCalls, elapse, lastResponseTime)
			} else {
				log.Infof("A/B test In progress... %v calls | last took %vms | %v elapsed", callCount, lastResponseTime, elapse)
			}
		}
		// Wait before polling again
		time.Sleep(1 * time.Second)
	}
}

// replaces the proxy function with the given (winner) function, and cleanups A/B functions
func (t *ABMeta) ABTestReplaceChosenFunction(path string, env string, threads int) {
	//TODO replace the winner with the proxy function
	_, err := t.tf.UploadLocal(t.FuncName, path, env, threads, false, []string{})
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

func (t *ABMeta) aBTestSetup(prometheus *PrometheusClient) error {
	log.Info("Cleaning push gateway before starting")
	err := aBTestCleanupPushGateway(prometheus, t.promJob, t.promProgram)
	if err != nil {
		return err
	}
	log.Info("Setting up A/B and proxy functions")
	// duplicate the function with a new name
	log.Info("duplicating the base function: ", t.AVersionName)
	log.Debugf("from oldPath: '%s'", t.AVersionPath)
	_, err = t.tf.UploadLocal(t.AVersionName, t.AVersionPath, t.AVersionRuntime, t.AVersionThreads, false, []string{})
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", t.FuncName, t.AVersionName, err)
		return err
	}

	// deploy the new version
	log.Infof("deploying new version as %v", t.BVersionName)
	log.Debugf("from newPath: '%s'", t.BVersionPath)
	_, err = t.tf.UploadLocal(t.BVersionName, t.BVersionPath, t.BVersionRuntime, t.BVersionThreads, false, []string{})
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", t.FuncName, t.BVersionName, err)
		return err
	}

	// deploy the proxy/metric function with the func name
	args := []string{"PORT=8000",
		"HOST=172.17.0.1",
		//"HOST=host.docker.internal", // docker desktop
		fmt.Sprintf("F1NAME=%s", t.AVersionName),
		fmt.Sprintf("F2NAME=%s", t.BVersionName),
		fmt.Sprintf("PROGRAM=ab-%s", t.FuncName),
	}

	proxyPath := "../umbilical-choir-proxy/binary/python-arm-linux"
	log.Infof("uploading proxy function as '%s'", t.FuncName)
	log.Debugf("from proxyPath: '%s'", proxyPath)
	_, err = t.tf.UploadLocal(t.FuncName, proxyPath, "python3", 1, true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", t.FuncName, err)
		return err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", t.FuncName)

	log.Info("Successfully completed ABTestSetup")
	return nil
}

// aBTestCleanup clean up the program after the test
func (t *ABMeta) aBTestCleanup(promClient *PrometheusClient) error {
	err := aBTestCleanupPushGateway(promClient, t.promJob, t.promProgram)
	if err != nil {
		return err
	}
	return nil
}

// aBTestCleanupPushGateway cleans up the metrics for the program from the PushGateway
func aBTestCleanupPushGateway(promClient *PrometheusClient, job string, program string) error {
	// Clean up the metrics. Prometheus keeps pulling from pushgateway
	err := promClient.PushGatewayDeleteMetricsForProgram(job, program)
	if err != nil {
		log.Errorf("Error cleaning up Pushgateway: %v", err)
		return err
	}
	return nil
}
