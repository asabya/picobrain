package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asabya/picobrain"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var newBrain = picobrain.New

var startHTTPServer = func(httpServer *server.StreamableHTTPServer, addr string) error {
	return httpServer.Start(addr)
}

var startAuthHTTPServer = func(httpServer *http.Server, addr string) error {
	if httpServer.Addr == "" {
		httpServer.Addr = addr
	}
	return httpServer.ListenAndServe()
}

type basicAuthCredentials struct {
	username string
	password string
}

func main() {
	if len(os.Args) < 2 {
		runServer(os.Args[1:])
		return
	}

	switch os.Args[1] {
	case "export":
		runExport(os.Args[2:])
	case "import":
		runImport(os.Args[2:])
	case "serve", "server":
		runServer(os.Args[2:])
	default:
		if strings.HasPrefix(os.Args[1], "-") {
			runServer(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			fmt.Fprintf(os.Stderr, "Usage: picobrain [command] [options]\n")
			fmt.Fprintf(os.Stderr, "Commands: serve, export, import\n")
			os.Exit(1)
		}
	}
}

func runServer(args []string) {
	if err := runServerE(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServerE(args []string) error {
	defaults := picobrain.DefaultConfig()

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", defaults.DBPath, "path to brain database")
	embedModel := fs.String("embed-model", defaults.EmbedModel, "embedding model name (e.g. nomic-embed-text-v1.5)")
	modelCache := fs.String("model-cache", defaults.ModelCacheDir, "directory to cache downloaded models")
	noAutoDownload := fs.Bool("no-auto-download", false, "disable automatic model download (fail if model not cached)")
	port := fs.String("port", "8080", "HTTP listen port")
	autoPruneDays := fs.Int("auto-prune-days", defaults.AutoPruneDays, "automatically prune thoughts older than N days (0 to disable)")
	prune := fs.Bool("prune", false, "run manual prune and exit")
	namespace := fs.String("namespace", defaults.DefaultNamespace, "default namespace for thoughts (e.g., 'default', 'project-alpha')")
	filteredArgs, authCreds, err := extractAuthFlag(args)
	if err != nil {
		return fmt.Errorf("%w\nhint: start picobrain with --auth=username:password (example: picobrain serve --auth=alice:secret)", err)
	}
	if err := fs.Parse(filteredArgs); err != nil {
		return err
	}

	cfg := picobrain.Config{
		DBPath:           *dbPath,
		EmbedModel:       *embedModel,
		ModelCacheDir:    *modelCache,
		AutoDownload:     !*noAutoDownload,
		AutoPruneDays:    *autoPruneDays,
		DefaultNamespace: *namespace,
	}

	brain, err := newBrain(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize brain: %w", err)
	}
	defer brain.Close()

	if *prune {
		ctx := context.Background()
		deleted, err := brain.Prune(ctx, cfg.AutoPruneDays)
		if err != nil {
			return fmt.Errorf("prune failed: %w", err)
		}
		fmt.Printf("Pruned %d thought(s) older than %d days\n", deleted, cfg.AutoPruneDays)
		return nil
	}

	if cfg.AutoPruneDays > 0 {
		ctx := context.Background()
		deleted, err := brain.Prune(ctx, cfg.AutoPruneDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "auto-prune failed: %v\n", err)
		} else if deleted > 0 {
			fmt.Printf("Auto-pruned %d thought(s) older than %d days\n", deleted, cfg.AutoPruneDays)
		}
	}

	s := server.NewMCPServer("picobrain", "0.1.0",
		server.WithPromptCapabilities(false),
	)
	picobrain.RegisterMCPTools(s, brain)

	s.AddPrompt(
		mcp.NewPrompt("observe",
			mcp.WithPromptDescription("System prompt for compressing conversation messages into dense observations"),
		),
		func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return mcp.NewGetPromptResult("System prompt for observational memory", []mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.Role("system"), mcp.NewTextContent(picobrain.ObserverPrompt)),
			}), nil
		},
	)

	s.AddPrompt(
		mcp.NewPrompt("reflect",
			mcp.WithPromptDescription("System prompt for consolidating and pruning observations"),
		),
		func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return mcp.NewGetPromptResult("System prompt for memory reflection", []mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.Role("system"), mcp.NewTextContent(picobrain.ReflectorPrompt)),
			}), nil
		},
	)
	streamableHTTPServer := server.NewStreamableHTTPServer(s)
	printStartupBanner(*port)
	if authCreds != nil {
		protectedMux := http.NewServeMux()
		protectedMux.Handle("/mcp", wrapBasicAuth(streamableHTTPServer, *authCreds))
		if err := startAuthHTTPServer(&http.Server{Handler: protectedMux}, ":"+*port); err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
	if err := startHTTPServer(streamableHTTPServer, ":"+*port); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func runExport(args []string) {
	cmd := newExportCommand()
	if err := cmd.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runImport(args []string) {
	cmd := newImportCommand()
	if err := cmd.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printStartupBanner(port string) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║                  🧠 PICOBRAIN v0.1.0                    ║")
	fmt.Println("║         Local semantic memory for AI agents              ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Status:     ✅ Running                                  ║\n")
	fmt.Printf("║  Endpoint:   http://localhost:%s/mcp                      ║\n", port)
	fmt.Printf("║  Time:       %s                           ║\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Println("║  Tools available:                                        ║")
	fmt.Println("║    • store_thought   - Save observations & facts         ║")
	fmt.Println("║    • semantic_search - Find memories by meaning          ║")
	fmt.Println("║    • list_recent     - Review recent thoughts            ║")
	fmt.Println("║    • stats           - Check memory statistics           ║")
	fmt.Println("║    • health          - Verify server health              ║")
	fmt.Println("║    • reflect         - Consolidate old observations      ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Println("║  Tips:                                                   ║")
	fmt.Println("║    → Store observations often (after every action)       ║")
	fmt.Println("║    → Search before asking users to repeat context        ║")
	fmt.Println("║    → Reflect periodically to keep memory efficient       ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func parseAuthFlag(raw string) (basicAuthCredentials, error) {
	username, password, ok := strings.Cut(raw, ":")
	if !ok || username == "" || password == "" {
		return basicAuthCredentials{}, fmt.Errorf("invalid --auth value %q: expected username:password", raw)
	}
	return basicAuthCredentials{username: username, password: password}, nil
}

func extractAuthFlag(args []string) ([]string, *basicAuthCredentials, error) {
	filtered := make([]string, 0, len(args))
	var creds *basicAuthCredentials
	for _, arg := range args {
		switch {
		case arg == "--auth" || arg == "-auth":
			return nil, nil, fmt.Errorf("invalid --auth value %q: expected username:password", arg)
		case strings.HasPrefix(arg, "--auth=") || strings.HasPrefix(arg, "-auth="):
			if creds != nil {
				return nil, nil, fmt.Errorf("duplicate --auth flag")
			}
			raw := strings.SplitN(arg, "=", 2)[1]
			parsed, err := parseAuthFlag(raw)
			if err != nil {
				return nil, nil, err
			}
			creds = &parsed
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered, creds, nil
}

func wrapBasicAuth(next http.Handler, creds basicAuthCredentials) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(username), []byte(creds.username)) != 1 || subtle.ConstantTimeCompare([]byte(password), []byte(creds.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="picobrain"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
