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

// RunMCPServer opens the SQLite database and starts an MCP stdio server.
func RunMCPServer(dbPath string) {
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] failed to open database %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.EnsureDefaultBoard(); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] failed to seed database: %v\n", err)
		os.Exit(1)
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

func registerTools(s *server.MCPServer, store *SQLiteStore) {

	// ── get_board ─────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("get_board",
			mcp.WithDescription("取得完整看板狀態，包含所有欄位（columns）與其下的任務卡（cards）。"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(store.GetBoard())
		},
	)

	// ── list_columns ──────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("list_columns",
			mcp.WithDescription("列出所有欄位（不含任務卡內容）。"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cols, err := store.ListColumns()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(cols)
		},
	)

	// ── create_column ─────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("create_column",
			mcp.WithDescription("新增一個欄位到看板。"),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("欄位名稱，例如：待辦、進行中、已完成")),
			mcp.WithString("color",
				mcp.Description("欄位標題顏色（十六進位），預設 #6366f1")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			color := req.GetString("color", "")
			col, err := store.CreateColumn(&AddColumnRequest{Title: title, Color: color})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(col)
		},
	)

	// ── update_column ─────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("update_column",
			mcp.WithDescription("更新欄位的名稱或顏色。"),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("欄位 ID")),
			mcp.WithString("title", mcp.Description("新的欄位名稱")),
			mcp.WithString("color", mcp.Description("新的顏色（十六進位）")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			colID := req.GetString("column_id", "")
			title := req.GetString("title", "")
			color := req.GetString("color", "")
			col, err := store.UpdateColumn(colID, &UpdateColumnRequest{Title: title, Color: color})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(col)
		},
	)

	// ── delete_column ─────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("delete_column",
			mcp.WithDescription("刪除欄位及其所有任務卡（不可復原）。"),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("要刪除的欄位 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			colID := req.GetString("column_id", "")
			if err := store.DeleteColumn(colID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("欄位已刪除"), nil
		},
	)

	// ── get_card ──────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("get_card",
			mcp.WithDescription("取得單一任務卡的完整資訊。"),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("任務卡 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cardID := req.GetString("card_id", "")
			card, err := store.GetCard(cardID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(card)
		},
	)

	// ── list_cards ────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("list_cards",
			mcp.WithDescription("列出指定欄位內的所有任務卡（依 position 排序）。"),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("欄位 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			colID := req.GetString("column_id", "")
			cards, err := store.ListCards(colID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(cards)
		},
	)

	// ── create_card ───────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("create_card",
			mcp.WithDescription("在指定欄位新增一張任務卡。"),
			mcp.WithString("column_id", mcp.Required(), mcp.Description("目標欄位 ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("任務標題")),
			mcp.WithString("description", mcp.Description("詳細描述")),
			mcp.WithString("priority", mcp.Description("優先級：high | medium | low，預設 medium")),
			mcp.WithString("assignee", mcp.Description("負責人姓名")),
			mcp.WithString("labels", mcp.Description("標籤（逗號分隔），例如：前端,Bug,緊急")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			card, err := store.CreateCard(&AddCardRequest{
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
			mcp.WithDescription("更新任務卡的標題、描述、優先級、負責人或標籤。"),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("任務卡 ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("新的任務標題")),
			mcp.WithString("description", mcp.Description("新的描述")),
			mcp.WithString("priority", mcp.Description("優先級：high | medium | low")),
			mcp.WithString("assignee", mcp.Description("負責人姓名")),
			mcp.WithString("labels", mcp.Description("新標籤（逗號分隔），留空則清除")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title := req.GetString("title", "")
			if strings.TrimSpace(title) == "" {
				return mcp.NewToolResultError("title is required"), nil
			}
			card, err := store.UpdateCard(req.GetString("card_id", ""), &UpdateCardRequest{
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
			mcp.WithDescription("永久刪除一張任務卡（不可復原）。"),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("任務卡 ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := store.DeleteCard(req.GetString("card_id", "")); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("任務卡已刪除"), nil
		},
	)

	// ── move_card ─────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("move_card",
			mcp.WithDescription("將任務卡移動到另一個欄位，可指定插入位置。"),
			mcp.WithString("card_id", mcp.Required(), mcp.Description("要移動的任務卡 ID")),
			mcp.WithString("from_column_id", mcp.Required(), mcp.Description("來源欄位 ID")),
			mcp.WithString("to_column_id", mcp.Required(), mcp.Description("目標欄位 ID")),
			mcp.WithNumber("to_index", mcp.Description("插入位置索引（0=最上方，-1=最下方，預設-1）")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			toIndex := req.GetInt("to_index", -1)
			err := store.MoveCard(&MoveCardRequest{
				CardID:       req.GetString("card_id", ""),
				FromColumnID: req.GetString("from_column_id", ""),
				ToColumnID:   req.GetString("to_column_id", ""),
				ToIndex:      toIndex,
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
			mcp.WithDescription("在整個看板中以關鍵字搜尋任務卡（標題或描述，不分大小寫）。"),
			mcp.WithString("query", mcp.Required(), mcp.Description("搜尋關鍵字")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query := strings.ToLower(strings.TrimSpace(req.GetString("query", "")))
			if query == "" {
				return mcp.NewToolResultError("query is required"), nil
			}
			type hit struct {
				ColumnTitle string `json:"columnTitle"`
				ColumnID    string `json:"columnId"`
				Card        *Card  `json:"card"`
			}
			var results []hit
			for _, col := range store.GetBoard().Columns {
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

	// ── get_stats ─────────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("get_stats",
			mcp.WithDescription("取得看板統計摘要：欄位數、任務總數、各欄位的優先級分佈。"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			board := store.GetBoard()
			type colStat struct {
				ColumnID    string         `json:"columnId"`
				ColumnTitle string         `json:"columnTitle"`
				TotalCards  int            `json:"totalCards"`
				ByPriority  map[string]int `json:"byPriority"`
			}
			type stats struct {
				TotalColumns int       `json:"totalColumns"`
				TotalCards   int       `json:"totalCards"`
				Columns      []colStat `json:"columns"`
			}
			st := stats{}
			for _, col := range board.Columns {
				cs := colStat{
					ColumnID:    col.ID,
					ColumnTitle: col.Title,
					TotalCards:  len(col.Cards),
					ByPriority:  map[string]int{"high": 0, "medium": 0, "low": 0},
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
