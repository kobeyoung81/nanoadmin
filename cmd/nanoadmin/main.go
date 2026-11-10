// Command nanoadmin serves the platform's admin domain: the agent-type,
// model, capability, prompt, permission-key, audit and configuration catalogues.
//
// The bus keeps only messages; everything a console edits lives here. This
// service writes only the admin_ prefix and reads nothing outside it — other
// modules' facts arrive over HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/agenttypes"
	"gitee.com/kobeyoung81/nanoadmin/internal/busclient"
	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/permissionkeys"
	"gitee.com/kobeyoung81/nanoadmin/internal/promptbundle"
	"gitee.com/kobeyoung81/nanoadmin/internal/seed"
	"gitee.com/kobeyoung81/nanoadmin/internal/server"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoadmin/migrations"
)

// version is stamped at build time (see Dockerfile).
var version = "dev"

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "serve":
		if err := serve(); err != nil {
			slog.Error("nanoadmin stopped", "error", err)
			os.Exit(1)
		}
	case "config":
		if err := configCmd(args); err != nil {
			slog.Error("config failed", "error", err)
			os.Exit(1)
		}
	case "permissions":
		if err := permissionsCmd(args); err != nil {
			slog.Error("permissions failed", "error", err)
			os.Exit(1)
		}
	case "agent-types":
		if err := agentTypesSeed(); err != nil {
			slog.Error("agent-types failed", "error", err)
			os.Exit(1)
		}
	case "prompts":
		if err := promptsSeed(args); err != nil {
			slog.Error("prompts failed", "error", err)
			os.Exit(1)
		}
	case "migrate":
		if err := migrate(args); err != nil {
			slog.Error("migrate failed", "error", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "usage: nanoadmin [serve|config {seed|set|get}|permissions seed|agent-types seed|prompts seed|migrate {up|status}|version]\n")
		os.Exit(2)
	}
}

func serve() error {
	cfg := config.Load()

	st, err := store.Open(cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// Apply the manifests before listening: a fresh database then comes up with
	// its indexes already in place, and a second start is a no-op.
	if err := applyMigrations(st); err != nil {
		return err
	}

	// Deleting a type must clear the agents pointing at it; their rows belong to
	// the portal, so the delete asks the owner (through the bus, which keeps the
	// internal contract it has always exposed) to do its part first.
	bus := busclient.New(cfg.BusURL, cfg.InternalToken)
	st.SetTypeRefClearer(func(typeID string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return bus.ClearAgentTypeRefs(ctx, typeID)
	})

	handler := server.New(cfg, st).WithCatalog(st).Handler()
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("nanoadmin listening", "addr", cfg.Addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// configCmd runs the config subcommands. Only the seed still belongs to this
// service's command line: reads are served over HTTP, and the console writes
// through the same API.
func configCmd(args []string) error {
	sub := "seed"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	cfg := config.Load()

	switch sub {
	case "seed":
		st, err := store.Open(cfg.MongoURI)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		if err := applyMigrations(st); err != nil {
			return err
		}
		for componentType, items := range seed.DefaultConfigItems() {
			if err := st.SeedConfigItems(componentType, items); err != nil {
				return fmt.Errorf("seed %s: %w", componentType, err)
			}
			fmt.Printf("Seeded %d config items for %q.\n", len(items), componentType)
		}
		return nil
	case "set":
		if len(args) != 3 {
			return fmt.Errorf("usage: nanoadmin config set <component-type> <key> <value>")
		}
		st, err := store.Open(cfg.MongoURI)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		if err := st.SetConfigItemTyped(args[0], args[1], args[2], "", ""); err != nil {
			return fmt.Errorf("set config item: %w", err)
		}
		fmt.Printf("Config %s.%s updated.\n", args[0], args[1])
		return nil
	case "get":
		if len(args) < 1 {
			return fmt.Errorf("usage: nanoadmin config get <component-type> [key]")
		}
		st, err := store.Open(cfg.MongoURI)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		if len(args) == 2 {
			item, ok, err := st.GetConfigItem(args[0], args[1])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no such config item: %s.%s", args[0], args[1])
			}
			fmt.Printf("%s = %s  [%s] %s\n", item.Key, item.Value, item.ValueType, item.Description)
			return nil
		}
		items, err := st.GetConfigItems(args[0])
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Printf("  %-40s = %-30s [%s]\n", item.Key, item.Value, item.ValueType)
		}
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q (want seed, set or get)", sub)
	}
}

// agentTypesSeed inserts the built-in agent types this binary ships, leaving
// anything already there untouched: prompts, permissions and settings are
// admin-editable and a deploy must not undo those edits.
func agentTypesSeed() error {
	cfg := config.Load()
	st, err := store.Open(cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if err := applyMigrations(st); err != nil {
		return err
	}
	defs := agenttypes.Definitions()
	inserted, err := st.SeedAgentTypes(defs)
	if err != nil {
		return fmt.Errorf("seed agent types: %w", err)
	}
	fmt.Printf("Seeded %d agent type(s) (%d inserted, %d already present).\n",
		len(defs), inserted, len(defs)-inserted)
	return nil
}

// promptsSeed writes the bundled prompts onto the seeded types and the shared
// head prompt onto its own document. --force also replaces edited text.
func promptsSeed(args []string) error {
	force := false
	for _, a := range args {
		if a == "--force" {
			force = true
		}
	}
	cfg := config.Load()
	st, err := store.Open(cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if err := applyMigrations(st); err != nil {
		return err
	}
	bundled, err := promptbundle.Load()
	if err != nil {
		return fmt.Errorf("load prompt bundle: %w", err)
	}
	updated, skipped, err := st.SeedPlatformPrompts(bundled, force)
	if err != nil {
		return fmt.Errorf("seed prompts: %w", err)
	}
	head, err := promptbundle.LoadSharedHead()
	if err != nil {
		return fmt.Errorf("load shared head prompt: %w", err)
	}
	headState := "absent"
	if head.Text != "" {
		headUpdated, err := st.SeedSessionHead(head, force)
		if err != nil {
			return fmt.Errorf("seed shared head prompt: %w", err)
		}
		headState = "left as-is"
		if headUpdated {
			headState = "updated"
		}
	}
	fmt.Printf("Seeded %d bundled prompt(s) (%d updated, %d left as-is); shared head prompt %s.\n",
		len(bundled), updated, skipped, headState)
	return nil
}

// permissionsCmd seeds the permission key catalog this binary ships.
//
// Seeding only inserts what is missing: descriptions and default actions are
// admin-editable, and re-running this on every deploy must not undo those edits.
func permissionsCmd(args []string) error {
	sub := "seed"
	if len(args) > 0 {
		sub = args[0]
	}
	cfg := config.Load()

	switch sub {
	case "seed":
		st, err := store.Open(cfg.MongoURI)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		if err := applyMigrations(st); err != nil {
			return err
		}
		keys := permissionkeys.Keys()
		inserted, err := st.SeedPermissionKeys(keys)
		if err != nil {
			return fmt.Errorf("seed permission keys: %w", err)
		}
		fmt.Printf("Seeded %d permission key(s) (%d inserted, %d already present).\n",
			len(keys), inserted, len(keys)-inserted)
		return nil
	default:
		return fmt.Errorf("unknown permissions subcommand %q (want seed)", sub)
	}
}

func migrate(args []string) error {
	sub := "up"
	if len(args) > 0 {
		sub = args[0]
	}
	cfg := config.Load()
	st, err := store.Open(cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	switch sub {
	case "up":
		manifests, err := migrations.Load()
		if err != nil {
			return err
		}
		ran, err := st.Migrate(manifests)
		if err != nil {
			return err
		}
		if len(ran) == 0 {
			fmt.Println("Database is up to date.")
			return nil
		}
		for _, name := range ran {
			fmt.Printf("Applied: %s\n", name)
		}
		return nil
	case "status":
		manifests, err := migrations.Load()
		if err != nil {
			return err
		}
		infos, err := st.MigrationStatus(manifests)
		if err != nil {
			return err
		}
		for _, info := range infos {
			state := "pending"
			if info.Applied {
				state = "applied " + info.AppliedAt.Format(time.RFC3339)
			}
			fmt.Printf("%-32s %s\n", info.Name, state)
		}
		return nil
	default:
		return fmt.Errorf("unknown migrate subcommand %q (want up or status)", sub)
	}
}

func applyMigrations(st *store.Store) error {
	manifests, err := migrations.Load()
	if err != nil {
		return err
	}
	ran, err := st.Migrate(manifests)
	if err != nil {
		return err
	}
	if len(ran) > 0 {
		slog.Info("applied migrations", "count", len(ran))
	}
	return nil
}
