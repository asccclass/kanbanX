package main

import (
	"net/http"

	SherryServer "github.com/asccclass/sherryserver"
)

func NewRouter(srv *SherryServer.Server, documentRoot string, store *SQLiteStore, hub *Hub) *http.ServeMux {
	router := http.NewServeMux()
	h := NewHandler(store, hub)

	staticFileServer := SherryServer.StaticFileServer{StaticPath: documentRoot, IndexPath: "index.html"}
	staticFileServer.AddRouter(router)

	router.HandleFunc("GET /api/board", h.GetBoard)
	router.HandleFunc("GET /api/users", h.ListUsers)

	router.HandleFunc("GET  /api/cards/{id}", h.GetCard)
	router.HandleFunc("POST /api/cards", h.CreateCard)
	router.HandleFunc("POST /api/cards/move", h.MoveCard)
	router.HandleFunc("PUT  /api/cards/{id}", h.UpdateCard)
	router.HandleFunc("DELETE /api/cards/{id}", h.DeleteCard)

	router.HandleFunc("GET  /api/columns", h.ListColumns)
	router.HandleFunc("GET  /api/columns/{id}", h.GetColumn)
	router.HandleFunc("GET  /api/columns/{id}/cards", h.ListCards)
	router.HandleFunc("POST /api/columns", h.CreateColumn)
	router.HandleFunc("PUT  /api/columns/{id}", h.UpdateColumn)
	router.HandleFunc("DELETE /api/columns/{id}", h.DeleteColumn)

	router.HandleFunc("GET /ws", h.ServeWS)
	return router
}
