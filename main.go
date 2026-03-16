package main

import (
	"fmt"
	"os"

	SherryServer "github.com/asccclass/sherryserver"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load("envfile"); err != nil {
		fmt.Println("Warning: envfile not found, using defaults")
	}

	// ── Mode selection ───────────────────────────────────────────────────────
	// Run as MCP stdio server when --mcp flag is passed.
	// Everything else starts the normal HTTP server.
	for _, arg := range os.Args[1:] {
		if arg == "--mcp" {
			dbPath := os.Getenv("DBPath")
			if dbPath == "" {
				dbPath = "kanban.db"
			}
			RunMCPServer(dbPath)
			return
		}
	}

	// ── HTTP Server mode ─────────────────────────────────────────────────────
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
	dbPath := os.Getenv("DBPath")
	if dbPath == "" {
		dbPath = "kanban.db"
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Printf("  ✗ Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.EnsureDefaultBoard(); err != nil {
		fmt.Printf("  ✗ Failed to seed database: %v\n", err)
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
	fmt.Printf("  🚀  HTTP Server → http://localhost:%s\n", port)
	fmt.Printf("  📡  WebSocket   → ws://localhost:%s/ws\n", port)
	fmt.Printf("  🗄️   Database    → %s\n", dbPath)
	fmt.Printf("  🤖  MCP mode    → run with --mcp flag\n\n")

	server.Start()
}
