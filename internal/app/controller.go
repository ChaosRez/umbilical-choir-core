package app

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	TinyFaaS "umbilical-choir-core/internal/pkg/tinyfaas"
)

func ABTest(funcName string, tf *TinyFaaS.TinyFaaS) error { // TODO fill the parameters
	log.Debugf("Started ABTest for '%s' function", funcName)

	// duplicate the function with a new name
	baseName := funcName + "01"
	_, err := tf.UploadLocal(baseName, "test/fns/sieve-of-eratosthenes", "nodejs", 1, false, []string{})
	//_, err := tf.uploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", funcName, baseName, err)
		return err
	}
	log.Info("base function duplicated: ", baseName)

	// deploy the new version
	newName := funcName + "02"
	_, err = tf.UploadLocal(newName, "test/fns/sieve-of-eratosthenes-new", "nodejs", 1, false, []string{})
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", funcName, newName, err)
		return err
	}
	log.Info("new version deployed: ", newName)

	// deploy the proxy/metric function with the func name
	args := []string{"PORT=8000",
		//"HOST=172.17.0.1",
		"HOST=host.docker.internal",
		fmt.Sprintf("F1NAME=%s", baseName),
		fmt.Sprintf("F2NAME=%s", newName),
		fmt.Sprintf("PROGRAM=ab-%s", funcName)}

	_, err = tf.UploadLocal(funcName, "../umbilical-choir-proxy", "binary", 1, true, args)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", funcName, err)
		return err
	}
	log.Infof("uploaded proxy function as '%s'. The traffic will now be managed by the proxy", funcName)

	// TODO if failed setting up, clean the deployments
	return nil
}
