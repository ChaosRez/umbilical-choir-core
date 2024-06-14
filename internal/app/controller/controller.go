package controller

import (
	"fmt"
	TinyFaaS "github.com/ChaosRez/go-tinyfaas"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	TinyFaaS "umbilical-choir-core/internal/pkg/tinyfaas"
)

func ABTest(funcName string, tf *TinyFaaS.TinyFaaS) error { // TODO fill the parameters
	log.Infof("Started ABTest for '%s' function", funcName)

	// duplicate the function with a new name
	baseName := funcName + "01"
	oldPath := "test/fns/sieve-of-eratosthenes" // TODO: get old function path from input
	_, err := tf.UploadLocal(baseName, oldPath, "nodejs", 1, false, []string{})
	//_, err := tf.uploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")
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
		//"HOST=172.17.0.1",
		"HOST=host.docker.internal",
		fmt.Sprintf("F1NAME=%s", baseName),
		fmt.Sprintf("F2NAME=%s", newName),
		fmt.Sprintf("PROGRAM=ab-%s", funcName)}

	proxyPath := "../umbilical-choir-proxy/binary/bash-arm-linux"
	_, err = tf.UploadLocal(funcName, proxyPath, "binary", 1, true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", funcName, err)
		return err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", funcName)
	log.Debugf("from proxyPath: '%s'", proxyPath)

	// TODO if failed setting up, clean the deployments
	return nil
}
