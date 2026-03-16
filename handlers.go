package main

import (
	"encoding/json"
	"log"
	"net/http"

)

// Handler bundles the SQLiteStore and Hub.
type Handler struct {
	store *SQLiteStore
	hub   *Hub
}

func NewHandler(store *SQLiteStore, hub *Hub) *Handler {
	return &Handler{store: store, hub: hub}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errResp(msg))
}

func notFound(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotFound, errResp(msg))
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("[API] internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, errResp("internal server error"))
}

func decode(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// broadcastBoard pushes the full board snapshot to every WebSocket client.
func (h *Handler) broadcastBoard() {
	h.hub.Broadcast("board_update", h.store.GetBoard())
}

// ─── Board ────────────────────────────────────────────────────────────────────

// GET /api/board — full board with all columns and cards
func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.GetBoard())
}

// ─── Cards ────────────────────────────────────────────────────────────────────

// GET /api/cards/{id}
func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request) {
	card, err := h.store.GetCard(r.PathValue("id"))
	if err != nil {
		notFound(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, okResp(card))
}

// GET /api/columns/{id}/cards
func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request) {
	cards, err := h.store.ListCards(r.PathValue("id"))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okResp(cards))
}

// POST /api/cards
func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request) {
	var req AddCardRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		badRequest(w, "title is required")
		return
	}
	if req.ColumnID == "" {
		badRequest(w, "columnId is required")
		return
	}

	card, err := h.store.CreateCard(&req)
	if err != nil {
		internalError(w, err)
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusCreated, okResp(card))
}

// PUT /api/cards/{id}
func (h *Handler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	var req UpdateCardRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		badRequest(w, "title is required")
		return
	}

	card, err := h.store.UpdateCard(r.PathValue("id"), &req)
	if err != nil {
		notFound(w, err.Error())
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusOK, okResp(card))
}

// DELETE /api/cards/{id}
func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteCard(r.PathValue("id")); err != nil {
		notFound(w, err.Error())
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusOK, okResp(nil))
}

// POST /api/cards/move
func (h *Handler) MoveCard(w http.ResponseWriter, r *http.Request) {
	var req MoveCardRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error())
		return
	}
	if req.CardID == "" || req.FromColumnID == "" || req.ToColumnID == "" {
		badRequest(w, "cardId, fromColumnId, toColumnId are required")
		return
	}

	if err := h.store.MoveCard(&req); err != nil {
		notFound(w, err.Error())
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusOK, okResp(nil))
}

// ─── Columns ─────────────────────────────────────────────────────────────────

// GET /api/columns
func (h *Handler) ListColumns(w http.ResponseWriter, r *http.Request) {
	cols, err := h.store.ListColumns()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okResp(cols))
}

// GET /api/columns/{id}
func (h *Handler) GetColumn(w http.ResponseWriter, r *http.Request) {
	col, err := h.store.GetColumn(r.PathValue("id"))
	if err != nil {
		notFound(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, okResp(col))
}

// POST /api/columns
func (h *Handler) CreateColumn(w http.ResponseWriter, r *http.Request) {
	var req AddColumnRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		badRequest(w, "title is required")
		return
	}

	col, err := h.store.CreateColumn(&req)
	if err != nil {
		internalError(w, err)
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusCreated, okResp(col))
}

// PUT /api/columns/{id}
func (h *Handler) UpdateColumn(w http.ResponseWriter, r *http.Request) {
	var req UpdateColumnRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error())
		return
	}

	col, err := h.store.UpdateColumn(r.PathValue("id"), &req)
	if err != nil {
		notFound(w, err.Error())
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusOK, okResp(col))
}

// DELETE /api/columns/{id}
func (h *Handler) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteColumn(r.PathValue("id")); err != nil {
		notFound(w, err.Error())
		return
	}
	h.broadcastBoard()
	writeJSON(w, http.StatusOK, okResp(nil))
}

// ─── WebSocket ────────────────────────────────────────────────────────────────

// GET /ws
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:  h.hub,
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.hub.register <- client

	// Push current board snapshot immediately to the new client
	if data, err := json.Marshal(WSMessage{Type: "board_update", Payload: h.store.GetBoard()}); err == nil {
		select {
		case client.send <- data:
		default:
		}
	}

	go client.writePump()
	client.readPump() // blocks until disconnect
}
