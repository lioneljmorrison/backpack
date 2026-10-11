// Command backpack runs the Backpack service and administers its tokens.
//
//	backpack serve
//	backpack token create --actor <name>
//	backpack token revoke --actor <name>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/health"
	"github.com/lioneljmorrison/backpack/internal/httpapi"
)

const usage = `usage:
  backpack serve
  backpack token create --actor <name>
  backpack token revoke --actor <name>
`

// workspacePrefix names each workspace's graph: ws:<slug>.
const workspacePrefix = "ws:"

type config struct {
	Addr         string
	FalkorURL    string
	TypesenseURL string
	SystemGraph  string
}

func loadConfig(getenv func(string) string) config {
	or := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	return config{
		Addr:         or("BACKPACK_ADDR", ":8780"),
		FalkorURL:    or("BACKPACK_FALKORDB_URL", "falkor://localhost:6379"),
		TypesenseURL: or("BACKPACK_TYPESENSE_URL", "http://localhost:8108"),
		SystemGraph:  or("BACKPACK_SYSTEM_GRAPH", "backpack"),
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errOut, usage)
		return 2
	}
	cfg := loadConfig(getenv)
	switch args[0] {
	case "serve":
		return report(errOut, serve(cfg))
	case "token":
		return token(cfg, args[1:], out, errOut)
	}
	fmt.Fprint(errOut, usage)
	return 2
}

func report(errOut io.Writer, err error) int {
	if err != nil {
		fmt.Fprintln(errOut, "backpack:", err)
		return 1
	}
	return 0
}

func token(cfg config, args []string, out, errOut io.Writer) int {
	if len(args) == 0 || (args[0] != "create" && args[0] != "revoke") {
		fmt.Fprint(errOut, usage)
		return 2
	}
	fs := flag.NewFlagSet("token "+args[0], flag.ContinueOnError)
	fs.SetOutput(errOut)
	actor := fs.String("actor", "", "the person or agent the token belongs to")
	if err := fs.Parse(args[1:]); err != nil || *actor == "" {
		fmt.Fprint(errOut, usage)
		return 2
	}
	store, closeDB, err := openStore(cfg)
	if err != nil {
		return report(errOut, err)
	}
	defer closeDB()

	if args[0] == "create" {
		tok, err := store.CreateToken(*actor)
		if err != nil {
			return report(errOut, err)
		}
		fmt.Fprintln(out, tok)
		fmt.Fprintf(errOut, "token for %s: shown once, store it now\n", *actor)
		return 0
	}
	n, err := store.Revoke(*actor)
	if err != nil {
		return report(errOut, err)
	}
	fmt.Fprintf(out, "revoked %d token(s) for %s\n", n, *actor)
	return 0
}

func openStore(cfg config) (*auth.Store, func(), error) {
	db, err := falkor.Connect(cfg.FalkorURL)
	if err != nil {
		return nil, nil, err
	}
	store, err := auth.NewStore(db.SelectGraph(cfg.SystemGraph))
	if err != nil {
		db.Conn.Close()
		return nil, nil, err
	}
	return store, func() { db.Conn.Close() }, nil
}

func serve(cfg config) error {
	db, err := falkor.Connect(cfg.FalkorURL)
	if err != nil {
		return err
	}
	defer db.Conn.Close()
	store, err := auth.NewStore(db.SelectGraph(cfg.SystemGraph))
	if err != nil {
		return err
	}
	b, err := board.New(db, cfg.SystemGraph, workspacePrefix)
	if err != nil {
		return err
	}
	checks := map[string]health.Check{
		"falkordb":  func(context.Context) error { return falkor.Ping(db) },
		"typesense": health.Typesense(cfg.TypesenseURL),
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(checks, store.ActorFor, b),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("backpack listening on %s", cfg.Addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
