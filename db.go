package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// ─── SQLiteStore ──────────────────────────────────────────────────────────────

// SQLiteStore is the persistence layer backed by SQLite.
// It also maintains an in-memory cache of the full Board so WebSocket
// broadcasts don't need a DB round-trip every time GetBoard() is called.
type SQLiteStore struct {
	db          *sql.DB
	mu          sync.RWMutex // guards cache
	cachedBoard *Board
}

// NewSQLiteStore opens (or creates) the SQLite database at path and runs
// schema migrations.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL mode: better concurrent read performance
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// ─── Schema Migration ─────────────────────────────────────────────────────────

const schema = `
CREATE TABLE IF NOT EXISTS boards (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS columns (
    id         TEXT PRIMARY KEY,
    board_id   TEXT NOT NULL,
    title      TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '#6366f1',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS cards (
    id          TEXT PRIMARY KEY,
    column_id   TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority    TEXT NOT NULL DEFAULT 'medium',
    goal_type   TEXT NOT NULL DEFAULT '',
    assignee    TEXT NOT NULL DEFAULT '',
    labels      TEXT NOT NULL DEFAULT '[]',
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_columns_board   ON columns(board_id, position);
CREATE INDEX IF NOT EXISTS idx_cards_column    ON cards(column_id, position);
`

func (s *SQLiteStore) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Idempotent column addition for existing databases that pre-date goal_type
	_, _ = s.db.Exec(`ALTER TABLE cards ADD COLUMN goal_type TEXT NOT NULL DEFAULT ''`)
	return nil
}

// ─── Seed ─────────────────────────────────────────────────────────────────────

// EnsureDefaultBoard creates the default board + seed data if no board exists.
func (s *SQLiteStore) EnsureDefaultBoard() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM boards`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		// Board already exists; just warm the cache
		return s.refreshCache()
	}

	now := time.Now()
	boardID := generateID()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO boards(id,title,created_at) VALUES(?,?,?)`,
		boardID, "專案開發看板", now)
	if err != nil {
		return err
	}

	type colSeed struct{ title, color string }
	type cardSeed struct {
		title, desc, priority, goalType, assignee string
		labels                                     []string
	}
	type seed struct {
		col   colSeed
		cards []cardSeed
	}

	seeds := []seed{
		{colSeed{"待辦事項", "#6366f1"}, []cardSeed{
			{"設計系統微服務架構", "規劃整體微服務架構、API Gateway 設計與技術選型評估", "high", "yearly", "Alice", []string{"架構", "設計"}},
			{"建立 CI/CD Pipeline", "使用 GitHub Actions 建立自動化測試與部署流程", "medium", "monthly", "Bob", []string{"DevOps"}},
			{"撰寫 API 規格文件", "使用 OpenAPI 3.0 規範撰寫完整 REST API 文件", "low", "", "Carol", []string{"文件"}},
		}},
		{colSeed{"進行中", "#f59e0b"}, []cardSeed{
			{"實作 JWT 認證機制", "整合 OAuth 2.0 與 JWT，實作 Refresh Token 邏輯", "high", "weekly", "Dave", []string{"後端", "安全"}},
			{"前端元件庫開發", "以 React + TypeScript 建立可重用 UI 元件系統", "medium", "", "Eve", []string{"前端"}},
		}},
		{colSeed{"審查中", "#8b5cf6"}, []cardSeed{
			{"資料庫索引優化", "分析查詢效能瓶頸，針對高頻查詢建立複合索引", "medium", "monthly", "Frank", []string{"資料庫", "效能"}},
		}},
		{colSeed{"已完成", "#10b981"}, []cardSeed{
			{"需求訪談與分析", "完成所有利害關係人訪談，產出需求規格書 v1.0", "low", "", "Grace", []string{"管理"}},
			{"開發環境建置", "Docker Compose 本機開發環境、環境變數管理", "low", "", "Henry", []string{"DevOps"}},
		}},
	}

	for colPos, s := range seeds {
		colID := generateID()
		_, err = tx.Exec(
			`INSERT INTO columns(id,board_id,title,color,position,created_at) VALUES(?,?,?,?,?,?)`,
			colID, boardID, s.col.title, s.col.color, colPos, now,
		)
		if err != nil {
			return err
		}
		for cardPos, c := range s.cards {
			lblJSON, _ := json.Marshal(c.labels)
			_, err = tx.Exec(`
				INSERT INTO cards(id,column_id,title,description,priority,goal_type,assignee,labels,position,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				generateID(), colID, c.title, c.desc, c.priority, c.goalType, c.assignee,
				string(lblJSON), cardPos, now, now,
			)
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return s.refreshCache()
}

// ─── Cache ────────────────────────────────────────────────────────────────────

// refreshCache rebuilds the in-memory board from SQLite (must NOT hold mu).
func (s *SQLiteStore) refreshCache() error {
	board, err := s.loadBoard()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cachedBoard = board
	s.mu.Unlock()
	return nil
}

// GetBoard returns the cached board snapshot (safe for concurrent reads).
func (s *SQLiteStore) GetBoard() *Board {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedBoard
}

// ─── loadBoard (private, reads DB) ───────────────────────────────────────────

func (s *SQLiteStore) loadBoard() (*Board, error) {
	row := s.db.QueryRow(`SELECT id, title, created_at FROM boards LIMIT 1`)
	board := &Board{}
	if err := row.Scan(&board.ID, &board.Title, &board.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return &Board{Title: "看板", Columns: []*Column{}}, nil
		}
		return nil, err
	}

	// Load columns
	colRows, err := s.db.Query(
		`SELECT id, board_id, title, color, position, created_at
		 FROM columns WHERE board_id=? ORDER BY position`, board.ID)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()

	for colRows.Next() {
		col := &Column{Cards: []*Card{}}
		if err := colRows.Scan(&col.ID, &col.BoardID, &col.Title, &col.Color,
			&col.Position, &col.CreatedAt); err != nil {
			return nil, err
		}
		board.Columns = append(board.Columns, col)
	}
	if board.Columns == nil {
		board.Columns = []*Column{}
	}

	// Load all cards in one query, then distribute into columns
	cardRows, err := s.db.Query(
		`SELECT id, column_id, title, description, priority, goal_type, assignee,
		        labels, position, created_at, updated_at
		 FROM cards
		 WHERE column_id IN (SELECT id FROM columns WHERE board_id=?)
		 ORDER BY column_id, position`, board.ID)
	if err != nil {
		return nil, err
	}
	defer cardRows.Close()

	// Index columns for fast lookup
	colMap := make(map[string]*Column, len(board.Columns))
	for _, c := range board.Columns {
		colMap[c.ID] = c
	}

	for cardRows.Next() {
		card := &Card{}
		var labelsJSON string
		if err := cardRows.Scan(&card.ID, &card.ColumnID, &card.Title,
			&card.Description, &card.Priority, &card.GoalType, &card.Assignee,
			&labelsJSON, &card.Position,
			&card.CreatedAt, &card.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(labelsJSON), &card.Labels); err != nil {
			card.Labels = []string{}
		}
		if col, ok := colMap[card.ColumnID]; ok {
			col.Cards = append(col.Cards, card)
		}
	}

	return board, nil
}

// ─── Board ────────────────────────────────────────────────────────────────────

// GetBoardID returns the ID of the first (and only) board.
func (s *SQLiteStore) GetBoardID() (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM boards LIMIT 1`).Scan(&id)
	return id, err
}

// ─── Cards ────────────────────────────────────────────────────────────────────

// GetCard fetches a single card by ID.
func (s *SQLiteStore) GetCard(id string) (*Card, error) {
	card := &Card{}
	var labelsJSON string
	err := s.db.QueryRow(`
		SELECT id, column_id, title, description, priority, goal_type, assignee,
		       labels, position, created_at, updated_at
		FROM cards WHERE id=?`, id).Scan(
		&card.ID, &card.ColumnID, &card.Title, &card.Description,
		&card.Priority, &card.GoalType, &card.Assignee, &labelsJSON,
		&card.Position, &card.CreatedAt, &card.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("card not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(labelsJSON), &card.Labels); err != nil {
		card.Labels = []string{}
	}
	return card, nil
}

// ListCards returns all cards in a column (ordered by position).
func (s *SQLiteStore) ListCards(columnID string) ([]*Card, error) {
	rows, err := s.db.Query(`
		SELECT id, column_id, title, description, priority, goal_type, assignee,
		       labels, position, created_at, updated_at
		FROM cards WHERE column_id=? ORDER BY position`, columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []*Card
	for rows.Next() {
		card := &Card{}
		var labelsJSON string
		if err := rows.Scan(&card.ID, &card.ColumnID, &card.Title,
			&card.Description, &card.Priority, &card.GoalType, &card.Assignee,
			&labelsJSON, &card.Position,
			&card.CreatedAt, &card.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(labelsJSON), &card.Labels); err != nil {
			card.Labels = []string{}
		}
		cards = append(cards, card)
	}
	return cards, nil
}

// CreateCard inserts a new card into a column.
func (s *SQLiteStore) CreateCard(req *AddCardRequest) (*Card, error) {
	if req.Priority == "" || !req.Priority.Valid() {
		req.Priority = PriorityMedium
	}
	if !req.GoalType.Valid() {
		req.GoalType = GoalNone
	}
	if req.Labels == nil {
		req.Labels = []string{}
	}

	// Determine next position
	var maxPos int
	s.db.QueryRow(`SELECT COALESCE(MAX(position)+1,0) FROM cards WHERE column_id=?`,
		req.ColumnID).Scan(&maxPos)

	now := time.Now()
	card := &Card{
		ID:          generateID(),
		ColumnID:    req.ColumnID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		GoalType:    req.GoalType,
		Assignee:    req.Assignee,
		Labels:      req.Labels,
		Position:    maxPos,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	lblJSON, _ := json.Marshal(card.Labels)
	_, err := s.db.Exec(`
		INSERT INTO cards(id,column_id,title,description,priority,goal_type,assignee,
		                  labels,position,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		card.ID, card.ColumnID, card.Title, card.Description,
		string(card.Priority), string(card.GoalType), card.Assignee, string(lblJSON),
		card.Position, card.CreatedAt, card.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := s.refreshCache(); err != nil {
		log.Printf("[DB] cache refresh error: %v", err)
	}
	return card, nil
}

// UpdateCard modifies an existing card's fields.
func (s *SQLiteStore) UpdateCard(id string, req *UpdateCardRequest) (*Card, error) {
	if req.Priority != "" && !req.Priority.Valid() {
		req.Priority = PriorityMedium
	}
	if req.Labels == nil {
		req.Labels = []string{}
	}

	now := time.Now()
	lblJSON, _ := json.Marshal(req.Labels)

	res, err := s.db.Exec(`
		UPDATE cards
		SET title=?, description=?, priority=?, goal_type=?, assignee=?, labels=?, updated_at=?
		WHERE id=?`,
		req.Title, req.Description, string(req.Priority), string(req.GoalType),
		req.Assignee, string(lblJSON), now, id,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("card not found: %s", id)
	}

	if err := s.refreshCache(); err != nil {
		log.Printf("[DB] cache refresh error: %v", err)
	}
	return s.GetCard(id)
}

// DeleteCard removes a card permanently.
func (s *SQLiteStore) DeleteCard(id string) error {
	res, err := s.db.Exec(`DELETE FROM cards WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("card not found: %s", id)
	}
	return s.refreshCache()
}

// MoveCard moves a card to another column at a specific index,
// then re-sequences positions for both affected columns.
func (s *SQLiteStore) MoveCard(req *MoveCardRequest) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Verify card exists
	var colID string
	if err := tx.QueryRow(`SELECT column_id FROM cards WHERE id=?`, req.CardID).Scan(&colID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("card not found: %s", req.CardID)
		}
		return err
	}

	// Move card to target column
	now := time.Now()
	if _, err := tx.Exec(`UPDATE cards SET column_id=?, updated_at=? WHERE id=?`,
		req.ToColumnID, now, req.CardID); err != nil {
		return err
	}

	// Re-sequence source column
	if err := resequence(tx, req.FromColumnID, req.CardID); err != nil {
		return err
	}

	// Re-sequence target column, inserting our card at the desired index
	if err := insertAtIndex(tx, req.ToColumnID, req.CardID, req.ToIndex); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return s.refreshCache()
}

// resequence renumbers positions for a column, skipping excludeID.
func resequence(tx *sql.Tx, columnID, excludeID string) error {
	rows, err := tx.Query(
		`SELECT id FROM cards WHERE column_id=? AND id!=? ORDER BY position`, columnID, excludeID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE cards SET position=? WHERE id=?`, i, id); err != nil {
			return err
		}
	}
	return nil
}

// insertAtIndex places cardID at the given index within columnID.
func insertAtIndex(tx *sql.Tx, columnID, cardID string, idx int) error {
	rows, err := tx.Query(
		`SELECT id FROM cards WHERE column_id=? AND id!=? ORDER BY position`, columnID, cardID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}

	// Clamp index
	if idx < 0 || idx > len(ids) {
		idx = len(ids)
	}

	// Insert the moved card at idx
	all := make([]string, 0, len(ids)+1)
	all = append(all, ids[:idx]...)
	all = append(all, cardID)
	all = append(all, ids[idx:]...)

	for i, id := range all {
		if _, err := tx.Exec(`UPDATE cards SET position=? WHERE id=?`, i, id); err != nil {
			return err
		}
	}
	return nil
}

// ─── Columns ─────────────────────────────────────────────────────────────────

// GetColumn fetches a single column with its cards.
func (s *SQLiteStore) GetColumn(id string) (*Column, error) {
	col := &Column{}
	err := s.db.QueryRow(
		`SELECT id, board_id, title, color, position, created_at FROM columns WHERE id=?`, id,
	).Scan(&col.ID, &col.BoardID, &col.Title, &col.Color, &col.Position, &col.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("column not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	cards, err := s.ListCards(col.ID)
	if err != nil {
		return nil, err
	}
	col.Cards = cards
	return col, nil
}

// ListColumns returns all columns for the board (without card details).
func (s *SQLiteStore) ListColumns() ([]*Column, error) {
	boardID, err := s.GetBoardID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, board_id, title, color, position, created_at
		 FROM columns WHERE board_id=? ORDER BY position`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []*Column
	for rows.Next() {
		col := &Column{Cards: []*Card{}}
		if err := rows.Scan(&col.ID, &col.BoardID, &col.Title, &col.Color,
			&col.Position, &col.CreatedAt); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// CreateColumn appends a new column to the board.
func (s *SQLiteStore) CreateColumn(req *AddColumnRequest) (*Column, error) {
	if req.Color == "" {
		req.Color = "#6366f1"
	}
	boardID, err := s.GetBoardID()
	if err != nil {
		return nil, err
	}

	var maxPos int
	s.db.QueryRow(`SELECT COALESCE(MAX(position)+1,0) FROM columns WHERE board_id=?`, boardID).Scan(&maxPos)

	now := time.Now()
	col := &Column{
		ID:        generateID(),
		BoardID:   boardID,
		Title:     req.Title,
		Color:     req.Color,
		Position:  maxPos,
		Cards:     []*Card{},
		CreatedAt: now,
	}

	_, err = s.db.Exec(
		`INSERT INTO columns(id,board_id,title,color,position,created_at) VALUES(?,?,?,?,?,?)`,
		col.ID, col.BoardID, col.Title, col.Color, col.Position, col.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := s.refreshCache(); err != nil {
		log.Printf("[DB] cache refresh error: %v", err)
	}
	return col, nil
}

// UpdateColumn modifies a column's title and/or color.
func (s *SQLiteStore) UpdateColumn(id string, req *UpdateColumnRequest) (*Column, error) {
	// Build dynamic UPDATE to avoid overwriting unchanged fields
	parts := []string{}
	args := []interface{}{}
	if req.Title != "" {
		parts = append(parts, "title=?")
		args = append(args, req.Title)
	}
	if req.Color != "" {
		parts = append(parts, "color=?")
		args = append(args, req.Color)
	}
	if len(parts) == 0 {
		return s.GetColumn(id)
	}
	args = append(args, id)

	res, err := s.db.Exec(
		`UPDATE columns SET `+strings.Join(parts, ",")+` WHERE id=?`, args...,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("column not found: %s", id)
	}

	if err := s.refreshCache(); err != nil {
		log.Printf("[DB] cache refresh error: %v", err)
	}
	return s.GetColumn(id)
}

// DeleteColumn removes a column and all its cards (ON DELETE CASCADE).
func (s *SQLiteStore) DeleteColumn(id string) error {
	res, err := s.db.Exec(`DELETE FROM columns WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("column not found: %s", id)
	}
	return s.refreshCache()
}
