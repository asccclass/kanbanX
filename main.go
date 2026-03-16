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

	// ── SQLite Store ────────────────────────────────────────────────────────
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

	// ── WebSocket Hub ───────────────────────────────────────────────────────
	hub := NewHub()
	go hub.Run()

	// ── HTTP Server ─────────────────────────────────────────────────────────
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
	fmt.Printf("  🚀  Server     → http://localhost:%s\n", port)
	fmt.Printf("  📡  WebSocket  → ws://localhost:%s/ws\n", port)
	fmt.Printf("  🗄️   Database   → %s\n", dbPath)
	fmt.Printf("  📂  Static     → %s\n\n", documentRoot)
	fmt.Printf("  API Endpoints:\n")
	fmt.Printf("    GET    /api/board\n")
	fmt.Printf("    GET    /api/cards/{id}\n")
	fmt.Printf("    POST   /api/cards\n")
	fmt.Printf("    POST   /api/cards/move\n")
	fmt.Printf("    PUT    /api/cards/{id}\n")
	fmt.Printf("    DELETE /api/cards/{id}\n")
	fmt.Printf("    GET    /api/columns\n")
	fmt.Printf("    GET    /api/columns/{id}\n")
	fmt.Printf("    GET    /api/columns/{id}/cards\n")
	fmt.Printf("    POST   /api/columns\n")
	fmt.Printf("    PUT    /api/columns/{id}\n")
	fmt.Printf("    DELETE /api/columns/{id}\n\n")

	server.Start()
}
