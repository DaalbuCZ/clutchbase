package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clutchbase/backend/internal/control"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	token := os.Getenv("CLUTCHBASE_ADMIN_TOKEN")
	if len(token) < 32 {
		return errors.New("CLUTCHBASE_ADMIN_TOKEN must contain at least 32 characters")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	api := &control.API{DB: db, AdminToken: token}
	if err := api.Migrate(startup); err != nil {
		return errors.New("database setup failed; check DATABASE_URL and PostgreSQL availability")
	}
	mux := appHandler(api.Handler(), os.Getenv("CLUTCHBASE_WEB_DIR"))
	addr := os.Getenv("CLUTCHBASE_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("Clutchbase API listening on %s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func appHandler(api http.Handler, dir string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	if dir != "" {
		files := http.FileServer(http.Dir(dir))
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			files.ServeHTTP(w, r)
		}))
	}
	return mux
}
