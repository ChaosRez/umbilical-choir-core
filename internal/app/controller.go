package app

import (
	log "github.com/sirupsen/logrus"
	TinyFaaS "umbilical-choir-core/internal/pkg/tinyfaas"
)

func ABTest(funcName string, tf *TinyFaaS.TinyFaaS) error { // TODO fill the parameters

	// duplicate the function with a new name
	baseName := funcName + "01"
	_, err := tf.UploadLocal(baseName, "test/fns/sieve-of-eratosthenes", "nodejs", 1)
	//_, err := tf.uploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion as '%s': %v", funcName, baseName, err)
		return err
	}
	log.Infof("uploaded '%s' function", baseName)

	// deploy the new version
	newName := funcName + "02"
	_, err = tf.UploadLocal(newName, "test/fns/sieve-of-eratosthenes-new", "nodejs", 1)
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion as '%s': %v", funcName, newName, err)
		return err
	}
	log.Infof("uploaded '%s' function", newName)

	// deploy the proxy/metric function with the func name
	_, err = tf.UploadLocal(funcName, "test/fns/proxy", "binary", 1)
	if err != nil {
		log.Errorf("error when deploying the proxy function as '%s': %v", funcName, err)
		return err
	}
	log.Infof("uploaded proxy function as '%s'", funcName)

	// TODO if failed setting up, clean the deployments
	log.Info("The traffic will now be managed by the proxy")

	return nil
}
