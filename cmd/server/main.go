package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"wifi-radar/internal/api"
	"wifi-radar/internal/collector"
	"wifi-radar/internal/store"
)

type ifList []string

func (i *ifList) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *ifList) Set(value string) error {
	if value == "" {
		return fmt.Errorf("interface name cannot be empty")
	}
	*i = append(*i, value)
	return nil
}

func main() {
	var (
		ifs      ifList
		interval time.Duration
		listen   string
		public   bool
	)

	flag.Var(&ifs, "if", "interface name to monitor (repeatable)")
	flag.DurationVar(&interval, "interval", 500*time.Millisecond, "sampling interval")
	flag.StringVar(&listen, "listen", "127.0.0.1:8888", "HTTP bind address")
	flag.BoolVar(&public, "public", false, "bind 0.0.0.0 (overrides listen if set)")
	flag.Parse()

	if len(ifs) == 0 {
		log.Fatalf("no interfaces provided; use --if <ifname>")
	}

	if public {
		listen = "0.0.0.0:8888"
	}

	st := store.New(8)
	apiHandler := api.API{Store: st}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", apiHandler.Status)
	mux.HandleFunc("/api/best", apiHandler.Best)
	mux.HandleFunc("/api/stream", apiHandler.Stream)

	staticDir := filepath.Join(mustCwd(), "web", "static")
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	go collectLoop(st, ifs, interval)

	log.Printf("listening on http://%s", listen)
	if err := http.ListenAndServe(listen, mux); err != nil {
		log.Fatal(err)
	}
}

func collectLoop(st *store.Store, ifs []string, interval time.Duration) {
	collectors := make([]collector.Collector, 0, len(ifs))
	for _, ifname := range ifs {
		collectors = append(collectors, collector.Collector{IfName: ifname})
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		for _, c := range collectors {
			sample, err := c.Collect()
			if err != nil {
				if errors.Is(err, collector.ErrNotConnected) {
					continue
				}
				log.Printf("collect %s: %v", c.IfName, err)
				continue
			}
			st.Update(sample)
		}
		<-ticker.C
	}
}

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get cwd: %v", err)
	}
	return cwd
}
