package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ─── WebSocket upgrader ───────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  4096,
	CheckOrigin:      func(r *http.Request) bool { return true },
}

// ─── Message envelope ─────────────────────────────────────────────────────────

type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ─── Client ───────────────────────────────────────────────────────────────────

// Client represents one connected WebSocket peer.
// telegramID links the client to a specific user's board so that
// board-update broadcasts are scoped per user.
type Client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	telegramID string // owner of this connection
	once       sync.Once
}

func (c *Client) safeClose() {
	c.once.Do(func() {
		c.hub.unregister <- c
	})
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4096
)

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		c.safeClose()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer c.safeClose()
	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] read error: %v", err)
			}
			return
		}
	}
}

// ─── Hub ──────────────────────────────────────────────────────────────────────

// Hub manages all connected WebSocket clients.
// boardBroadcast is a targeted channel: each message carries a telegramID so
// only that user's clients receive it.
type Hub struct {
	clients       map[*Client]bool
	globalBcast   chan []byte  // sent to every client (online_count etc.)
	boardBcast    chan boardMsg // sent only to clients matching telegramID
	register      chan *Client
	unregister    chan *Client
	mu            sync.Mutex
}

type boardMsg struct {
	telegramID string
	data       []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		globalBcast: make(chan []byte, 256),
		boardBcast:  make(chan boardMsg, 256),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
	}
}

// Run is the Hub's main event loop; must be started as a goroutine.
func (h *Hub) Run() {
	for {
		select {

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("[WS] connected   tid=%-15s online=%d", client.telegramID, count)
			h.broadcastOnlineCount(count)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("[WS] disconnected tid=%-15s online=%d", client.telegramID, count)
			h.broadcastOnlineCount(count)

		// Global broadcast — every client (e.g. online_count)
		case message := <-h.globalBcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.Unlock()

		// Per-user board broadcast — only clients whose telegramID matches
		case msg := <-h.boardBcast:
			h.mu.Lock()
			for client := range h.clients {
				if client.telegramID != msg.telegramID {
					continue
				}
				select {
				case client.send <- msg.data:
				default:
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastBoard sends a board_update only to WebSocket clients that belong
// to the given telegramID.
func (h *Hub) BroadcastBoard(telegramID string, payload interface{}) {
	data, err := json.Marshal(WSMessage{Type: "board_update", Payload: payload})
	if err != nil {
		log.Printf("[WS] marshal error: %v", err)
		return
	}
	select {
	case h.boardBcast <- boardMsg{telegramID: telegramID, data: data}:
	default:
		log.Printf("[WS] boardBcast channel full for tid=%s", telegramID)
	}
}

// Broadcast sends a typed message to every connected client (global).
func (h *Hub) Broadcast(msgType string, payload interface{}) {
	data, err := json.Marshal(WSMessage{Type: msgType, Payload: payload})
	if err != nil {
		log.Printf("[WS] marshal error: %v", err)
		return
	}
	select {
	case h.globalBcast <- data:
	default:
		log.Printf("[WS] globalBcast channel full, dropped")
	}
}

func (h *Hub) broadcastOnlineCount(count int) {
	data, _ := json.Marshal(WSMessage{Type: "online_count", Payload: count})
	select {
	case h.globalBcast <- data:
	default:
	}
}

func (h *Hub) OnlineCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
