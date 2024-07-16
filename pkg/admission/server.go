package admission

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/cnrancher/rancher-flat-network/pkg/admission/webhook"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
)

type Server struct {
	address  string
	port     int
	certFile string
	keyFile  string

	wctx *wrangler.Context
}

// NewAdmissionWebhookServer creates a server for admission FlatNetworkSubnets
func NewAdmissionWebhookServer(
	address string,
	port int,
	cert string,
	key string,
	wctx *wrangler.Context,
) *Server {
	return &Server{
		address:  address,
		port:     port,
		certFile: cert,
		keyFile:  key,
		wctx:     wctx,
	}
}

func (s *Server) Run(ctx context.Context) error {
	pair, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		return fmt.Errorf("failed to load key pair [%v] [%v]: %w",
			s.certFile, s.keyFile, err)
	}

	addr := fmt.Sprintf("%v:%v", s.address, s.port)
	handler := webhook.NewWebhookHandler(s.wctx)

	var httpServer *http.Server
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/hostname", hostnameHandler)
	http.HandleFunc("/validate", handler.ValidateHandler)
	httpServer = &http.Server{
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		Addr: addr,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{
				pair,
			},
			MinVersion: tls.VersionTLS12,
		},
		ReadHeaderTimeout: time.Second * 10,
	}
	logrus.Infof("start listen flat-network admission webhook server on %v", addr)
	if err = httpServer.ListenAndServeTLS("", ""); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("failed to start admission web server: %w", err)
	}
	return nil
}
