package main

import "flag"

// config holds the runtime flags of the dairygate server.
type config struct {
	addr   string
	dbPath string
}

// parseConfig reads and validates command-line flags.
func parseConfig() *config {
	cfg := &config{}
	flag.StringVar(&cfg.addr, "addr", ":8080", "HTTP listen address")
	flag.StringVar(&cfg.dbPath, "db", "dairygate.db", "SQLite database path (use :memory: for in-memory)")
	flag.Parse()
	return cfg
}
