package app

import (
	log "github.com/sirupsen/logrus"
	TinyFaaS "umbilical-choir-core/internal/pkg/tinyfaas"
)

func abTest(funcName string) error { // TODO fill the parameters
	// initialize tinyFaaS manager instance
	tf := TinyFaaS.New("localhost", "8080")

	// duplicate the function with a new name
	_, err := tf.UploadLocal(funcName+"-old", "test/fns/sieve-of-eratosthenes", "nodejs", 1)
	//_, err := tf.uploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")
	if err != nil {
		log.Errorf("error when duplicating the '%s' funcion: %v", funcName, err)
		return err
	}
	// deploy the new version
	_, err = tf.UploadLocal(funcName+"-new", "test/fns/sieve-of-eratosthenes-new", "nodejs", 1)
	if err != nil {
		log.Errorf("error when deploying the new '%s' funcion: %v", funcName, err)
		return err
	}

	// deploy the proxy/metric function with the func name
	_, err = tf.UploadLocal(funcName, "???", "nodejs", 1) // TODO add our function

	// TODO if failed setting up, clean the deployments

	return nil
}
