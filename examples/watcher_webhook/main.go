// watcher_webhook demonstrates running the background Windows Update watcher
// with a webhook emitter that fires on build discovery / update events.
//
// The program starts the watcher, registers a webhook POST target, and runs
// until interrupted. It also prints each event to stdout.
//
// Usage:
//
//	go run ./examples/watcher_webhook \
//	  --db      catalog.db \
//	  --webhook https://hooks.example.internal/winupdate \
//	  --interval 30m
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/catalog"
	"github.com/deploymenttheory/go-sdk-uupdump/catalog/events"
	"github.com/deploymenttheory/go-sdk-uupdump/catalog/store"
	"github.com/deploymenttheory/go-sdk-uupdump/catalog/watcher"
	"github.com/deploymenttheory/go-sdk-uupdump/wuproto/soap"
	"go.uber.org/zap"
)

func main() {
	dbPath := flag.String("db", "catalog.db", "SQLite catalog database path")
	webhookURL := flag.String("webhook", "", "webhook URL for build events (optional)")
	interval := flag.Duration("interval", 30*time.Minute, "watcher poll interval")
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	// ── Catalog store ────────────────────────────────────────────────────────
	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open catalog: %v", err)
	}
	defer st.Close() //nolint:errcheck

	// ── Event bus ────────────────────────────────────────────────────────────
	bus := events.NewBus()

	// Print every event to stdout.
	unsubscribe := bus.Subscribe(catalog.EventHandlerFunc(func(ctx context.Context, e catalog.BuildEvent) {
		fmt.Printf("[%s] %s %s (%s/%s)\n",
			time.Now().Format(time.RFC3339),
			e.Type, e.Build.UUID, e.Build.Build, e.Build.Arch,
		)
	}))
	defer unsubscribe()

	// Optional webhook emitter.
	if *webhookURL != "" {
		hook := events.NewWebhookEmitter(*webhookURL, nil, logger)
		unsubHook := bus.Subscribe(catalog.EventHandlerFunc(func(ctx context.Context, e catalog.BuildEvent) {
			hook.HandleEvent(ctx, e)
		}))
		defer unsubHook()
		fmt.Printf("webhook emitter active → %s\n", *webhookURL)
	}

	// ── WU SOAP client ───────────────────────────────────────────────────────
	wuClient, err := soap.New(logger)
	if err != nil {
		log.Fatalf("init WU client: %v", err)
	}

	// ── Watcher ──────────────────────────────────────────────────────────────
	w := watcher.New(wuClient, st, bus, logger, *interval, nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "watcher starting (interval=%s, db=%s)\n", *interval, *dbPath)
	w.Start(ctx)

	<-ctx.Done()
	fmt.Fprintln(os.Stderr, "shutting down watcher…")
	w.Stop()
	fmt.Fprintln(os.Stderr, "done")
}
