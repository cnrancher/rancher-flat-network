package admission

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/cnrancher/flat-network-operator/pkg/admission/webhook"
	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
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

	logrus.Infof("starting flat-network admission webhook server")
	handler := webhook.NewWebhookHandler(s.wctx)

	var httpServer *http.Server
	http.HandleFunc("/validate", handler.ValidateHandler)

	httpServer = &http.Server{
		Addr:      fmt.Sprintf("%s:%d", s.address, s.port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	if err = httpServer.ListenAndServeTLS("", ""); err != nil {
		return fmt.Errorf("failed to start admission web server: %w", err)
	}
	return nil
}
