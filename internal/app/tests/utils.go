package tests

import (
	log "github.com/sirupsen/logrus"
	"time"
	"umbilical-choir-core/internal/app/poller"
)

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
				log.Debugf("Polled for signal: %v", endTest)
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
