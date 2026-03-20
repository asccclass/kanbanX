# KanbanX — 多人即時看板系統

> Go 1.24 · SherryServer · SQLite · WebSocket · MCP · Telegram 多用戶隔離

KanbanX 是一套雙模式任務管理工具：

- **HTTP 網頁模式**：深色科技主題看板，即時多人協作，WebSocket 推播同步
- **MCP 模式**：供 OpenClaw / Claude Desktop 透過自然語言控制看板，每個 Telegram 用戶擁有獨立隔離的個人看板

兩種模式共用同一個 SQLite 資料庫，資料即時互通。

---

## 目錄

1. [快速開始](#快速開始)
2. [編譯](#編譯)
3. [HTTP 網頁伺服器](#http-網頁伺服器)
4. [MCP 伺服器（OpenClaw / Claude Desktop）](#mcp-伺服器)
5. [REST API 參考](#rest-api-參考)
6. [MCP 工具參考](#mcp-工具參考)
7. [環境設定](#環境設定)
8. [Docker 部署](#docker-部署)
9. [專案結構](#專案結構)
10. [資料庫 Schema](#資料庫-schema)

---

## 快速開始

```bash
# 需求：Go 1.24+（不需要 CGO / C 編譯器）

git clone <此專案>
cd kanban

# 下載依賴（已 vendor，可離線）
go mod vendor

# 編譯
CGO_ENABLED=0 go build -mod=vendor -o kanban .

# 啟動網頁伺服器
./kanban
# → 開啟瀏覽器：http://localhost:8080
```

---

## 編譯

```bash
# 標準編譯（Linux / macOS）
CGO_ENABLED=0 go build -mod=vendor -o kanban .

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -o kanban.exe .

# 使用 Makefile
make build          # 編譯當前平台
make build-windows  # 交叉編譯 Windows
make run            # 直接以 go run 啟動（開發用）
make clean          # 清除編譯產物與資料庫
```

> 使用純 Go 的 SQLite（`ncruces/go-sqlite3` WASM 嵌入版），**不需要安裝 GCC 或 C 工具鏈**。

---

## HTTP 網頁伺服器

### 啟動

```bash
./kanban
# 或指定資料庫路徑
DBPath=/data/myboard.db ./kanban
```

啟動後輸出：

```
  🚀  HTTP Server → http://localhost:8080
  📡  WebSocket   → ws://localhost:8080/ws
  🗄️   Database    → kanban.db
  🤖  MCP mode    → run with --mcp flag
```

### 功能說明

| 功能 | 說明 |
|------|------|
| **即時同步** | 任何人新增/移動/刪除任務，所有連線的瀏覽器即時更新（WebSocket） |
| **多用戶切換** | URL 加上 `?telegram_id=xxx` 即可切換查看不同用戶的看板 |
| **拖拉移動** | HTML5 Drag & Drop，跨欄移動任務卡並自動儲存 |
| **任務詳情** | 點擊任務卡展開右側 Drawer，顯示完整資訊 |
| **斷線重連** | WebSocket 自動重連，指數退避（1→2→4→8→30 秒），分頁切回立即重試 |
| **RWD 響應式** | 支援桌機、平板、手機 |

### 網頁多用戶查看

```
# 查看 Telegram ID 為 123456789 的用戶看板
http://localhost:8080?telegram_id=123456789

# 查看所有用戶列表（管理用）
GET /api/users
```

---

## MCP 伺服器

### 什麼是 MCP 模式？

以 `--mcp` 旗標啟動時，程式透過 **stdio JSON-RPC** 提供 16 個 MCP 工具，讓 Claude（或其他 MCP 相容的 AI）用自然語言操作看板。

### 啟動

```bash
# OpenClaw / Claude Desktop 會自動管理此程序，通常不需手動執行
./kanban --mcp

# 手動測試（送一行 JSON-RPC，按 Ctrl+C 結束）
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./kanban --mcp
```

### 設定 OpenClaw / Claude Desktop

在 MCP 設定檔（例如 `claude_desktop_config.json`）加入：

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

**路徑範例：**

```json
// macOS / Linux
"command": "/Users/yourname/kanban/kanban"

// Windows
"command": "C:\\Users\\yourname\\kanban\\kanban.exe"
```

### 安裝 SKILL.md

將 `SKILL.md` 複製到 OpenClaw 的 skills 資料夾（通常為 `/mnt/skills/user/` 或 OpenClaw 文件指定路徑），Claude 讀取後即知道如何正確呼叫所有工具。

### Telegram 多用戶隔離

每個 Telegram 用戶以其**數字 ID**（例如 `123456789`）識別。首次呼叫任何工具時，系統自動建立含 4 個預設欄位的個人看板：

```
待辦事項 → 進行中 → 審查中 → 已完成
```

各用戶的資料完全隔離，無法互相存取。

### 自然語言使用範例

在 OpenClaw 的 Telegram 對話中直接說：

```
「幫我記錄：明天下午三點要開週會」
→ Claude 呼叫 add_quick_task，新增到你的待辦欄位

「我的看板現在有什麼任務？」
→ Claude 呼叫 get_my_board，整理並回報

「週會準備好了，幫我標記為完成」
→ Claude 呼叫 search_cards 找到任務，再呼叫 mark_done 移至已完成欄位

「給我本週進度摘要」
→ Claude 呼叫 get_my_stats，回報各欄位任務數量

「把『開發新功能』的優先級改為高」
→ Claude 呼叫 search_cards + update_card 完成修改
```

---

## REST API 參考

所有 API 均回傳統一格式：

```json
// 成功
{ "status": "ok", "data": { ... } }

// 失敗
{ "status": "error", "message": "錯誤原因" }
```

多用戶識別：在 Query String 加上 `?telegram_id=<數字ID>`，省略時使用共用的 `web_admin` 看板。

### Board

| Method | Path | Query | 說明 |
|--------|------|-------|------|
| `GET` | `/api/board` | `?telegram_id=` | 取得完整看板（含所有欄位與任務卡） |
| `GET` | `/api/users` | — | 列出所有已建立看板的用戶 |

### Columns（欄位）

| Method | Path | Query | 說明 |
|--------|------|-------|------|
| `GET` | `/api/columns` | `?telegram_id=` | 列出所有欄位 |
| `GET` | `/api/columns/{id}` | — | 取得單一欄位（含任務卡） |
| `GET` | `/api/columns/{id}/cards` | `?telegram_id=` | 列出欄位內所有任務卡 |
| `POST` | `/api/columns` | `?telegram_id=` | 新增欄位 |
| `PUT` | `/api/columns/{id}` | `?telegram_id=` | 更新欄位名稱/顏色 |
| `DELETE` | `/api/columns/{id}` | `?telegram_id=` | 刪除欄位（含所有任務卡） |

**POST /api/columns 請求範例：**
```json
{
  "title": "封存",
  "color": "#64748b"
}
```

### Cards（任務卡）

| Method | Path | Query | 說明 |
|--------|------|-------|------|
| `GET` | `/api/cards/{id}` | `?telegram_id=` | 取得單一任務卡 |
| `POST` | `/api/cards` | `?telegram_id=` | 新增任務卡 |
| `POST` | `/api/cards/move` | `?telegram_id=` | 移動任務卡 |
| `PUT` | `/api/cards/{id}` | `?telegram_id=` | 更新任務卡 |
| `DELETE` | `/api/cards/{id}` | `?telegram_id=` | 刪除任務卡 |

**POST /api/cards 請求範例：**
```json
{
  "columnId": "<欄位ID>",
  "title": "實作登入功能",
  "description": "支援 OAuth 2.0 與 JWT",
  "priority": "high",
  "assignee": "Alice",
  "labels": ["後端", "安全"]
}
```

**POST /api/cards/move 請求範例：**
```json
{
  "cardId": "<任務卡ID>",
  "fromColumnId": "<來源欄位ID>",
  "toColumnId": "<目標欄位ID>",
  "toIndex": -1
}
```

### WebSocket

| Path | Query | 說明 |
|------|-------|------|
| `GET /ws` | `?telegram_id=` | 升級為 WebSocket 連線，即時接收看板更新 |

**WebSocket 訊息格式（伺服器 → 客戶端）：**
```json
// 看板更新（任何操作後廣播）
{ "type": "board_update", "payload": { ...Board } }

// 在線人數更新
{ "type": "online_count", "payload": 3 }
```

### 完整使用範例（cURL）

```bash
# 取得特定用戶的看板
curl "http://localhost:8080/api/board?telegram_id=123456789"

# 新增欄位
curl -X POST "http://localhost:8080/api/columns?telegram_id=123456789" \
  -H "Content-Type: application/json" \
  -d '{"title":"封存","color":"#64748b"}'

# 新增任務卡
curl -X POST "http://localhost:8080/api/cards?telegram_id=123456789" \
  -H "Content-Type: application/json" \
  -d '{"columnId":"<col-id>","title":"修復登入 Bug","priority":"high","assignee":"Alice","labels":["Bug"]}'

# 移動任務卡（標記完成）
curl -X POST "http://localhost:8080/api/cards/move?telegram_id=123456789" \
  -H "Content-Type: application/json" \
  -d '{"cardId":"<card-id>","fromColumnId":"<from>","toColumnId":"<done-col>","toIndex":-1}'

# 更新任務卡
curl -X PUT "http://localhost:8080/api/cards/<card-id>?telegram_id=123456789" \
  -H "Content-Type: application/json" \
  -d '{"title":"修復登入 Bug ✓","priority":"low","assignee":"Alice","labels":["完成"]}'

# 刪除任務卡
curl -X DELETE "http://localhost:8080/api/cards/<card-id>?telegram_id=123456789"
```

---

## MCP 工具參考

> **重要**：每個工具都需要 `telegram_id` 參數（Telegram 用戶數字 ID）。

### 看板工具

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `get_my_board` | `telegram_id` | — | 取得完整個人看板 |
| `get_my_stats` | `telegram_id` | — | 統計：欄位數、任務數、優先級分佈 |
| `rename_my_board` | `telegram_id`, `title` | — | 重新命名看板 |

### 欄位工具

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `list_columns` | `telegram_id` | — | 列出所有欄位 |
| `create_column` | `telegram_id`, `title` | `color` | 新增欄位 |
| `update_column` | `telegram_id`, `column_id` | `title`, `color` | 更新欄位 |
| `delete_column` | `telegram_id`, `column_id` | — | 刪除欄位與所有任務卡 |

### 任務卡工具

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `list_cards` | `telegram_id`, `column_id` | — | 列出欄位內所有任務卡 |
| `get_card` | `telegram_id`, `card_id` | — | 取得任務卡詳情 |
| `create_card` | `telegram_id`, `column_id`, `title` | `description`, `priority`, `assignee`, `labels` | 新增任務卡 |
| `update_card` | `telegram_id`, `card_id`, `title` | `description`, `priority`, `assignee`, `labels` | 更新任務卡 |
| `delete_card` | `telegram_id`, `card_id` | — | 刪除任務卡 |
| `move_card` | `telegram_id`, `card_id`, `from_column_id`, `to_column_id` | `to_index` | 移動任務卡 |
| `search_cards` | `telegram_id`, `query` | — | 全文搜尋（標題+描述） |

### 快捷工具

| 工具 | 必填參數 | 選填參數 | 說明 |
|------|---------|---------|------|
| `add_quick_task` | `telegram_id`, `title` | `description`, `priority` | 快速新增到第一個欄位，無需欄位 ID |
| `mark_done` | `telegram_id`, `card_id` | — | 移動至最後一個欄位（通常是「已完成」） |

### 參數說明

| 參數 | 類型 | 說明 |
|------|------|------|
| `telegram_id` | string | Telegram 用戶數字 ID，例如 `"123456789"` |
| `priority` | string | `high` \| `medium` \| `low`，預設 `medium` |
| `color` | string | CSS 十六進位色碼，例如 `"#6366f1"`，預設 `"#6366f1"` |
| `labels` | string | 逗號分隔，例如 `"工作,重要"`；傳 `""` 清除所有標籤 |
| `to_index` | number | `0`=最上方，`-1`=最下方（預設） |

---

## 環境設定

編輯 `envfile`（或以環境變數覆蓋）：

```env
# 伺服器設定
PORT=8080
SystemName=KanbanX

# 檔案路徑
DocumentRoot=www/html
TemplateRoot=www/template

# SQLite 資料庫路徑
# 相對路徑（相對於執行檔位置）或絕對路徑
DBPath=kanban.db
```

---

## Docker 部署

### Dockerfile

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o kanban .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/kanban .
COPY envfile .
COPY www/ ./www/

# 資料庫持久化目錄
VOLUME ["/app/data"]
ENV DBPath=/app/data/kanban.db
ENV PORT=8080

EXPOSE 8080
ENTRYPOINT ["/app/kanban"]
```

### 同時提供 HTTP 與 MCP

由於 MCP 使用 stdio，HTTP 模式才需要開放 port：

```yaml
# docker-compose.yml
services:
  kanbanx:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
    environment:
      - DBPath=/app/data/kanban.db
      - PORT=8080
```

```bash
docker compose up -d
```

MCP 模式下，OpenClaw 直接在本機執行 `kanban --mcp`，無需 Docker。

---

## 專案結構

```
kanban/
├── main.go              # 進入點：--mcp 旗標切換模式
├── mcp_server.go        # MCP stdio 伺服器（16 個工具）
├── router.go            # HTTP 路由
├── handlers.go          # HTTP handlers（CRUD + WebSocket）
├── hub.go               # WebSocket 廣播中心
├── db.go                # SQLiteStore（多用戶 board 管理）
├── board.go             # 資料模型（Board, Column, Card）
├── envfile              # 環境設定
├── go.mod / go.sum      # Go 模組定義
├── Makefile             # 建置指令
├── SKILL.md             # OpenClaw Skill 文件（供 Claude 讀取）
├── vendor/              # 所有依賴原始碼（可離線編譯）
├── vendor_patches/      # 非 GitHub 依賴的本地鏡像
└── www/
    └── html/
        └── index.html   # 前端（深色玻璃擬態科技主題）
```

---

## 資料庫 Schema

```sql
-- 每個 Telegram 用戶一個 board
CREATE TABLE boards (
    id          TEXT PRIMARY KEY,
    telegram_id TEXT NOT NULL UNIQUE,  -- Telegram 數字 ID
    title       TEXT NOT NULL DEFAULT '我的看板',
    created_at  DATETIME NOT NULL
);

-- 欄位屬於某個 board，刪除 board 時 CASCADE 刪除
CREATE TABLE columns (
    id         TEXT PRIMARY KEY,
    board_id   TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '#6366f1',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL
);

-- 任務卡屬於某個 column，刪除 column 時 CASCADE 刪除
CREATE TABLE cards (
    id          TEXT PRIMARY KEY,
    column_id   TEXT NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority    TEXT NOT NULL DEFAULT 'medium',  -- high | medium | low
    assignee    TEXT NOT NULL DEFAULT '',
    labels      TEXT NOT NULL DEFAULT '[]',       -- JSON 陣列
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
```

---

## 注意事項

- **MCP 與 HTTP 共用同一個 DB**：兩個程序同時存取同一個 `kanban.db` 是安全的（SQLite WAL 模式）
- **delete_column 級聯刪除**：會一併刪除欄位內所有任務卡，操作前請確認
- **資料所有權驗證**：每個操作都會驗證 `telegram_id` 是否為資料的擁有者，防止跨用戶存取
- **首次使用**：呼叫任何工具/API 並帶入 `telegram_id` 後，系統自動建立個人看板，無需手動初始化
