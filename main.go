package main

import (
	"fmt"
	"log"
)

func main() {
	// initialize tinyFaaS manager instance
	tf := NewTinyFaaS("localhost", "8080")

	/** upload a function **/
	respU, err := tf.uploadLocal("sieve", "test/fns/sieve-of-eratosthenes", "nodejs", 1)
	//respU, err := tf.uploadURL("sieve", "tinyFaaS-main/test/fns/sieve-of-eratosthenes", "nodejs", 1, "https://github.com/OpenFogStack/tinyFaas/archive/main.zip")

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if err != nil {
		log.Fatalln("error when calling tinyFaaS: ", err)
	}
	// Print the response
	fmt.Printf("upload success: %s\n", respU) // 200 success, 400 bad request

	/** delete a function **/
	errD := tf.delete("sieve")

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if errD != nil {
		log.Fatalln("error when calling tinyFaaS: ", errD)
	}
	// Print the response
	fmt.Println("delete Success") // 200 success, 500 not exist

	/** get results log **/
	respL, errL := tf.resultsLog()

	// Check tinyFaaS response. (inc. timeout, ok, other errors?)
	if errL != nil {
		log.Fatalln("error when calling tinyFaaS: ", errL)
	}
	// Print the response
	fmt.Printf("results log: %s\n", respL) // 200 success

	/** list functions **/
	respF := tf.functions()
	// Print the response
	fmt.Printf("functions: %s\n", respF)

	/** wipe functions **/
	tf.wipeFunctions()
	// Print the response
	fmt.Println("wiped functions successfully") // 200 success, 400 bad request

}

//func init() {
//}
