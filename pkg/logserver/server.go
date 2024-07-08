package logserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	defaultSocketLocation = "/tmp/log.sock"
)

// Server structure is used to the store backend information
type Server struct {
	SocketLocation string
	Debug          bool
}

// StartServerWithDefaults starts the server with default values
func StartServerWithDefaults(ctx context.Context) {
	s := Server{
		SocketLocation: defaultSocketLocation,
	}
	s.Start(ctx)
}

// Start the server
func (s *Server) Start(ctx context.Context) {
	os.Remove(s.SocketLocation)
	go s.ListenAndServe(ctx)
}

// ListenAndServe is used to setup handlers and
// start listening on the specified location
func (s *Server) ListenAndServe(ctx context.Context) error {
	logrus.Infof("Listening logserver on %s", s.SocketLocation)
	server := http.Server{
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		ReadHeaderTimeout: time.Second * 30,
	}
	http.HandleFunc("/v1/loglevel", s.loglevel)
	socketListener, err := net.Listen("unix", s.SocketLocation)
	if err != nil {
		return err
	}
	return server.Serve(socketListener)
}

func (s *Server) loglevel(rw http.ResponseWriter, req *http.Request) {
	// curl -X POST -d "level=debug" localhost:12345/v1/loglevel
	logrus.Debugf("Received loglevel request")
	if req.Method == http.MethodGet {
		level := logrus.GetLevel().String()
		rw.Write([]byte(fmt.Sprintf("%s\n", level)))
	}

	if req.Method == http.MethodPost {
		if err := req.ParseForm(); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(fmt.Sprintf("Failed to parse form: %v\n", err)))
		}
		level, err := logrus.ParseLevel(req.Form.Get("level"))
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(fmt.Sprintf("Failed to parse loglevel: %v\n", err)))
		} else {
			logrus.SetLevel(level)
			logrus.Infof("set loglevel to [%v]", level.String())
			rw.Write([]byte("OK\n"))
		}
	}
}
