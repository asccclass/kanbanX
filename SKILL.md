# KanbanX MCP Skill

## Overview

KanbanX is a real-time, multi-user Kanban board backed by SQLite. This skill exposes the full board via MCP tools so Claude can read, create, update, delete, move, and search tasks and columns on behalf of the user.

## Setup

### 1. Build the binary

```bash
cd kanban
CGO_ENABLED=0 go build -mod=vendor -o kanban .
```

### 2. Configure the MCP client (Claude Desktop / OpenClaw)

Add the following block to your MCP client configuration (e.g. `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "kanbanx": {
      "command": "/absolute/path/to/kanban",
      "args": ["--mcp"],
      "env": {
        "DBPath": "/absolute/path/to/kanban.db"
      }
    }
  }
}
```

> **Tip:** The same `kanban.db` file is shared between the HTTP web UI and the MCP server, so changes made through Claude are immediately visible in the browser and vice-versa.

### 3. Start the HTTP UI (optional but recommended)

```bash
./kanban          # starts web UI on http://localhost:8080
./kanban --mcp    # starts MCP stdio server
```

Both modes read from the same SQLite database.

---

## Available Tools

### Board

| Tool | Description |
|------|-------------|
| `get_board` | Return the full board: all columns and their cards |
| `get_stats` | Summary statistics: column count, card count, priority breakdown per column |

### Columns

| Tool | Required args | Optional args | Description |
|------|--------------|---------------|-------------|
| `list_columns` | — | — | List all columns (without card content) |
| `create_column` | `title` | `color` | Add a new column |
| `update_column` | `column_id` | `title`, `color` | Rename or recolour a column |
| `delete_column` | `column_id` | — | Delete column and ALL its cards |

### Cards

| Tool | Required args | Optional args | Description |
|------|--------------|---------------|-------------|
| `list_cards` | `column_id` | — | List all cards in a column |
| `get_card` | `card_id` | — | Get full details of a single card |
| `create_card` | `column_id`, `title` | `description`, `priority`, `assignee`, `labels` | Create a new card |
| `update_card` | `card_id`, `title` | `description`, `priority`, `assignee`, `labels` | Update a card |
| `delete_card` | `card_id` | — | Permanently delete a card |
| `move_card` | `card_id`, `from_column_id`, `to_column_id` | `to_index` | Move card to another column |
| `search_cards` | `query` | — | Full-text search across title and description |

---

## Argument Reference

### `priority`
Accepted values: `high` · `medium` · `low`  
Default: `medium`

### `color`
Any CSS hex colour string, e.g. `#6366f1`, `#10b981`, `#f59e0b`  
Default: `#6366f1`

### `labels`
Comma-separated string, e.g. `"前端,Bug,緊急"`  
Pass an empty string `""` to clear all labels.

### `to_index`
Integer. `0` = insert at top, `-1` = append at bottom.  
Default: `-1`

---

## Example Workflows

### View the board
```
Use the get_board tool to show me the current kanban board.
```

### Create a card
```
Create a high-priority card titled "Fix login bug" in the "進行中" column,
assigned to Alice, with labels "後端,Bug".
```
→ Claude will call `list_columns` to find the column ID, then `create_card`.

### Move a card to done
```
Move the card "Fix login bug" to the "已完成" column.
```
→ Claude calls `search_cards` to find the card, then `move_card`.

### Weekly standup summary
```
Give me a standup summary: what's in progress, what's blocked (high priority in 待辦事項),
and what was completed this week?
```
→ Claude calls `get_board` and summarises the data.

### Bulk create from a list
```
Add the following tasks to the "待辦事項" column, all medium priority, assigned to Bob:
- Write unit tests for auth module
- Update API documentation
- Set up staging environment
```
→ Claude calls `create_card` three times sequentially.

### Board health check
```
Use get_stats to show me which columns are most backlogged.
```

---

## Data Model

```
Board
└── Column[]  (id, title, color, position)
    └── Card[]  (id, columnId, title, description,
                 priority, assignee, labels[], position,
                 createdAt, updatedAt)
```

---

## Notes for Claude

- Always call `get_board` or `list_columns` first to resolve human-readable column names to IDs before calling column- or card-specific tools.
- When the user says "move to done" or similar, use `search_cards` to locate the card if no ID is available.
- `delete_column` cascades — it will remove ALL cards inside. Confirm with the user before calling it.
- `labels` is always a comma-separated string in tool arguments; the server returns it as a JSON array.
- IDs are opaque numeric strings; never guess or fabricate them — always look them up first.
