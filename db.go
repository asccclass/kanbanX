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

// SQLiteStore is the persistence layer.
// Board data is keyed by Telegram user ID; each Telegram user owns exactly one board.
// A read-through cache (boardCache) avoids DB round-trips on every WebSocket broadcast.
type SQLiteStore struct {
	db         *sql.DB
	mu         sync.RWMutex          // guards boardCache
	boardCache map[string]*Board     // telegramID → *Board
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db, boardCache: make(map[string]*Board)}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

// ─── Schema ───────────────────────────────────────────────────────────────────

const schema = `
-- Each Telegram user owns exactly one board.
CREATE TABLE IF NOT EXISTS boards (
    id          TEXT PRIMARY KEY,
    telegram_id TEXT NOT NULL UNIQUE,  -- Telegram numeric user ID (stored as text)
    title       TEXT NOT NULL DEFAULT '我的看板',
    created_at  DATETIME NOT NULL
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
    assignee    TEXT NOT NULL DEFAULT '',
    labels      TEXT NOT NULL DEFAULT '[]',
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_boards_telegram ON boards(telegram_id);
CREATE INDEX IF NOT EXISTS idx_columns_board   ON columns(board_id, position);
CREATE INDEX IF NOT EXISTS idx_cards_column    ON cards(column_id, position);
`

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

// ─── User Board Management ────────────────────────────────────────────────────

// EnsureUserBoard returns the board for a Telegram user, creating it (with default
// columns) if this is their first time.
func (s *SQLiteStore) EnsureUserBoard(telegramID string) (*Board, error) {
	telegramID = strings.TrimSpace(telegramID)
	if telegramID == "" {
		return nil, fmt.Errorf("telegram_id is required")
	}

	// Check cache first
	s.mu.RLock()
	if b, ok := s.boardCache[telegramID]; ok {
		s.mu.RUnlock()
		return b, nil
	}
	s.mu.RUnlock()

	// Check DB
	var boardID string
	err := s.db.QueryRow(`SELECT id FROM boards WHERE telegram_id=?`, telegramID).Scan(&boardID)
	if err == sql.ErrNoRows {
		// First time — seed a fresh board for this user
		if err2 := s.seedUserBoard(telegramID); err2 != nil {
			return nil, fmt.Errorf("seed board for %s: %w", telegramID, err2)
		}
	} else if err != nil {
		return nil, err
	}

	return s.loadAndCacheBoard(telegramID)
}

// seedUserBoard creates a default board + columns for a brand-new Telegram user.
func (s *SQLiteStore) seedUserBoard(telegramID string) error {
	now := time.Now()
	boardID := generateID()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO boards(id, telegram_id, title, created_at) VALUES(?,?,?,?)`,
		boardID, telegramID, "我的看板", now,
	)
	if err != nil {
		return err
	}

	defaultCols := []struct{ title, color string }{
		{"待辦事項", "#6366f1"},
		{"進行中",   "#f59e0b"},
		{"審查中",   "#8b5cf6"},
		{"已完成",   "#10b981"},
	}
	for i, col := range defaultCols {
		_, err = tx.Exec(
			`INSERT INTO columns(id, board_id, title, color, position, created_at) VALUES(?,?,?,?,?,?)`,
			generateID(), boardID, col.title, col.color, i, now,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// loadAndCacheBoard reads the full board for a Telegram user from DB and caches it.
func (s *SQLiteStore) loadAndCacheBoard(telegramID string) (*Board, error) {
	board, err := s.loadBoardByTelegramID(telegramID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.boardCache[telegramID] = board
	s.mu.Unlock()
	return board, nil
}

// refreshUserCache reloads the board for telegramID from DB and updates the cache.
func (s *SQLiteStore) refreshUserCache(telegramID string) error {
	_, err := s.loadAndCacheBoard(telegramID)
	return err
}

// GetCachedBoard returns the cached board for a user (may be nil if not yet loaded).
func (s *SQLiteStore) GetCachedBoard(telegramID string) *Board {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.boardCache[telegramID]
}

// GetBoard is the legacy single-board accessor used by the HTTP layer.
// It returns the first board found (used only by web UI admin view).
func (s *SQLiteStore) GetBoard() *Board {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, b := range s.boardCache {
		return b
	}
	return &Board{Title: "看板", Columns: []*Column{}}
}

// ListAllUsers returns all Telegram users who have a board.
func (s *SQLiteStore) ListAllUsers() ([]*TelegramUser, error) {
	rows, err := s.db.Query(
		`SELECT telegram_id, title, created_at FROM boards ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*TelegramUser
	for rows.Next() {
		u := &TelegramUser{}
		if err := rows.Scan(&u.TelegramID, &u.DisplayName, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []*TelegramUser{}
	}
	return users, nil
}

// UpdateBoardTitle renames the board for a Telegram user.
func (s *SQLiteStore) UpdateBoardTitle(telegramID, title string) error {
	_, err := s.db.Exec(`UPDATE boards SET title=? WHERE telegram_id=?`, title, telegramID)
	if err != nil {
		return err
	}
	return s.refreshUserCache(telegramID)
}

// ─── Private DB load ──────────────────────────────────────────────────────────

func (s *SQLiteStore) loadBoardByTelegramID(telegramID string) (*Board, error) {
	board := &Board{}
	err := s.db.QueryRow(
		`SELECT id, telegram_id, title, created_at FROM boards WHERE telegram_id=?`,
		telegramID,
	).Scan(&board.ID, &board.TelegramID, &board.Title, &board.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("board not found for telegram_id %s: %w", telegramID, err)
	}

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

	// Load all cards for this board in one query
	cardRows, err := s.db.Query(`
		SELECT id, column_id, title, description, priority, assignee,
		       labels, position, created_at, updated_at
		FROM cards
		WHERE column_id IN (SELECT id FROM columns WHERE board_id=?)
		ORDER BY column_id, position`, board.ID)
	if err != nil {
		return nil, err
	}
	defer cardRows.Close()

	colMap := make(map[string]*Column, len(board.Columns))
	for _, c := range board.Columns {
		colMap[c.ID] = c
	}

	for cardRows.Next() {
		card := &Card{}
		var labelsJSON string
		if err := cardRows.Scan(&card.ID, &card.ColumnID, &card.Title,
			&card.Description, &card.Priority, &card.Assignee,
			&labelsJSON, &card.Position, &card.CreatedAt, &card.UpdatedAt); err != nil {
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

// getBoardIDForTelegramUser resolves board ID — used internally before card/column ops.
func (s *SQLiteStore) getBoardIDForTelegramUser(telegramID string) (string, error) {
	board, err := s.EnsureUserBoard(telegramID)
	if err != nil {
		return "", err
	}
	return board.ID, nil
}

// ─── Cards ────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) GetCard(id string) (*Card, error) {
	card := &Card{}
	var labelsJSON string
	err := s.db.QueryRow(`
		SELECT id, column_id, title, description, priority, assignee,
		       labels, position, created_at, updated_at
		FROM cards WHERE id=?`, id).Scan(
		&card.ID, &card.ColumnID, &card.Title, &card.Description,
		&card.Priority, &card.Assignee, &labelsJSON,
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

func (s *SQLiteStore) ListCards(columnID string) ([]*Card, error) {
	rows, err := s.db.Query(`
		SELECT id, column_id, title, description, priority, assignee,
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
		if err := rows.Scan(&card.ID, &card.ColumnID, &card.Title, &card.Description,
			&card.Priority, &card.Assignee, &labelsJSON, &card.Position,
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

func (s *SQLiteStore) CreateCard(telegramID string, req *AddCardRequest) (*Card, error) {
	if req.Priority == "" || !req.Priority.Valid() {
		req.Priority = PriorityMedium
	}
	if req.Labels == nil {
		req.Labels = []string{}
	}

	// Verify the column belongs to this user's board
	if err := s.assertColumnOwnership(telegramID, req.ColumnID); err != nil {
		return nil, err
	}

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
		Assignee:    req.Assignee,
		Labels:      req.Labels,
		Position:    maxPos,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	lblJSON, _ := json.Marshal(card.Labels)
	_, err := s.db.Exec(`
		INSERT INTO cards(id,column_id,title,description,priority,assignee,
		                  labels,position,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		card.ID, card.ColumnID, card.Title, card.Description,
		string(card.Priority), card.Assignee, string(lblJSON),
		card.Position, card.CreatedAt, card.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := s.refreshUserCache(telegramID); err != nil {
		log.Printf("[DB] cache refresh: %v", err)
	}
	return card, nil
}

func (s *SQLiteStore) UpdateCard(telegramID, id string, req *UpdateCardRequest) (*Card, error) {
	if err := s.assertCardOwnership(telegramID, id); err != nil {
		return nil, err
	}
	if req.Priority != "" && !req.Priority.Valid() {
		req.Priority = PriorityMedium
	}
	if req.Labels == nil {
		req.Labels = []string{}
	}
	now := time.Now()
	lblJSON, _ := json.Marshal(req.Labels)
	res, err := s.db.Exec(`
		UPDATE cards SET title=?, description=?, priority=?, assignee=?, labels=?, updated_at=?
		WHERE id=?`,
		req.Title, req.Description, string(req.Priority),
		req.Assignee, string(lblJSON), now, id,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("card not found: %s", id)
	}
	if err := s.refreshUserCache(telegramID); err != nil {
		log.Printf("[DB] cache refresh: %v", err)
	}
	return s.GetCard(id)
}

func (s *SQLiteStore) DeleteCard(telegramID, id string) error {
	if err := s.assertCardOwnership(telegramID, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM cards WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("card not found: %s", id)
	}
	return s.refreshUserCache(telegramID)
}

func (s *SQLiteStore) MoveCard(telegramID string, req *MoveCardRequest) error {
	if err := s.assertCardOwnership(telegramID, req.CardID); err != nil {
		return err
	}
	if err := s.assertColumnOwnership(telegramID, req.ToColumnID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	if _, err := tx.Exec(`UPDATE cards SET column_id=?, updated_at=? WHERE id=?`,
		req.ToColumnID, now, req.CardID); err != nil {
		return err
	}
	if err := resequence(tx, req.FromColumnID, req.CardID); err != nil {
		return err
	}
	if err := insertAtIndex(tx, req.ToColumnID, req.CardID, req.ToIndex); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.refreshUserCache(telegramID)
}

// ─── Columns ──────────────────────────────────────────────────────────────────

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

func (s *SQLiteStore) ListColumns(telegramID string) ([]*Column, error) {
	boardID, err := s.getBoardIDForTelegramUser(telegramID)
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
	if cols == nil {
		cols = []*Column{}
	}
	return cols, nil
}

func (s *SQLiteStore) CreateColumn(telegramID string, req *AddColumnRequest) (*Column, error) {
	if req.Color == "" {
		req.Color = "#6366f1"
	}
	boardID, err := s.getBoardIDForTelegramUser(telegramID)
	if err != nil {
		return nil, err
	}
	var maxPos int
	s.db.QueryRow(`SELECT COALESCE(MAX(position)+1,0) FROM columns WHERE board_id=?`, boardID).Scan(&maxPos)
	now := time.Now()
	col := &Column{
		ID: generateID(), BoardID: boardID, Title: req.Title,
		Color: req.Color, Position: maxPos, Cards: []*Card{}, CreatedAt: now,
	}
	_, err = s.db.Exec(
		`INSERT INTO columns(id,board_id,title,color,position,created_at) VALUES(?,?,?,?,?,?)`,
		col.ID, col.BoardID, col.Title, col.Color, col.Position, col.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := s.refreshUserCache(telegramID); err != nil {
		log.Printf("[DB] cache refresh: %v", err)
	}
	return col, nil
}

func (s *SQLiteStore) UpdateColumn(telegramID, id string, req *UpdateColumnRequest) (*Column, error) {
	if err := s.assertColumnOwnership(telegramID, id); err != nil {
		return nil, err
	}
	parts, args := []string{}, []interface{}{}
	if req.Title != "" {
		parts = append(parts, "title=?"); args = append(args, req.Title)
	}
	if req.Color != "" {
		parts = append(parts, "color=?"); args = append(args, req.Color)
	}
	if len(parts) == 0 {
		return s.GetColumn(id)
	}
	args = append(args, id)
	res, err := s.db.Exec(`UPDATE columns SET `+strings.Join(parts, ",")+` WHERE id=?`, args...)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("column not found: %s", id)
	}
	if err := s.refreshUserCache(telegramID); err != nil {
		log.Printf("[DB] cache refresh: %v", err)
	}
	return s.GetColumn(id)
}

func (s *SQLiteStore) DeleteColumn(telegramID, id string) error {
	if err := s.assertColumnOwnership(telegramID, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM columns WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("column not found: %s", id)
	}
	return s.refreshUserCache(telegramID)
}

// ─── Ownership Guards ─────────────────────────────────────────────────────────
// Prevent a user from touching another user's data.

func (s *SQLiteStore) assertColumnOwnership(telegramID, columnID string) error {
	boardID, err := s.getBoardIDForTelegramUser(telegramID)
	if err != nil {
		return err
	}
	var owner string
	err = s.db.QueryRow(`SELECT board_id FROM columns WHERE id=?`, columnID).Scan(&owner)
	if err == sql.ErrNoRows {
		return fmt.Errorf("column not found: %s", columnID)
	}
	if err != nil {
		return err
	}
	if owner != boardID {
		return fmt.Errorf("column %s does not belong to your board", columnID)
	}
	return nil
}

func (s *SQLiteStore) assertCardOwnership(telegramID, cardID string) error {
	boardID, err := s.getBoardIDForTelegramUser(telegramID)
	if err != nil {
		return err
	}
	var colBoardID string
	err = s.db.QueryRow(`
		SELECT c.board_id FROM columns c
		JOIN cards k ON k.column_id = c.id
		WHERE k.id=?`, cardID).Scan(&colBoardID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("card not found: %s", cardID)
	}
	if err != nil {
		return err
	}
	if colBoardID != boardID {
		return fmt.Errorf("card %s does not belong to your board", cardID)
	}
	return nil
}

// ─── Position helpers ─────────────────────────────────────────────────────────

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
	if idx < 0 || idx > len(ids) {
		idx = len(ids)
	}
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

// ─── Legacy / HTTP helpers ────────────────────────────────────────────────────
// These wrappers let the existing HTTP handlers work with the first available user's board.

func (s *SQLiteStore) EnsureDefaultBoard() error {
	// For the web UI: load all existing boards into cache
	rows, err := s.db.Query(`SELECT telegram_id FROM boards`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			return err
		}
		if _, err := s.loadAndCacheBoard(tid); err != nil {
			log.Printf("[DB] preload board %s: %v", tid, err)
		}
	}
	return nil
}

// GetBoardID returns the board ID for a telegram user (auto-creating if needed).
func (s *SQLiteStore) GetBoardID(telegramID string) (string, error) {
	return s.getBoardIDForTelegramUser(telegramID)
}
