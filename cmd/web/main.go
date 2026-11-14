package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/zakirov-net/arduino-telemetry/internal/config"
	"github.com/zakirov-net/arduino-telemetry/internal/mqttclient"
	"github.com/zakirov-net/arduino-telemetry/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if _, err := os.Stat(".env"); err == nil {
		_ = godotenv.Load(".env")
	}

	cfg, err := config.LoadWeb()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	store, err := storage.OpenWithRetry(cfg.PostgresDSN, 30*time.Second)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer store.Close()

	mqttClient, err := mqttclient.NewClient(mqttclient.Options{
		Broker:   cfg.Broker,
		ClientID: cfg.ClientID,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		log.Fatalf("mqtt: %v", err)
	}
	defer mqttClient.Disconnect(250)

	mux := http.NewServeMux()
	h := &apiHandler{store: store, mqtt: mqttClient, cfg: cfg}
	h.register(mux)

	// Статика из embed, с fallback на index.html для SPA-роутов
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("embed fs: %v", err)
	}
	mux.Handle("GET /", spaHandler(http.FS(sub)))

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Printf("web listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = srv.Shutdown(shutdownCtx)
}
