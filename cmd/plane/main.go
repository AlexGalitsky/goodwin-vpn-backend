package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"website.goodwin.vpn/plane/internal/api"
	"website.goodwin.vpn/plane/internal/store"
)

func main() {
	ctx := context.Background()
	dbURL := getenv("DATABASE_URL", "postgres://plane:plane@127.0.0.1:5432/plane?sslmode=disable")
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer st.Close()
	if getenv("SEED_DEV", "0") != "0" {
		if err := st.SeedDev(ctx); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}
	srv := api.New(st, api.Config{
		AdminPassword: getenv("ADMIN_PASSWORD", "change-me"),
		SessionSecret: getenv("SESSION_SECRET", "dev-session-secret-change-me"),
		PublicSubBase: getenv("PUBLIC_SUB_BASE", "http://127.0.0.1:8080"),
		AdminDir:      getenv("ADMIN_DIR", ""),
	})
	addr := api.ParseListen(getenv("LISTEN", ":8080"))
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	pollEvery := time.Minute
	if n, err := strconv.Atoi(getenv("TRAFFIC_POLL_SEC", "60")); err == nil && n > 0 {
		pollEvery = time.Duration(n) * time.Second
	}
	pollCtx, pollCancel := context.WithCancel(context.Background())
	defer pollCancel()
	go srv.CollectTrafficLoop(pollCtx, pollEvery)
	go srv.SweepEntitlementLoop(pollCtx, pollEvery)
	go func() {
		log.Printf("plane listen %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(c)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
