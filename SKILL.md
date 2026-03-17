# KanbanX MCP Skill — 多用戶個人看板（Telegram ID 識別）

## 概覽

KanbanX 是以 SQLite 持久化的即時多用戶看板。每個 Telegram 用戶擁有**完全獨立的個人看板**，資料互相隔離。Claude 透過 MCP 工具以 Telegram 用戶 ID 來識別身份，並自動建立、存取該用戶的看板。

---

## 安裝與設定

### 1. 編譯

```bash
cd kanban
CGO_ENABLED=0 go build -mod=vendor -o kanban .
```

### 2. OpenClaw MCP 設定

在 OpenClaw 的 MCP 設定檔中加入：

```json
{
  "mcpServers": {
    "kanbanx": {
      "command": "/絕對路徑/kanban",
      "args": ["--mcp"],
      "env": {
        "DBPath": "/絕對路徑/kanban.db"
      }
    }
  }
}
```

### 3. 同步開啟網頁看板（選用）

```bash
./kanban        # 網頁 UI → http://localhost:8080?telegram_id=用戶ID
./kanban --mcp  # OpenClaw 會自動管理此程序，不需手動執行
```

---

## Claude 必讀：使用規則

> **每次呼叫任何工具，都必須傳入 `telegram_id`（Telegram 用戶的數字 ID）。**
> 這是區分不同用戶資料的唯一依據。

- **如何取得 `telegram_id`**：OpenClaw 在 Telegram 對話中會自動提供當前用戶的 Telegram 數字 ID（例如 `123456789`）。Claude 應將此 ID 傳入每一個工具呼叫。
- **首次使用**：呼叫任何工具時，若該用戶沒有看板，系統會自動建立含有 4 個預設欄位的個人看板（待辦事項、進行中、審查中、已完成）。
- **資料隔離**：用戶 A 無法存取用戶 B 的任何看板、欄位或任務卡。
- **ID 查詢**：永遠先呼叫 `get_my_board` 或 `list_columns` 取得欄位 ID，再進行任何欄位或任務卡操作。不要猜測或捏造 ID。

---

## 工具列表

### 看板

| 工具 | 說明 |
|------|------|
| `get_my_board` | 取得完整個人看板（含所有欄位與任務卡） |
| `get_my_stats` | 統計摘要：欄位數、任務總數、優先級分佈 |
| `rename_my_board` | 重新命名看板標題 |

### 欄位

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `list_columns` | `telegram_id` | — | 列出所有欄位 |
| `create_column` | `telegram_id`, `title` | `color` | 新增欄位 |
| `update_column` | `telegram_id`, `column_id` | `title`, `color` | 更新欄位 |
| `delete_column` | `telegram_id`, `column_id` | — | 刪除欄位（含所有任務卡） |

### 任務卡

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `list_cards` | `telegram_id`, `column_id` | — | 列出欄位內所有任務卡 |
| `get_card` | `telegram_id`, `card_id` | — | 取得任務卡詳情 |
| `create_card` | `telegram_id`, `column_id`, `title` | `description`, `priority`, `assignee`, `labels` | 新增任務卡 |
| `update_card` | `telegram_id`, `card_id`, `title` | `description`, `priority`, `assignee`, `labels` | 更新任務卡 |
| `delete_card` | `telegram_id`, `card_id` | — | 刪除任務卡 |
| `move_card` | `telegram_id`, `card_id`, `from_column_id`, `to_column_id` | `to_index` | 移動任務卡 |
| `search_cards` | `telegram_id`, `query` | — | 搜尋任務卡（標題或描述） |

### 快捷工具（Telegram 對話常用）

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `add_quick_task` | `telegram_id`, `title` | `description`, `priority` | 直接新增到第一個欄位，無需指定欄位 ID |
| `mark_done` | `telegram_id`, `card_id` | — | 將任務移至最後一個欄位（通常是「已完成」） |

---

## 參數說明

### `telegram_id`
Telegram 用戶的**數字 ID**（不是用戶名 @username）。例如：`123456789`。OpenClaw 在 Telegram 對話中可直接取得。

### `priority`
接受值：`high` | `medium` | `low`　　預設：`medium`

### `color`
CSS 十六進位色碼，例如：`#6366f1`、`#10b981`、`#f59e0b`　　預設：`#6366f1`

### `labels`
逗號分隔字串，例如：`"工作,重要,今日"`　　傳空字串 `""` 可清除所有標籤

### `to_index`
整數。`0` = 插入最上方，`-1` = 附加到最下方（預設）

---

## 典型 Telegram 對話情境

### 快速記錄待辦
```
用戶：幫我記錄：明天要交季報
Claude → add_quick_task(telegram_id="用戶ID", title="明天要交季報", priority="high")
回應：已新增「明天要交季報」到「待辦事項」
```

### 查看今日任務
```
用戶：我現在有哪些任務？
Claude → get_my_board(telegram_id="用戶ID")
回應：整理並呈現各欄位的任務清單
```

### 標記任務完成
```
用戶：季報已經交了
Claude → search_cards(telegram_id="用戶ID", query="季報")
Claude → mark_done(telegram_id="用戶ID", card_id="...")
回應：「明天要交季報」已移動到「已完成」✓
```

### 整理優先事項
```
用戶：把「開發新功能」的優先級改成高
Claude → search_cards(telegram_id="用戶ID", query="開發新功能")
Claude → update_card(telegram_id="用戶ID", card_id="...", title="開發新功能", priority="high", ...)
```

### 每週摘要
```
用戶：給我本週看板摘要
Claude → get_my_stats(telegram_id="用戶ID")
回應：已完成 N 個，進行中 M 個，待辦 K 個...
```

---

## 資料模型

```
boards (一個 Telegram 用戶 → 一個 board)
  telegram_id  TEXT UNIQUE    ← Telegram 用戶數字 ID
  title        TEXT           ← 看板標題
  
columns (屬於某個 board)
  board_id     TEXT           ← 外鍵 → boards.id
  title        TEXT
  color        TEXT
  position     INTEGER

cards (屬於某個 column)
  column_id    TEXT           ← 外鍵 → columns.id
  title, description, priority, assignee, labels
  position     INTEGER
```

---

## 注意事項

- `delete_column` 會連同欄位內所有任務卡一起刪除，**執行前務必向用戶確認**。
- 所有工具都有**資料所有權驗證**：若某任務卡或欄位不屬於該 `telegram_id` 的看板，操作會被拒絕。
- `search_cards` 只搜尋**當前用戶**的看板，不會跨用戶搜尋。
- 網頁 UI 可透過 `http://localhost:8080?telegram_id=123456789` 查看特定用戶的看板。
