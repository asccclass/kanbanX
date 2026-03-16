# KanbanX — 即時多人看板系統（SQLite 持久化版）

以 Go 1.24 + SherryServer + SQLite + WebSocket 打造的全功能看板工具。
資料完整持久化至 SQLite，重啟不遺失。

---

## 功能特色

| 功能 | 說明 |
|------|------|
| 🗄️ SQLite 持久化 | 所有看板、欄位、任務卡均寫入 SQLite，重啟不遺失 |
| 📡 即時多人同步 | WebSocket 廣播：任何人操作立即推播給所有連線者 |
| 🃏 完整 CRUD API | 看板、欄位、任務卡各自擁有 RESTful CRUD 端點 |
| 🖱️ 拖拉移動任務 | HTML5 Drag & Drop，跨欄移動並持久化位置順序 |
| 👥 在線人數顯示 | 即時顯示目前多少人正在同步使用 |
| 📱 RWD 響應式介面 | 桌機 / 平板 / 手機自適應深色科技主題 |

---

## 快速啟動

```bash
# 需求：Go 1.24+、GCC (for CGO / sqlite3)

cd kanban

# 方式一：使用 vendor 目錄（無網路也可編譯）
CGO_ENABLED=1 go build -mod=vendor -o kanban .
./kanban

# 方式二：直接執行（自動下載依賴）
go run .

# 瀏覽器開啟：http://localhost:8080
```

### 環境設定（`envfile`）

```env
PORT=8080
DocumentRoot=www/html
TemplateRoot=www/template
DBPath=kanban.db       # SQLite 資料庫路徑
```

---

## REST API 完整列表

### Board

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/board` | 取得完整看板（含所有欄位與任務卡） |

### Cards（任務卡）

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/cards/{id}` | 取得單一任務卡 |
| `POST` | `/api/cards` | 新增任務卡 |
| `PUT` | `/api/cards/{id}` | 更新任務卡 |
| `DELETE` | `/api/cards/{id}` | 刪除任務卡 |
| `POST` | `/api/cards/move` | 移動任務卡（跨欄位） |

### Columns（欄位）

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/columns` | 列出所有欄位 |
| `GET` | `/api/columns/{id}` | 取得單一欄位（含任務卡） |
| `GET` | `/api/columns/{id}/cards` | 列出欄位內所有任務卡 |
| `POST` | `/api/columns` | 新增欄位 |
| `PUT` | `/api/columns/{id}` | 更新欄位名稱/顏色 |
| `DELETE` | `/api/columns/{id}` | 刪除欄位（含旗下所有任務卡） |

### WebSocket

| Path | 說明 |
|------|------|
| `GET /ws` | WebSocket 升級端點 |

---

## API 使用範例

```bash
# 取得完整看板
curl http://localhost:8080/api/board

# 新增任務卡
curl -X POST http://localhost:8080/api/cards \
  -H "Content-Type: application/json" \
  -d '{
    "columnId": "<column-id>",
    "title": "實作登入功能",
    "description": "支援 OAuth 2.0",
    "priority": "high",
    "assignee": "Alice",
    "labels": ["後端", "安全"]
  }'

# 更新任務卡
curl -X PUT http://localhost:8080/api/cards/<card-id> \
  -H "Content-Type: application/json" \
  -d '{
    "title": "實作登入功能 ✓",
    "description": "已完成",
    "priority": "low",
    "assignee": "Alice",
    "labels": ["完成"]
  }'

# 移動任務卡
curl -X POST http://localhost:8080/api/cards/move \
  -H "Content-Type: application/json" \
  -d '{
    "cardId": "<card-id>",
    "fromColumnId": "<from-col>",
    "toColumnId": "<to-col>",
    "toIndex": 0
  }'

# 新增欄位
curl -X POST http://localhost:8080/api/columns \
  -H "Content-Type: application/json" \
  -d '{"title": "封存", "color": "#64748b"}'

# 更新欄位
curl -X PUT http://localhost:8080/api/columns/<col-id> \
  -H "Content-Type: application/json" \
  -d '{"title": "封存區", "color": "#334155"}'

# 刪除欄位（同時刪除其下所有任務卡）
curl -X DELETE http://localhost:8080/api/columns/<col-id>
```

---

## API 回傳格式

所有 API 回傳統一格式：

```json
// 成功
{ "status": "ok", "data": { ... } }

// 失敗
{ "status": "error", "message": "error description" }
```

---

## WebSocket 訊息格式

```json
// 伺服器 → 客戶端：看板完整更新
{ "type": "board_update", "payload": { ...Board } }

// 伺服器 → 客戶端：在線人數
{ "type": "online_count", "payload": 3 }
```

---

## SQLite 資料庫 Schema

```sql
CREATE TABLE boards (
  id TEXT PRIMARY KEY, title TEXT NOT NULL, created_at DATETIME NOT NULL
);
CREATE TABLE columns (
  id TEXT PRIMARY KEY, board_id TEXT NOT NULL,
  title TEXT NOT NULL, color TEXT NOT NULL,
  position INTEGER NOT NULL, created_at DATETIME NOT NULL,
  FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
);
CREATE TABLE cards (
  id TEXT PRIMARY KEY, column_id TEXT NOT NULL,
  title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
  priority TEXT NOT NULL DEFAULT 'medium', assignee TEXT NOT NULL DEFAULT '',
  labels TEXT NOT NULL DEFAULT '[]',   -- JSON array
  position INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
  FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
);
```

---

## 專案結構

```
kanban/
├── main.go              # 進入點：初始化 SQLiteStore、Hub、SherryServer
├── router.go            # 路由：所有 REST API + WebSocket 端點
├── handlers.go          # HTTP handlers（CRUD + WS upgrade）
├── hub.go               # WebSocket Hub（連線管理、ping/pong、廣播）
├── db.go                # SQLiteStore：schema migration、所有 DB 操作
├── board.go             # 資料模型（Card、Column、Board）+ DTO
├── envfile              # 環境設定
├── go.mod / go.sum      # 模組定義
├── vendor/              # 所有依賴原始碼（可離線編譯）
├── vendor_patches/      # 本地鏡像：go.uber.org/zap 等（非 GitHub 的依賴）
└── www/html/index.html  # 前端：深色科技主題 Kanban UI
```

---

## Docker 部署

```dockerfile
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY . .
RUN CGO_ENABLED=1 go build -mod=vendor -o kanban .

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
