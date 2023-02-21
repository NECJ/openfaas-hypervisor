package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/invoke", invokeFunction)

	fmt.Printf("Server up!!\n")
	err := http.ListenAndServe(":8080", nil)

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

func invokeFunction(w http.ResponseWriter, r *http.Request) {
	// body, _ := ioutil.ReadAll(r.Body)
	// fmt.Printf("got / request\n")
	// fmt.Printf("%s\n", body)
	// io.WriteString(w, "This is my website!\n")

	// err := json.NewDecoder(r.Body).Decode(&p)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }

	// // Do something with the Person struct...
	// fmt.Fprintf(w, "Person: %+v", p)
	io.WriteString(w, "3.1415\n")
}
