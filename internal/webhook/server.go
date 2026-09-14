package webhook

import (
	"net/http"

	"github.com/febryandana/gitlab-tg-notify/internal/config"
)

// NewServer builds the *http.Server that routes cfg.Server.WebhookPath to h,
// listening on cfg.Server.ListenAddr.
func NewServer(cfg *config.Config, h http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.Server.WebhookPath, h)

	return &http.Server{
		Addr:    cfg.Server.ListenAddr,
		Handler: mux,
	}
}
