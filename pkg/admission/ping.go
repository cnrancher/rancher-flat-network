package admission

import (
	"fmt"
	"net/http"
	"os"
)

func pingHandler(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("pong\n"))
}

func hostnameHandler(w http.ResponseWriter, req *http.Request) {
	n, err := os.Hostname()
	if err != nil {
		err := fmt.Errorf("failed to get hostname: %w", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Write([]byte(n + "\n"))
}
