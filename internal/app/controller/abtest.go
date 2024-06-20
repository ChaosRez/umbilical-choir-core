package controller

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	"time"
)

// ABTest returns true if the AB test was successful, false otherwise
// the test runs at least for 'minDuration' seconds and at least 'minCalls' are made to the function
func ABTest(funcName string, minDurationSec int, minCalls int, promHost string, tf *TinyFaaS.TinyFaaS) (bool, error) {
	log.Infof("Called ABTest for '%s' function", funcName)
	minDuration := time.Duration(minDurationSec) * time.Second

	// Create a new Prometheus API client
	promClient, err := NewPrometheusClient(promHost)
	if err != nil {
		return false, err
	}
	ABTestCleanup(promClient, "umbilical-choir", fmt.Sprintf("ab-%s", funcName)) // clean once before starting

	//// set up functions
	err = aBTestSetup(funcName, tf)
	if err != nil {
		log.Errorf("Error in ABTestSetup for '%s' function: %v", funcName, err)
		// cleanup in case the setup failed
		ABTestCleanup(promClient, "umbilical-choir", fmt.Sprintf("ab-%s", funcName))
		return false, err
	}

	log.Info("Starting to poll the count of proxyTime from Prometheus")
	beginning := time.Now()
	for {
		elapse := time.Since(beginning)
		// Query the count of proxyTime call metric
		resVec, errq := promClient.Query(fmt.Sprintf(`call_count{job="umbilical-choir", program="ab-%s"}`, funcName))
		if errq != nil {
			log.Errorf("Error querying proxyTime count: %v", errq)
			return false, errq
		}
		// Convert the resVec, as a vector is expected to be returned
		countVec, ok := resVec.(model.Vector)
		if !ok {
			return false, fmt.Errorf("resVec is not a model.Vector. dump: %countVec", resVec)
		}

		// If no calls were made, log and wait
		if len(countVec) == 0 {
			log.Debugf("no '%v()' calls after %v, waiting...", funcName, elapse)
		} else {
			callCount := countVec[0].Value // take the count value
			resVec2, errq2 := promClient.Query(fmt.Sprintf(`proxy_time{job="umbilical-choir", program="ab-%s"}`, funcName))
			if errq2 != nil {
				log.Errorf("Error querying proxyTime: %v", errq2)
				return false, errq
			}
			//log.Debugf(resVec2.(model.Vector).String())
			responseVec, ok2 := resVec2.(model.Vector)
			if !ok2 {
				return false, fmt.Errorf("responseVec is not a model.Vector. dump: %v", responseVec)
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
					// Clean up the metrics for the program after the test
					ABTestCleanup(promClient, "umbilical-choir", fmt.Sprintf("ab-%s", funcName))
					return true, nil
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

func aBTestSetup(funcName string, tf *TinyFaaS.TinyFaaS) error {
	log.Info("Setting up A/B and proxy functions")
	// duplicate the function with a new name
	baseName := funcName + "01"
	oldPath := "test/fns/sieve-of-eratosthenes" // TODO: get old function path from input
	_, err := tf.UploadLocal(baseName, oldPath, "nodejs", 1, false, []string{})
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", funcName, baseName, err)
		return err
	}
	log.Info("base function duplicated: ", baseName)
	log.Debugf("from oldPath: '%s'", oldPath)

	// deploy the new version
	newName := funcName + "02"
	newPath := "test/fns/sieve-of-eratosthenes-new" // TODO: get new function path from input
	_, err = tf.UploadLocal(newName, newPath, "nodejs", 1, false, []string{})
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", funcName, newName, err)
		return err
	}
	log.Info("new version deployed: ", newName)
	log.Debugf("from newPath: '%s'", newPath)

	// deploy the proxy/metric function with the func name
	args := []string{"PORT=8000",
		"HOST=172.17.0.1",
		//"HOST=host.docker.internal", // docker desktop
		fmt.Sprintf("F1NAME=%s", baseName),
		fmt.Sprintf("F2NAME=%s", newName),
		fmt.Sprintf("PROGRAM=ab-%s", funcName),
	}

	proxyPath := "../umbilical-choir-proxy/binary/python-arm-linux"
	_, err = tf.UploadLocal(funcName, proxyPath, "python3", 1, true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", funcName, err)
		return err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", funcName)
	log.Debugf("from proxyPath: '%s'", proxyPath)

	log.Info("Successfully completed ABTestSetup")
	return nil
}

// ABTestCleanup cleans up the metrics for the program after the test
func ABTestCleanup(promClient *PrometheusClient, job string, program string) {
	err := promClient.PushGatewayDeleteMetricsForProgram(job, program)
	if err != nil {
		log.Errorf("Error cleaning up Pushgateway: %v", err)
	}
}
