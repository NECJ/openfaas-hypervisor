package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	// register as ready with hypervisor
	go func() {
		for {
			if res, err := http.Get("http://localhost:8080/ready"); err == nil && res.StatusCode == 200 {
				argsWithProg := os.Args
				http.Get("http://" + argsWithProg[1] + ":8080/ready")
				break
			}
		}
	}()

	// setup http server
	http.HandleFunc("/invoke", invoke)
	http.HandleFunc("/ready", ready)
	err := http.ListenAndServe(":8080", nil)
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

func ready(w http.ResponseWriter, r *http.Request) {}

func invoke(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "3.1415!!!\n")
}
