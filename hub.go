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
	CheckOrigin:      func(r *http.Request) bool { return true }, // allow all origins
}

// ─── Message envelope ─────────────────────────────────────────────────────────

// WSMessage is the standard envelope for all WebSocket messages.
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ─── Client ───────────────────────────────────────────────────────────────────

// Client represents a single connected WebSocket peer.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	once sync.Once
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

// writePump sends messages from the hub to the WebSocket connection.
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

// readPump drains incoming frames (keeps connection alive, detects disconnect).
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

// Hub manages all active WebSocket clients and broadcasts messages to them all.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run is the Hub's main event loop; call as a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("[WS] client connected  | online: %d", count)
			h.broadcastOnlineCount(count)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("[WS] client disconnected | online: %d", count)
			h.broadcastOnlineCount(count)

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Slow/dead client — drop it
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.Unlock()
		}
	}
}

// Broadcast serialises a typed message and queues it for all clients.
func (h *Hub) Broadcast(msgType string, payload interface{}) {
	data, err := json.Marshal(WSMessage{Type: msgType, Payload: payload})
	if err != nil {
		log.Printf("[WS] marshal error: %v", err)
		return
	}
	select {
	case h.broadcast <- data:
	default:
		log.Printf("[WS] broadcast channel full, message dropped")
	}
}

func (h *Hub) broadcastOnlineCount(count int) {
	data, _ := json.Marshal(WSMessage{Type: "online_count", Payload: count})
	select {
	case h.broadcast <- data:
	default:
	}
}

// OnlineCount returns the number of currently connected clients.
func (h *Hub) OnlineCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
