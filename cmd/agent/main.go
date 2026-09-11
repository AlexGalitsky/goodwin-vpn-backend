package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/agentd"
)

type fileConfig struct {
	Listen    string `json:"listen"`
	Token     string `json:"token"`
	AllowExec bool   `json:"allow_exec"`
}

func main() {
	cfgPath := flag.String("config", os.Getenv("AGENT_CONFIG"), "path to agent.json")
	flag.Parse()

	cfg := fileConfig{
		Listen:    getenv("AGENT_LISTEN", "0.0.0.0:19400"),
		Token:     os.Getenv("AGENT_TOKEN"),
		AllowExec: getenv("AGENT_ALLOW_EXEC", "1") != "0",
	}
	if *cfgPath != "" {
		raw, err := os.ReadFile(*cfgPath)
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			log.Fatalf("config json: %v", err)
		}
	}
	if cfg.Listen == "" {
		cfg.Listen = "0.0.0.0:19400"
	}
	if cfg.Token == "" {
		cfg.Token = mustToken()
		log.Printf("generated AGENT_TOKEN=%s (set AGENT_TOKEN or agent.json to persist)", cfg.Token)
	}
	srv := agentd.New(agentd.Config{
		Listen:    cfg.Listen,
		Token:     cfg.Token,
		AllowExec: cfg.AllowExec,
		Version:   "dev",
		Prefix:    prefixFrom(cfgPath),
	})
	log.Printf("agent listen %s allow_exec=%v", cfg.Listen, cfg.AllowExec)
	httpSrv := &http.Server{Addr: listenAddr(cfg.Listen), Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func prefixFrom(cfgPath *string) string {
	if cfgPath != nil && *cfgPath != "" {
		return filepath.Dir(*cfgPath)
	}
	return "/opt/goodwin-vpn-agent"
}

func listenAddr(v string) string {
	if strings.HasPrefix(v, ":") {
		return v
	}
	return v
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func mustToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}
