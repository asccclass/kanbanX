package main

import (
	"fmt"
	"math/rand"
	"time"
)

// ─── Priority ────────────────────────────────────────────────────────────────

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

func (p Priority) Valid() bool {
	return p == PriorityHigh || p == PriorityMedium || p == PriorityLow
}

// ─── Core Models ─────────────────────────────────────────────────────────────

// Card represents a single task on the board.
type Card struct {
	ID          string    `json:"id"`
	ColumnID    string    `json:"columnId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    Priority  `json:"priority"`
	Assignee    string    `json:"assignee"`
	Labels      []string  `json:"labels"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Column represents a stage lane on the board.
type Column struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"boardId"`
	Title     string    `json:"title"`
	Color     string    `json:"color"`
	Position  int       `json:"position"`
	Cards     []*Card   `json:"cards"`
	CreatedAt time.Time `json:"createdAt"`
}

// Board is the top-level container.
type Board struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Columns   []*Column `json:"columns"`
	CreatedAt time.Time `json:"createdAt"`
}

// ─── Request / Response DTOs ─────────────────────────────────────────────────

type AddCardRequest struct {
	ColumnID    string   `json:"columnId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    Priority `json:"priority"`
	Assignee    string   `json:"assignee"`
	Labels      []string `json:"labels"`
}

type UpdateCardRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    Priority `json:"priority"`
	Assignee    string   `json:"assignee"`
	Labels      []string `json:"labels"`
}

type MoveCardRequest struct {
	CardID       string `json:"cardId"`
	FromColumnID string `json:"fromColumnId"`
	ToColumnID   string `json:"toColumnId"`
	ToIndex      int    `json:"toIndex"`
}

type AddColumnRequest struct {
	Title string `json:"title"`
	Color string `json:"color"`
}

type UpdateColumnRequest struct {
	Title string `json:"title"`
	Color string `json:"color"`
}

// ─── ID Generator ─────────────────────────────────────────────────────────────

func generateID() string {
	return fmt.Sprintf("%d%04d", time.Now().UnixNano(), rand.Intn(9999))
}

// ─── API response helpers ──────────────────────────────────────────────────────

type APIResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func okResp(data interface{}) APIResponse {
	return APIResponse{Status: "ok", Data: data}
}

func errResp(msg string) APIResponse {
	return APIResponse{Status: "error", Message: msg}
}
