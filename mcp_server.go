package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RunMCPServer starts a stdio MCP server backed by the given SQLite database.
func RunMCPServer(dbPath string) {
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Pre-load any existing user boards into cache
	if err := store.EnsureDefaultBoard(); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] preload warning: %v\n", err)
	}

	s := server.NewMCPServer(
		"KanbanX",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	registerTools(s, store)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] server error: %v\n", err)
		os.Exit(1)
	}
}

// ─── Tool Registration ────────────────────────────────────────────────────────
//
// IMPORTANT: Every tool requires a `telegram_id` parameter.
// Claude must always pass the Telegram numeric user ID of the person
// it is currently talking to. The server uses this ID to:
//   1. Auto-create a personal board on first use.
//   2. Scope all reads and writes to that user's data only.
//   3. Prevent one user from accessing another user's data.

const telegramIDDesc = "Telegram 用戶的數字 ID。若設為個人使用模式，此欄位可省略，將使用預設的個人 ID。"

func getUserID(req mcp.CallToolRequest) (string, error) {
	tid := req.GetString("telegram_id", "")
	if tid != "" {
		return tid, nil
	}
	defaultID := os.Getenv("DEFAULT_USER_ID")
	if defaultID != "" {
		return defaultID, nil
	}
	return "", fmt.Errorf("telegram_id is required or DEFAULT_USER_ID must be set")
}

func registerTools(s *server.MCPServer, store *SQLiteStore) {

	// ── get_my_board ──────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("get_my_board",
			mcp.WithDescription("取得用戶的個人看板（含所有欄位與任務卡）。首次呼叫時會自動建立預設看板。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			board, err := store.EnsureUserBoard(tid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(board)
		},
	)

	// ── get_my_stats ──────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("get_my_stats",
			mcp.WithDescription("取得用戶看板的統計摘要：欄位數、任務總數、各欄位優先級分佈。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			board, err := store.EnsureUserBoard(tid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			type colStat struct {
				ColumnID    string         `json:"columnId"`
				ColumnTitle string         `json:"columnTitle"`
				TotalCards  int            `json:"totalCards"`
				ByPriority  map[string]int `json:"byPriority"`
			}
			type stats struct {
				TelegramID   string    `json:"telegramId"`
				BoardTitle   string    `json:"boardTitle"`
				TotalColumns int       `json:"totalColumns"`
				TotalCards   int       `json:"totalCards"`
				Columns      []colStat `json:"columns"`
			}
			st := stats{TelegramID: tid, BoardTitle: board.Title}
			for _, col := range board.Columns {
				cs := colStat{
					ColumnID: col.ID, ColumnTitle: col.Title,
					TotalCards: len(col.Cards),
					ByPriority: map[string]int{"high": 0, "medium": 0, "low": 0},
				}
				for _, c := range col.Cards {
					cs.ByPriority[string(c.Priority)]++
					st.TotalCards++
				}
				st.Columns = append(st.Columns, cs)
				st.TotalColumns++
			}
			return jsonResult(st)
		},
	)

	// ── rename_my_board ───────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("rename_my_board",
			mcp.WithDescription("重新命名用戶的個人看板標題。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("title", mcp.Required(), mcp.Description("新的看板標題")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			if err := store.UpdateBoardTitle(tid, title); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("看板已重新命名為「%s」", title)), nil
		},
	)

	// ── list_columns ──────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("list_columns",
			mcp.WithDescription("列出用戶看板的所有欄位（不含任務卡詳情）。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			cols, err := store.ListColumns(tid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(cols)
		},
	)

	// ── create_column ─────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("create_column",
			mcp.WithDescription("在用戶的看板新增一個欄位。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("title", mcp.Required(), mcp.Description("欄位名稱")),
			mcp.WithString("color", mcp.Description("欄位顏色（十六進位），預設 #6366f1")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			col, err := store.CreateColumn(tid, &AddColumnRequest{
				Title: title, Color: req.GetString("color", ""),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(col)
		},
	)

	// ── update_column ─────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("update_column",
			mcp.WithDescription("更新用戶看板中某欄位的名稱或顏色。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("欄位 ID")),
			mcp.WithString("title", mcp.Description("新的欄位名稱")),
			mcp.WithString("color", mcp.Description("新的顏色（十六進位）")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			col, err := store.UpdateColumn(tid, req.GetString("column_id", ""),
				&UpdateColumnRequest{
					Title: req.GetString("title", ""),
					Color: req.GetString("color", ""),
				})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(col)
		},
	)

	// ── delete_column ─────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("delete_column",
			mcp.WithDescription("刪除用戶看板中的某欄位及其所有任務卡（不可復原）。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("要刪除的欄位 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := store.DeleteColumn(tid, req.GetString("column_id", "")); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("欄位已刪除"), nil
		},
	)

	// ── list_cards ────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("list_cards",
			mcp.WithDescription("列出用戶某欄位內的所有任務卡（依 position 排序）。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("欄位 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			colID := req.GetString("column_id", "")
			if err := store.assertColumnOwnership(tid, colID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			cards, err := store.ListCards(colID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(cards)
		},
	)

	// ── get_card ──────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("get_card",
			mcp.WithDescription("取得用戶某張任務卡的完整資訊。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("任務卡 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			cardID := req.GetString("card_id", "")
			if err := store.assertCardOwnership(tid, cardID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			card, err := store.GetCard(cardID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(card)
		},
	)

	// ── create_card ───────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("create_card",
			mcp.WithDescription("在用戶指定欄位新增一張任務卡。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("目標欄位 ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("任務標題")),
			mcp.WithString("description", mcp.Description("任務詳細描述")),
			mcp.WithString("priority", mcp.Description("優先級：high | medium | low，預設 medium")),
			mcp.WithString("assignee", mcp.Description("負責人姓名")),
			mcp.WithString("labels", mcp.Description("標籤（逗號分隔），例如：工作,重要,今日")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			card, err := store.CreateCard(tid, &AddCardRequest{
				ColumnID:    req.GetString("column_id", ""),
				Title:       title,
				Description: req.GetString("description", ""),
				Priority:    Priority(req.GetString("priority", "")),
				Assignee:    req.GetString("assignee", ""),
				Labels:      parseLabels(req.GetString("labels", "")),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(card)
		},
	)

	// ── update_card ───────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("update_card",
			mcp.WithDescription("更新用戶某張任務卡的內容。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("任務卡 ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("新的任務標題")),
			mcp.WithString("description", mcp.Description("新的描述")),
			mcp.WithString("priority", mcp.Description("優先級：high | medium | low")),
			mcp.WithString("assignee", mcp.Description("負責人姓名")),
			mcp.WithString("labels", mcp.Description("新標籤（逗號分隔），留空則清除")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			card, err := store.UpdateCard(tid, req.GetString("card_id", ""), &UpdateCardRequest{
				Title:       title,
				Description: req.GetString("description", ""),
				Priority:    Priority(req.GetString("priority", "")),
				Assignee:    req.GetString("assignee", ""),
				Labels:      parseLabels(req.GetString("labels", "")),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(card)
		},
	)

	// ── delete_card ───────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("delete_card",
			mcp.WithDescription("永久刪除用戶的某張任務卡（不可復原）。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("任務卡 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := store.DeleteCard(tid, req.GetString("card_id", "")); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("任務卡已刪除"), nil
		},
	)

	// ── move_card ─────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("move_card",
			mcp.WithDescription("將用戶的任務卡移動到另一個欄位，可指定插入位置。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("要移動的任務卡 ID")),
			mcp.WithString("from_column_id", mcp.Required(), mcp.Description("來源欄位 ID")),
			mcp.WithString("to_column_id", mcp.Required(), mcp.Description("目標欄位 ID")),
			mcp.WithNumber("to_index", mcp.Description("插入位置（0=最上方，-1=最下方，預設-1）")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			err = store.MoveCard(tid, &MoveCardRequest{
				CardID:       req.GetString("card_id", ""),
				FromColumnID: req.GetString("from_column_id", ""),
				ToColumnID:   req.GetString("to_column_id", ""),
				ToIndex:      req.GetInt("to_index", -1),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("任務卡已移動"), nil
		},
	)

	// ── search_cards ──────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("search_cards",
			mcp.WithDescription("在用戶的看板中以關鍵字搜尋任務卡（標題或描述，不分大小寫）。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("query", mcp.Required(), mcp.Description("搜尋關鍵字")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			query := strings.ToLower(strings.TrimSpace(req.GetString("query", "")))
			if query == "" {
				return mcp.NewToolResultError("query is required"), nil
			}
			board, err := store.EnsureUserBoard(tid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			type hit struct {
				ColumnTitle string `json:"columnTitle"`
				ColumnID    string `json:"columnId"`
				Card        *Card  `json:"card"`
			}
			var results []hit
			for _, col := range board.Columns {
				for _, card := range col.Cards {
					if strings.Contains(strings.ToLower(card.Title), query) ||
						strings.Contains(strings.ToLower(card.Description), query) {
						results = append(results, hit{col.Title, col.ID, card})
					}
				}
			}
			if results == nil {
				results = []hit{}
			}
			return jsonResult(results)
		},
	)

	// ── add_quick_task ────────────────────────────────────────────────────────
	// Convenience tool: creates a card in the first column of the user's board.
	// Ideal for fast "add this to my todo list" use-cases via Telegram chat.
	s.AddTool(
		mcp.NewTool("add_quick_task",
			mcp.WithDescription("快速新增待辦事項到用戶看板的第一個欄位（無需指定欄位 ID）。適合 Telegram 快速記事使用。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("title", mcp.Required(), mcp.Description("任務標題")),
			mcp.WithString("description", mcp.Description("任務描述（選填）")),
			mcp.WithString("priority", mcp.Description("優先級：high | medium | low，預設 medium")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			board, err := store.EnsureUserBoard(tid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if len(board.Columns) == 0 {
				return mcp.NewToolResultError("no columns found — please create a column first"), nil
			}
			colID := board.Columns[0].ID
			card, err := store.CreateCard(tid, &AddCardRequest{
				ColumnID:    colID,
				Title:       title,
				Description: req.GetString("description", ""),
				Priority:    Priority(req.GetString("priority", "medium")),
				Labels:      []string{},
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result := map[string]interface{}{
				"card":        card,
				"columnTitle": board.Columns[0].Title,
				"message":     fmt.Sprintf("已新增「%s」到「%s」", card.Title, board.Columns[0].Title),
			}
			return jsonResult(result)
		},
	)

	// ── mark_done ─────────────────────────────────────────────────────────────
	// Convenience tool: moves a card to the last column (typically "已完成").
	s.AddTool(
		mcp.NewTool("mark_done",
			mcp.WithDescription("將任務卡標記為完成（移動到看板最後一個欄位）。"),
			mcp.WithString("telegram_id", mcp.Description(telegramIDDesc)),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("要完成的任務卡 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tid, err := getUserID(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			cardID := req.GetString("card_id", "")

			board, err := store.EnsureUserBoard(tid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if len(board.Columns) == 0 {
				return mcp.NewToolResultError("no columns on board"), nil
			}
			lastCol := board.Columns[len(board.Columns)-1]

			// Find card's current column
			if err := store.assertCardOwnership(tid, cardID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			card, err := store.GetCard(cardID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if card.ColumnID == lastCol.ID {
				return mcp.NewToolResultText(fmt.Sprintf("「%s」已在最後一個欄位「%s」", card.Title, lastCol.Title)), nil
			}

			err = store.MoveCard(tid, &MoveCardRequest{
				CardID:       cardID,
				FromColumnID: card.ColumnID,
				ToColumnID:   lastCol.ID,
				ToIndex:      -1,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("「%s」已移動到「%s」✓", card.Title, lastCol.Title)), nil
		},
	)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func parseLabels(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	labels := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			labels = append(labels, t)
		}
	}
	return labels
}
