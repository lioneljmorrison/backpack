// Command backpack runs the Backpack service and administers its tokens.
//
//	backpack serve
//	backpack token create --actor <name>
//	backpack token revoke --actor <name>
//	backpack reindex [--workspace <slug>]
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
	"github.com/lioneljmorrison/backpack/internal/search"
)

const usage = `usage:
  backpack serve
  backpack token create --actor <name>
  backpack token revoke --actor <name>
  backpack reindex [--workspace <slug>]
`

// rebuildTimeout bounds loading the search model and a full rebuild of the
// index. On a new Typesense volume, loading the model downloads it.
const rebuildTimeout = 2 * time.Minute

type config struct {
	Addr         string
	FalkorURL    string
	TypesenseURL string
	TypesenseKey string
	SystemGraph  string
	// WorkspacePrefix names each workspace's graph, ws:<slug>, and
	// CollectionPrefix its search collection, ws_<slug>. They are fixed;
	// only tests change them.
	WorkspacePrefix  string
	CollectionPrefix string
}

func loadConfig(getenv func(string) string) config {
	or := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	return config{
		Addr:             or("BACKPACK_ADDR", ":8780"),
		FalkorURL:        or("BACKPACK_FALKORDB_URL", "falkor://localhost:6379"),
		TypesenseURL:     or("BACKPACK_TYPESENSE_URL", "http://localhost:8108"),
		TypesenseKey:     getenv("BACKPACK_TYPESENSE_API_KEY"),
		SystemGraph:      or("BACKPACK_SYSTEM_GRAPH", "backpack"),
		WorkspacePrefix:  "ws:",
		CollectionPrefix: "ws_",
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
	return dispatch(loadConfig(getenv), args, out, errOut)
}

func dispatch(cfg config, args []string, out, errOut io.Writer) int {
	switch args[0] {
	case "serve":
		return report(errOut, serve(cfg))
	case "token":
		return token(cfg, args[1:], out, errOut)
	case "reindex":
		return reindex(cfg, args[1:], out, errOut)
	}
	fmt.Fprint(errOut, usage)
	return 2
}

// reindex rebuilds one workspace's search index, or every workspace's, from
// the board.
func reindex(cfg config, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("reindex", flag.ContinueOnError)
	fs.SetOutput(errOut)
	ws := fs.String("workspace", "", "only this workspace")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprint(errOut, usage)
		return 2
	}
	db, err := falkor.Connect(cfg.FalkorURL)
	if err != nil {
		return report(errOut, err)
	}
	defer db.Conn.Close()
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		return report(errOut, err)
	}
	x := search.New(cfg.TypesenseURL, cfg.TypesenseKey, cfg.CollectionPrefix, b)
	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()
	if *ws != "" {
		if err := x.Rebuild(ctx, *ws); err != nil {
			return report(errOut, err)
		}
		fmt.Fprintf(out, "reindexed %s\n", *ws)
		return 0
	}
	all, err := b.Workspaces()
	if err != nil {
		return report(errOut, err)
	}
	if err := x.RebuildAll(ctx); err != nil {
		return report(errOut, err)
	}
	fmt.Fprintf(out, "reindexed %d workspace(s)\n", len(all))
	return 0
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
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		return err
	}
	x := search.New(cfg.TypesenseURL, cfg.TypesenseKey, cfg.CollectionPrefix, b)
	b.OnChange(x.Report)
	rebuild(x)
	checks := map[string]health.Check{
		"falkordb":  func(context.Context) error { return falkor.Ping(db) },
		"typesense": health.Typesense(cfg.TypesenseURL),
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(checks, store.ActorFor, b, x),
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

// rebuild loads the search model, then refills every workspace's search
// index from the board. The board works without search, so a failure is
// logged and the service starts anyway.
func rebuild(x *search.Index) {
	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()
	if err := x.Warm(ctx); err != nil {
		log.Printf("search: loading the model failed, search is unavailable until a reindex: %v", err)
		return
	}
	if err := x.RebuildAll(ctx); err != nil {
		log.Printf("search: rebuild failed, search may be incomplete until a reindex: %v", err)
		return
	}
	log.Print("search: index rebuilt")
}
