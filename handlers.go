package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

)

type Handler struct {
	store *SQLiteStore
	hub   *Hub
}

func NewHandler(store *SQLiteStore, hub *Hub) *Handler {
	return &Handler{store: store, hub: hub}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

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

// telegramID extracts Telegram ID from ?telegram_id= query param or JSON body field.
// Falls back to a default "admin" sentinel so the web UI still works without a Telegram ID.
func (h *Handler) telegramID(r *http.Request) string {
	if tid := strings.TrimSpace(r.URL.Query().Get("telegram_id")); tid != "" {
		return tid
	}
	return "web_admin" // Web UI uses a shared admin board
}

func (h *Handler) broadcastBoard(telegramID string) {
	board := h.store.GetCachedBoard(telegramID)
	if board != nil {
		h.hub.Broadcast("board_update", board)
	}
}

// ─── Board ────────────────────────────────────────────────────────────────────

// GET /api/board?telegram_id=xxx
func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	board, err := h.store.EnsureUserBoard(tid)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

// GET /api/users — list all users who have a board (for web UI switcher)
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListAllUsers()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okResp(users))
}

// ─── Cards ────────────────────────────────────────────────────────────────────

func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	cardID := r.PathValue("id")
	if err := h.store.assertCardOwnership(tid, cardID); err != nil {
		notFound(w, err.Error()); return
	}
	card, err := h.store.GetCard(cardID)
	if err != nil {
		notFound(w, err.Error()); return
	}
	writeJSON(w, http.StatusOK, okResp(card))
}

func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	colID := r.PathValue("id")
	if err := h.store.assertColumnOwnership(tid, colID); err != nil {
		notFound(w, err.Error()); return
	}
	cards, err := h.store.ListCards(colID)
	if err != nil {
		internalError(w, err); return
	}
	writeJSON(w, http.StatusOK, okResp(cards))
}

func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	var req AddCardRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error()); return
	}
	if req.Title == "" {
		badRequest(w, "title is required"); return
	}
	if req.ColumnID == "" {
		badRequest(w, "columnId is required"); return
	}
	card, err := h.store.CreateCard(tid, &req)
	if err != nil {
		internalError(w, err); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusCreated, okResp(card))
}

func (h *Handler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	var req UpdateCardRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error()); return
	}
	if req.Title == "" {
		badRequest(w, "title is required"); return
	}
	card, err := h.store.UpdateCard(tid, r.PathValue("id"), &req)
	if err != nil {
		notFound(w, err.Error()); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusOK, okResp(card))
}

func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	if err := h.store.DeleteCard(tid, r.PathValue("id")); err != nil {
		notFound(w, err.Error()); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusOK, okResp(nil))
}

func (h *Handler) MoveCard(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	var req MoveCardRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error()); return
	}
	if req.CardID == "" || req.FromColumnID == "" || req.ToColumnID == "" {
		badRequest(w, "cardId, fromColumnId, toColumnId are required"); return
	}
	if err := h.store.MoveCard(tid, &req); err != nil {
		notFound(w, err.Error()); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusOK, okResp(nil))
}

// ─── Columns ──────────────────────────────────────────────────────────────────

func (h *Handler) ListColumns(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	cols, err := h.store.ListColumns(tid)
	if err != nil {
		internalError(w, err); return
	}
	writeJSON(w, http.StatusOK, okResp(cols))
}

func (h *Handler) GetColumn(w http.ResponseWriter, r *http.Request) {
	col, err := h.store.GetColumn(r.PathValue("id"))
	if err != nil {
		notFound(w, err.Error()); return
	}
	writeJSON(w, http.StatusOK, okResp(col))
}

func (h *Handler) CreateColumn(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	var req AddColumnRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error()); return
	}
	if req.Title == "" {
		badRequest(w, "title is required"); return
	}
	col, err := h.store.CreateColumn(tid, &req)
	if err != nil {
		internalError(w, err); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusCreated, okResp(col))
}

func (h *Handler) UpdateColumn(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	var req UpdateColumnRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid JSON: "+err.Error()); return
	}
	col, err := h.store.UpdateColumn(tid, r.PathValue("id"), &req)
	if err != nil {
		notFound(w, err.Error()); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusOK, okResp(col))
}

func (h *Handler) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	tid := h.telegramID(r)
	if err := h.store.DeleteColumn(tid, r.PathValue("id")); err != nil {
		notFound(w, err.Error()); return
	}
	h.broadcastBoard(tid)
	writeJSON(w, http.StatusOK, okResp(nil))
}

// ─── WebSocket ────────────────────────────────────────────────────────────────

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		return
	}

	tid := h.telegramID(r)
	// Ensure the user's board exists before subscribing
	board, err := h.store.EnsureUserBoard(tid)
	if err != nil {
		conn.Close()
		return
	}

	client := &Client{hub: h.hub, conn: conn, send: make(chan []byte, 256)}
	h.hub.register <- client

	// Send current board state immediately
	if data, err := json.Marshal(WSMessage{Type: "board_update", Payload: board}); err == nil {
		select {
		case client.send <- data:
		default:
		}
	}

	go client.writePump()
	client.readPump()
}
