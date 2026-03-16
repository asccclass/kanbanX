# KanbanX

> 即時多人看板系統 — Go 1.24 · SherryServer · SQLite · WebSocket · MCP

KanbanX 是一套以 Go 打造的全功能看板工具，資料持久化至 SQLite，支援多人即時協作，並可作為 **MCP（Model Context Protocol）服務**讓 Claude / OpenClaw 直接操控看板。

---

## 功能總覽

| 分類 | 功能 |
|------|------|
| 🗄️ **持久化** | SQLite 儲存，重啟資料不遺失；WAL 模式支援高並發讀寫 |
| 📡 **即時同步** | WebSocket 廣播：任何操作立即推播至所有連線的瀏覽器 |
| 🔄 **自動重連** | 指數退避重連（1→2→4→8→16→30s）、分頁切回立即重試、頂部 Banner 倒數提示 |
| 🃏 **任務卡展開** | 點擊卡片滑出右側 Drawer，完整顯示描述、標籤、負責人、時間戳記 |
| 🖱️ **拖拉移動** | HTML5 Drag & Drop，跨欄移動並同步至資料庫 |
| 🏗️ **欄位管理** | 新增、更名、換色、刪除欄位（含 CASCADE 刪除任務卡） |
| 👥 **在線人數** | Navbar 即時顯示連線人數 |
| 📱 **RWD** | 桌機 / 平板 / 手機自適應深色玻璃擬態介面 |
| 🤖 **MCP 服務** | 13 個 MCP Tools，讓 Claude 以自然語言管理看板 |
| 🌐 **REST API** | 完整 CRUD endpoints，可供外部系統整合 |

---

## 系統需求

- Go **1.24+**
- 無需 C 編譯器（使用 `ncruces/go-sqlite3` 純 Go WASM 實作）

---

## 快速啟動

### 1. 編譯

```bash
cd kanban

# 使用 vendor 目錄（無網路也可編譯，推薦）
CGO_ENABLED=0 go build -mod=vendor -o kanban .

# 或直接執行（自動下載依賴，需要網路）
go run .
```

### 2. 啟動 HTTP 網頁服務

```bash
./kanban
# 或
go run .
```

開啟瀏覽器：**http://localhost:8080**

### 3. 啟動 MCP 服務（供 Claude / OpenClaw 使用）

```bash
./kanban --mcp
```

> HTTP 模式與 MCP 模式共用同一個 `kanban.db`，可同時開啟，資料互相同步。

---

## 環境設定

編輯專案根目錄的 `envfile`：

```env
PORT=8080                  # HTTP 服務埠（MCP 模式忽略此設定）
DocumentRoot=www/html      # 靜態檔案根目錄
TemplateRoot=www/template  # 模板根目錄
DBPath=kanban.db           # SQLite 資料庫路徑
```

---

## 網頁介面操作說明

### 看板主畫面

| 操作 | 方式 |
|------|------|
| 查看任務卡詳情 | **點擊**卡片主體 → 右側 Drawer 展開 |
| 編輯任務卡 | Drawer 中按「✎ 編輯任務」，或直接點卡片右上角 ✎ |
| 刪除任務卡 | Drawer 中按「✕ 刪除」，或直接點卡片右上角 ✕ |
| 移動任務卡 | **拖拉**卡片至目標欄位放開 |
| 新增任務卡 | 點擊欄位底部「＋ 新增任務」 |
| 新增欄位 | Navbar 右上角「＋ 新增欄位」，或看板最右側虛線格 |

### 卡片詳情 Drawer

點擊任何卡片後，右側滑入詳情面板，包含：

- 頂部色條（依優先級變色）
- 所屬欄位名稱
- 完整標題（無截斷）
- 優先級 Badge
- 負責人（頭像 + 姓名）
- 完整標籤列表
- 完整描述（保留換行）
- 建立時間 / 最後更新時間
- 快速操作：**編輯** / **刪除**

按 `Esc`、點擊遮罩區域、或 Drawer 右上角 ✕ 可關閉。

### WebSocket 連線狀態

Navbar 右上角顯示連線 Pill：

| 顏色 | 狀態 |
|------|------|
| 🔵 青色 | 已連線 |
| 🔴 紅色 | 已斷線 |
| 🟡 琥珀色 | 重連中 |

**斷線時**頂部橫幅滑出，顯示重試倒數與「立即重試」按鈕。  
重連成功後橫幅短暫顯示綠色「連線已恢復」，隨即自動收起。

---

## REST API

所有 API 回傳統一格式：

```json
// 成功
{ "status": "ok", "data": { ... } }

// 失敗
{ "status": "error", "message": "描述錯誤原因" }
```

### 看板

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/board` | 取得完整看板（含所有欄位與任務卡） |

### 欄位（Columns）

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/columns` | 列出所有欄位 |
| `GET` | `/api/columns/{id}` | 取得單一欄位（含任務卡） |
| `GET` | `/api/columns/{id}/cards` | 列出欄位內所有任務卡 |
| `POST` | `/api/columns` | 新增欄位 |
| `PUT` | `/api/columns/{id}` | 更新欄位名稱 / 顏色 |
| `DELETE` | `/api/columns/{id}` | 刪除欄位（含旗下所有任務卡） |

### 任務卡（Cards）

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/cards/{id}` | 取得單一任務卡 |
| `POST` | `/api/cards` | 新增任務卡 |
| `PUT` | `/api/cards/{id}` | 更新任務卡 |
| `DELETE` | `/api/cards/{id}` | 刪除任務卡 |
| `POST` | `/api/cards/move` | 移動任務卡至另一欄位 |

### WebSocket

| Path | 說明 |
|------|------|
| `GET /ws` | WebSocket 升級端點 |

### cURL 範例

```bash
BASE=http://localhost:8080

# 取得完整看板
curl $BASE/api/board

# 新增欄位
curl -X POST $BASE/api/columns \
  -H "Content-Type: application/json" \
  -d '{"title": "測試中", "color": "#06c8e8"}'

# 新增任務卡
curl -X POST $BASE/api/cards \
  -H "Content-Type: application/json" \
  -d '{
    "columnId": "<column-id>",
    "title": "實作登入功能",
    "description": "支援 OAuth 2.0 + JWT",
    "priority": "high",
    "assignee": "Alice",
    "labels": ["後端", "安全"]
  }'

# 更新任務卡
curl -X PUT $BASE/api/cards/<card-id> \
  -H "Content-Type: application/json" \
  -d '{
    "title": "實作登入功能 ✓",
    "description": "已完成驗收",
    "priority": "low",
    "assignee": "Alice",
    "labels": ["完成"]
  }'

# 移動任務卡（to_index: -1 = 加到最後）
curl -X POST $BASE/api/cards/move \
  -H "Content-Type: application/json" \
  -d '{
    "cardId": "<card-id>",
    "fromColumnId": "<from-id>",
    "toColumnId": "<to-id>",
    "toIndex": -1
  }'

# 刪除任務卡
curl -X DELETE $BASE/api/cards/<card-id>

# 更新欄位
curl -X PUT $BASE/api/columns/<col-id> \
  -H "Content-Type: application/json" \
  -d '{"title": "驗收完成", "color": "#22c87a"}'

# 刪除欄位（同時刪除其下所有任務卡）
curl -X DELETE $BASE/api/columns/<col-id>
```

---

## WebSocket 訊息格式

客戶端連線至 `ws://localhost:8080/ws` 後，伺服器主動推送：

```json
// 看板完整更新（任何 CRUD 操作後廣播）
{ "type": "board_update", "payload": { /* Board 物件 */ } }

// 在線人數更新（有人連線或斷線時廣播）
{ "type": "online_count", "payload": 3 }
```

---

## MCP 服務

### 設定 Claude Desktop / OpenClaw

在 `claude_desktop_config.json` 加入：

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

### 可用 MCP Tools（13 個）

**看板**

| Tool | 說明 |
|------|------|
| `get_board` | 取得完整看板（所有欄位與任務卡） |
| `get_stats` | 統計摘要：各欄位任務數、優先級分佈 |

**欄位**

| Tool | 必填參數 | 選填參數 | 說明 |
|------|----------|----------|------|
| `list_columns` | — | — | 列出所有欄位 |
| `create_column` | `title` | `color` | 新增欄位 |
| `update_column` | `column_id` | `title`, `color` | 更新欄位 |
| `delete_column` | `column_id` | — | 刪除欄位（含所有任務卡） |

**任務卡**

| Tool | 必填參數 | 選填參數 | 說明 |
|------|----------|----------|------|
| `list_cards` | `column_id` | — | 列出欄位內所有任務卡 |
| `get_card` | `card_id` | — | 取得單一任務卡詳情 |
| `create_card` | `column_id`, `title` | `description`, `priority`, `assignee`, `labels` | 新增任務卡 |
| `update_card` | `card_id`, `title` | `description`, `priority`, `assignee`, `labels` | 更新任務卡 |
| `delete_card` | `card_id` | — | 刪除任務卡 |
| `move_card` | `card_id`, `from_column_id`, `to_column_id` | `to_index` | 移動任務卡 |
| `search_cards` | `query` | — | 以關鍵字搜尋任務卡 |

### 參數說明

| 參數 | 型別 | 說明 |
|------|------|------|
| `priority` | string | `high` \| `medium` \| `low`，預設 `medium` |
| `color` | string | CSS 十六進位色碼，例如 `#6366f1`，預設 `#6366f1` |
| `labels` | string | 逗號分隔字串，例如 `前端,Bug,緊急`；留空則清除所有標籤 |
| `to_index` | number | `0` = 最上方，`-1` = 最下方，預設 `-1` |

### MCP 使用情境範例

```
# 查看看板現況
用 get_board 工具顯示目前的看板

# 新增任務卡
在「進行中」欄位新增一張高優先級的任務「修復登入 Bug」，
指派給 Alice，標籤為「後端, Bug」

# 移動任務卡
把「修復登入 Bug」移到「已完成」欄位

# 搜尋任務
搜尋所有包含「API」的任務卡

# 看板統計
用 get_stats 顯示哪個欄位待辦最多

# 批次建立任務
在「待辦事項」欄位新增以下三張任務，全部中優先、指派給 Bob：
- 撰寫單元測試
- 更新 API 文件
- 設定 Staging 環境
```

---

## SQLite Schema

```sql
CREATE TABLE boards (
  id         TEXT PRIMARY KEY,
  title      TEXT NOT NULL,
  created_at DATETIME NOT NULL
);

CREATE TABLE columns (
  id         TEXT PRIMARY KEY,
  board_id   TEXT NOT NULL,
  title      TEXT NOT NULL,
  color      TEXT NOT NULL DEFAULT '#6366f1',
  position   INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
);

CREATE TABLE cards (
  id          TEXT PRIMARY KEY,
  column_id   TEXT NOT NULL,
  title       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  priority    TEXT NOT NULL DEFAULT 'medium',
  assignee    TEXT NOT NULL DEFAULT '',
  labels      TEXT NOT NULL DEFAULT '[]',  -- JSON 陣列
  position    INTEGER NOT NULL DEFAULT 0,
  created_at  DATETIME NOT NULL,
  updated_at  DATETIME NOT NULL,
  FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
);
```

---

## 專案結構

```
kanban/
├── main.go              # 進入點：--mcp 旗標切換 HTTP / MCP 模式
├── router.go            # HTTP 路由：REST API + WebSocket
├── handlers.go          # HTTP handlers（CRUD + WS upgrade）
├── hub.go               # WebSocket Hub（連線管理、ping/pong、廣播）
├── db.go                # SQLiteStore：schema migration + 所有 DB 操作
├── board.go             # 資料模型（Card, Column, Board）+ DTO
├── mcp_server.go        # MCP stdio 服務：13 個 tools
├── SKILL.md             # MCP Skill 說明文件（供 Claude / OpenClaw 讀取）
├── envfile              # 環境設定
├── go.mod / go.sum      # 模組定義
├── Makefile             # 常用指令
├── vendor/              # 所有依賴原始碼（離線可編譯）
├── vendor_patches/      # 非 GitHub 依賴的本地鏡像
└── www/
    └── html/
        └── index.html   # 前端：深色玻璃擬態 Kanban UI
```

---

## Makefile

```bash
make build          # 編譯 binary（CGO_ENABLED=0）
make build-windows  # 交叉編譯 Windows .exe
make run            # 直接執行（go run .）
make vendor         # 更新 vendor 目錄
make tidy           # 整理 go.mod / go.sum
make clean          # 清除 binary 與資料庫
```

---

## Docker 部署

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o kanban .

FROM alpine
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/kanban .
COPY envfile .
COPY www/ ./www/
EXPOSE 8080
VOLUME ["/app/data"]
ENV DBPath=/app/data/kanban.db
ENTRYPOINT ["/app/kanban"]
```

```bash
docker build -t kanbanx .
docker run -p 8080:8080 -v $(pwd)/data:/app/data kanbanx

# MCP 模式
docker run -v $(pwd)/data:/app/data kanbanx --mcp
```

---

## 技術架構

```
瀏覽器 A ──WebSocket──┐
瀏覽器 B ──WebSocket──┤  WebSocket Hub（廣播中心）
瀏覽器 C ──WebSocket──┘         │
         ──REST API──▶  HTTP Handlers ──▶ SQLiteStore
Claude ──MCP stdio──▶  MCP Server  ──▶ SQLiteStore
                                              │
                                         kanban.db（WAL）
```

- **HTTP 模式**：SherryServer + gorilla/websocket，服務網頁 UI 與 REST API
- **MCP 模式**：mark3labs/mcp-go v0.45.0，透過 stdio 提供 13 個工具
- **資料庫**：ncruces/go-sqlite3（純 Go WASM，無需 CGO），WAL 模式 + Foreign Key CASCADE
- **前端**：HTML + CSS（玻璃擬態設計）+ HTMX + Vanilla JS，WebSocket 即時同步

---

## 授權

MIT License
