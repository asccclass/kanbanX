package main

import (
	"fmt"
	"os"

	SherryServer "github.com/asccclass/sherryserver"
	"github.com/joho/godotenv"
)

func main() {
	// ── Detect MCP mode FIRST, before any stdout output ─────────────────────
	// In MCP stdio mode stdout is a pure JSON-RPC channel.
	// Nothing may be written to stdout before (or instead of) JSON.
	mcpMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--mcp" {
			mcpMode = true
			break
		}
	}

	// ── Load envfile ─────────────────────────────────────────────────────────
	if err := godotenv.Load("envfile"); err != nil {
		if mcpMode {
			// MCP: diagnostics go to stderr only — stdout must stay clean
			fmt.Fprintln(os.Stderr, "[kanbanx] warning: envfile not found, using defaults")
		} else {
			fmt.Println("Warning: envfile not found, using defaults")
		}
	}

	dbPath := os.Getenv("DBPath")
	if dbPath == "" {
		dbPath = "kanban.db"
	}

	// ── MCP stdio server mode ────────────────────────────────────────────────
	if mcpMode {
		RunMCPServer(dbPath)
		return
	}

	// ── HTTP server mode ─────────────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	documentRoot := os.Getenv("DocumentRoot")
	if documentRoot == "" {
		documentRoot = "www/html"
	}
	templateRoot := os.Getenv("TemplateRoot")
	if templateRoot == "" {
		templateRoot = "www/template"
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.EnsureDefaultBoard(); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Failed to seed database: %v\n", err)
		os.Exit(1)
	}

	hub := NewHub()
	go hub.Run()

	server, err := SherryServer.NewServer(":"+port, documentRoot, templateRoot)
	if err != nil {
		panic(err)
	}

	router := NewRouter(server, documentRoot, store, hub)
	server.Server.Handler = router

	fmt.Printf("\n")
	fmt.Printf("  ██╗  ██╗ █████╗ ███╗   ██╗██████╗  █████╗ ███╗   ██╗██╗  ██╗\n")
	fmt.Printf("  ██║ ██╔╝██╔══██╗████╗  ██║██╔══██╗██╔══██╗████╗  ██║╚██╗██╔╝\n")
	fmt.Printf("  █████╔╝ ███████║██╔██╗ ██║██████╔╝███████║██╔██╗ ██║ ╚███╔╝ \n")
	fmt.Printf("  ██╔═██╗ ██╔══██║██║╚██╗██║██╔══██╗██╔══██║██║╚██╗██║ ██╔██╗ \n")
	fmt.Printf("  ██║  ██╗██║  ██║██║ ╚████║██████╔╝██║  ██║██║ ╚████║██╔╝ ██╗\n")
	fmt.Printf("  ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝  ╚═╝\n")
	fmt.Printf("\n")
	fmt.Printf("  🚀  HTTP Server → http://[IP_ADDRESS] or http://localhost:%s\n", port)
	fmt.Printf("  📡  WebSocket   → ws://[IP_ADDRESS] or ws://localhost:%s/ws\n", port)
	fmt.Printf("  🗄️   Database    → %s\n", dbPath)
	fmt.Printf("  🤖  MCP mode    → run with --mcp flag\n\n")

	server.Start()
}
