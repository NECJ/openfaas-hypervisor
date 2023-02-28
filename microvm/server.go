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
			if res, err := http.Get("http://localhost/ready"); err == nil && res.StatusCode == 200 {
				argsWithProg := os.Args
				http.Get("http://" + argsWithProg[1] + "/ready")
				break
			}
		}
	}()

	// setup http server
	http.HandleFunc("/invoke", invoke)
	http.HandleFunc("/ready", ready)
	err := http.ListenAndServe("", nil)
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

func ready(w http.ResponseWriter, r *http.Request) {}

func invoke(w http.ResponseWriter, r *http.Request) {
	// compute pi
	// reps := 4000000000
	// result := 3.0
	// op := 1
	// for i := 2; i < 2*reps+1; i += 2 {
	// 	result += 4.0 / float64(i*(i+1)*(i+2)*op)
	// 	op *= -1
	// }
	// io.WriteString(w, fmt.Sprintf("%f", result))
	io.WriteString(w, "3.1415")
}
