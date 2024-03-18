package main

import (
	"fmt"
	"log"
)

func main() {
	// initialize tinyFaaS manager instance
	tf := NewTinyFaaS("localhost", "8080")

	/** upload a function **/
	//respU, err := tf.uploadLocal("sieve", "test/fns/sieve-of-eratosthenes", "nodejs", 1)
	respU, err := tf.uploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if err != nil {
		log.Fatalln("error when calling tinyFaaS: ", err)
	}
	// Print the response
	fmt.Printf("upload (%d): %s\n", respU.StatusCode(), respU) // 200 success, 400 bad request

	/** delete a function **/
	respD, err := tf.delete("sieve")

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if err != nil {
		log.Fatalln("error when calling tinyFaaS: ", err)
	}
	// Print the response
	fmt.Printf("delete (%d): %s\n", respD.StatusCode(), respD) // 200 success, 500 not exist

	/** get results log **/
	respL, err := tf.resultsLog()

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if err != nil {
		log.Fatalln("error when calling tinyFaaS: ", err)
	}
	// Print the response
	fmt.Printf("results log (%d): %s\n", respL.StatusCode(), respL) // 200 success

	/** wipe functions **/
	respW, err := tf.wipeFunctions()

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if err != nil {
		log.Fatalln("error when calling tinyFaaS: ", err)
	}
	// Print the response
	fmt.Printf("wipe functions (%d): %s\n", respW.StatusCode(), respW) // 200 success, 400 bad request

	/** list functions **/
	respF, err := tf.functions()

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if err != nil {
		log.Fatalln("error when calling tinyFaaS: ", err)
	}
	// Print the response
	fmt.Printf("functions (%d): %s\n", respF.StatusCode(), respF) // 200 success

}

//func init() {
//}
