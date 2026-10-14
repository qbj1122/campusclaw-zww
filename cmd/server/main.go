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

	"github.com/qbj1122/campusclaw-zww/internal/auth"
	"github.com/qbj1122/campusclaw-zww/internal/config"
	"github.com/qbj1122/campusclaw-zww/internal/db"
	"github.com/qbj1122/campusclaw-zww/internal/httpapi"
	"github.com/qbj1122/campusclaw-zww/internal/llm"
	"github.com/qbj1122/campusclaw-zww/internal/service"
	"github.com/qbj1122/campusclaw-zww/internal/store"
	"github.com/qbj1122/campusclaw-zww/internal/vector"
)

func main() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "error", err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DSN(), log)
	if err != nil {
		log.Error("database connection failed", "error", err.Error())
		os.Exit(1)
	}
	defer database.Close()
	if err := db.Migrate(ctx, database, log); err != nil {
		log.Error("migration failed", "error", err.Error())
		os.Exit(1)
	}
	if err := db.Seed(ctx, database, cfg.UploadDir, cfg.SeedPassword); err != nil {
		log.Error("seed failed", "error", err.Error())
		os.Exit(1)
	}

	st := store.New(database)
	llmClient := llm.New(cfg.ChatBaseURL, cfg.ChatAPIKey, cfg.ChatModel, cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDim)
	vecClient := vector.New(cfg.QdrantURL, cfg.QdrantCollection)
	retrieval := &service.Retrieval{Store: st, LLM: llmClient, Vector: vecClient, Cfg: cfg, Log: log}
	mcpClient := service.NewMCPClient()
	assistant := &service.Assistant{Store: st, LLM: llmClient, Retrieval: retrieval, MCP: mcpClient, Cfg: cfg, Log: log}

	srv := &httpapi.Server{
		Cfg: cfg, Store: st, Auth: &auth.Manager{Secret: []byte(cfg.JWTSecret), TTL: cfg.JWTTTL, Store: st},
		Retrieval: retrieval, Assistant: assistant, MCP: mcpClient, Log: log,
		Limiter: httpapi.NewLoginLimiter(5, 5*time.Minute),
	}

	// Index seed data before accepting requests: re-indexing deletes and
	// re-writes vectors, and a concurrent search must not observe that gap.
	retrieval.EnsureAllIndexed(ctx)
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = st.CleanupRevocations(ctx)
			}
		}
	}()

	httpServer := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      0, // SSE streams stay open
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Info("campusclaw api listening", "addr", httpServer.Addr, "offline_ai", llmClient.Offline())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Info("shutdown complete")
}
