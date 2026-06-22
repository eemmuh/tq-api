package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/keepcode/api/internal/api"
	"github.com/keepcode/api/internal/job"
	"github.com/keepcode/api/internal/worker"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	workers := flag.Int("workers", 2, "number of background workers")
	dbPath := flag.String("db", "jobs.db", "SQLite database path")
	flag.Parse()

	store, err := job.OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	pool := worker.NewPool(store, *workers, *workers*8)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Run(ctx)

	pending, err := store.RestartPending(ctx)
	if err != nil {
		log.Fatalf("restart pending jobs: %v", err)
	}
	for _, id := range pending {
		pool.Enqueue(id)
	}
	if len(pending) > 0 {
		log.Printf("re-enqueued %d pending job(s) from database", len(pending))
	}

	h := api.NewHandler(store, pool)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("POST /jobs", h.CreateJob)
	mux.HandleFunc("GET /jobs", h.ListJobs)
	mux.HandleFunc("GET /jobs/{id}", h.GetJob)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("task queue API listening on %s (%d workers)", *addr, *workers)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}
