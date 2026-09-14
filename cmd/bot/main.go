// Command bot is the gitlab-tg-notify entry point: it loads config, opens
// the SQLite store, creates the Telegram client, and starts the webhook
// HTTP server (PRD §7).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/febryandana/gitlab-tg-notify/internal/config"
	"github.com/febryandana/gitlab-tg-notify/internal/logging"
	"github.com/febryandana/gitlab-tg-notify/internal/store"
	"github.com/febryandana/gitlab-tg-notify/internal/telegram"
	"github.com/febryandana/gitlab-tg-notify/internal/webhook"
)

const defaultConfigPath = "/app/config.yaml"

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = defaultConfigPath
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := logging.New(cfg.Logging.Level, cfg.Logging.Format)

	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer db.Close()

	issueStore := store.NewIssueThreadStore(db)
	timeTrackingStore := store.NewTimeTrackingStore(db)

	tgClient, err := telegram.NewClient(cfg.Telegram.BotToken, cfg.Telegram.ChatID, cfg.Telegram.UseTopics, log)
	if err != nil {
		return err
	}

	handler := webhook.NewHandler(cfg, tgClient, issueStore, timeTrackingStore, log)
	srv := webhook.NewServer(cfg, handler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Server.ListenAddr, "path", cfg.Server.WebhookPath)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
