package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/asabya/picobrain"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
	defaults := picobrain.DefaultConfig()

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", defaults.DBPath, "path to brain database")
	embedModel := fs.String("embed-model", defaults.EmbedModel, "embedding model name (e.g. nomic-embed-text-v1.5)")
	modelCache := fs.String("model-cache", defaults.ModelCacheDir, "directory to cache downloaded models")
	noAutoDownload := fs.Bool("no-auto-download", false, "disable automatic model download (fail if model not cached)")
	port := fs.String("port", "8080", "HTTP listen port")
	autoPruneDays := fs.Int("auto-prune-days", defaults.AutoPruneDays, "automatically prune thoughts older than N days (0 to disable)")
	prune := fs.Bool("prune", false, "run manual prune and exit")
	fs.Parse(args)

	cfg := picobrain.Config{
		DBPath:        *dbPath,
		EmbedModel:    *embedModel,
		ModelCacheDir: *modelCache,
		AutoDownload:  !*noAutoDownload,
		AutoPruneDays: *autoPruneDays,
	}

	brain, err := picobrain.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize brain: %v\n", err)
		os.Exit(1)
	}
	defer brain.Close()

	if *prune {
		ctx := context.Background()
		deleted, err := brain.Prune(ctx, cfg.AutoPruneDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prune failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Pruned %d thought(s) older than %d days\n", deleted, cfg.AutoPruneDays)
		os.Exit(0)
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

	printStartupBanner(*port)

	httpServer := server.NewStreamableHTTPServer(s)
	if err := httpServer.Start(":" + *port); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
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
