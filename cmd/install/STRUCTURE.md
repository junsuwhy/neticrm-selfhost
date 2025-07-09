# Docker Compose Self‑Host 安裝器（Go 單檔版）

## 目標

用 **單一 Go 程式**（`main.go`）在 Linux bash 下引導使用者：

1. 偵測並處理現有 `.env` 與資料庫檔。
2. 確認系統已安裝 `docker` / `docker compose`。
3. 互動式詢問語言與所有環境變數，密碼欄位遮蔽並雙重確認。
4. 將設定寫入 `.env` 後，自動執行 `docker compose up -d` 完成部署。
5. 全程使用彩色輸出增強可讀性。

---

## 專案結構（極簡）

```text
cmd/install/
└── main.go       # 只此一檔

example.env   # 預設範本（以 embed 內嵌）
data/example.Caddyfile

```

---

## 相依套件

| 功能         | 套件                                 | 匯入別名       |
| ---------- | ---------------------------------- | ---------- |
| 互動式選單 / 輸入 | `github.com/AlecAivazis/survey/v2` | `survey`   |
| 讀寫 `.env`  | `github.com/joho/godotenv`         | `godotenv` |
| 彩色輸出       | `github.com/fatih/color`           | `color`    |
| 檔案 / 指令操作  | 標準庫 `os`、`os/exec`、`embed`         | —          |

安裝：

```bash
go get github.com/AlecAivazis/survey/v2 \
       github.com/joho/godotenv \
       github.com/fatih/color
```

---

## 主程式骨架

```go
// main.go
package main

import (
    "embed"
    "fmt"
    "log"
    "os"
    "os/exec"

    "github.com/AlecAivazis/survey/v2"
    "github.com/fatih/color"
)

//go:embed example.env
var tplFS embed.FS

func main() {
    if err := doCheck(); err != nil {
        log.Fatal(err)
    }

    cfg, err := doAsk()
    if err != nil {
        log.Fatal(err)
    }

    if err := doRun(cfg); err != nil {
        log.Fatal(err)
    }

    color.New(color.FgGreen).Println("✅ 安裝完成！")
}
```

> `cfg` 可以是自訂 struct，收集所有互動結果。

---

### doCheck：事前檢查

#### 語言選擇

安裝程序首先會詢問使用者選擇語言：

- 支援語言：
  - 英文
  - 繁體中文

此選項會影響所有呈現的問句，和最後寫入的 `.env` 檔案中的 `LANGUAGE` 變數。

#### 檢查 Docker & Compose

   ```go
   if _, err := exec.LookPath("docker"); err != nil { ... }
   ```
   
  **確認 Docker Compose 插件可用**

   ```go
   // 使用 Docker CLI 確認 Compose 插件版本
   if err := exec.Command("docker", "compose", "version").Run(); err != nil {
       return fmt.Errorf("請先安裝並啟用 Docker Compose 插件: %w", err)
   }
   ```

  * 若無則提供這些選項：
    * 告訴使用者，建議安裝 Docker，結束安裝
    * 僅更改 .env 檔案，那就繼續
  * 有即可跳過此步驟

#### 偵測 `.env` 與資料庫資料夾 `/data/mariadb_data`（資料庫資料夾需確認有無內容）

   * 若皆存在，說明其中 DOMAIN 和 HTTP_PORT、ADMIN_LOGIN_USER 變數，使用 `survey.Select` 提供：

     * 執行 docker 啟動指令（檢查有無 Caddyfile 檔案，有則使用 SSL 配置啟動，無則使用預設配置啟動）
     * 備份並覆蓋（確認則提示將複製為 `.env.bak, 若 .bak 已存在則用 .env.bak1, .env.bak2...etc ， /data/mariadb_data 亦詢問`）
     * 檢視 ADMIN_LOGIN_PASSWORD （請注意此密碼為明文，且僅為初始密碼，可能已更改）
     * 結束安裝
   * 備份用 `os.ReadFile` + `os.WriteFile`
   * 不存在則跳過此步驟

#### 偵測 Caddyfile

   * 若存在，顯示 Caddyfile 的內容，詢問是否要使用 SSL 配置啟動
   * 不存在則跳過此步驟

### doAsk：互動詢問

#### 域名和 SSL 設定

##### SSL 設定詢問
- 詢問：「您是否有網域並希望自動設定 SSL？ [y/N]」
- 如果選擇「y」：
  - 詢問域名：「請輸入您的域名 (例如 example.com)」（這是必填項目，SSL 模式下不可為空）
  - 詢問電子郵件：「請輸入您的電子郵件 (用於 Let's Encrypt SSL 證書)」
  - 端口不需要手動設定，由 Caddy 自動處理

- 如果選擇「n」或直接按 Enter：
  - 詢問網站網址：「網站網址 (domain，可留空)」
  - 如果留空域名，則詢問端口：「請輸入 Port (預設 8080)」
    - 如果端口也留空，將使用預設值「8080」

#### MySQL 設定

- 詢問：「是否要修改 MySQL 參數？(y/N)」
- 如果選擇「n」或直接按 Enter：
  - 自動生成 `MYSQL_ROOT_PASSWORD`（13 位隨機密碼）
  - 保留 `MYSQL_DATABASE` 和 `MYSQL_USER` 的預設值
  - 自動生成 `MYSQL_PASSWORD`（13 位隨機密碼）
  
- 如果選擇「y」：
  - `MYSQL_ROOT_PASSWORD`：
    - 詢問：「MYSQL_ROOT_PASSWORD (留空自動產生)」
    - **密碼使用隱碼輸入，不顯示在螢幕上**
    - 如果留空，自動生成 13 位隨機密碼
    - 如果輸入密碼，則要求再次輸入確認：「請再次輸入MYSQL_ROOT_PASSWORD密碼確認」
    - 如果兩次密碼不一致，顯示錯誤訊息：「✗ 兩次密碼不一致，請重新輸入。」
  
  - `MYSQL_DATABASE`：
    - 詢問：「MYSQL_DATABASE (留空使用預設值)」
    
  - `MYSQL_USER`：
    - 詢問：「MYSQL_USER (留空使用預設值)」
    
  - `MYSQL_PASSWORD`：
    - 詢問：「MYSQL_PASSWORD (留空自動產生)」
    - **密碼使用隱碼輸入，不顯示在螢幕上**
    - 如果留空，自動生成 13 位隨機密碼
    - 如果輸入密碼，則要求再次輸入確認：「請再次輸入MYSQL_PASSWORD密碼確認」
    - 如果兩次密碼不一致，顯示錯誤訊息：「✗ 兩次密碼不一致，請重新輸入。」

#### 管理員認證設定

- `ADMIN_LOGIN_USER`：
  - 詢問：「ADMIN_LOGIN_USER (留空自動產生)」
  - 如果留空，使用預設值「example」

- `ADMIN_LOGIN_PASSWORD`：
  - 詢問：「ADMIN_LOGIN_PASSWORD (留空自動產生)」
  - **密碼使用隱碼輸入，不顯示在螢幕上**
  - 如果留空，自動生成 11 位隨機密碼
  - 如果輸入密碼，則要求再次輸入確認：「請再次輸入密碼確認」
  - 如果兩次密碼不一致，顯示錯誤訊息：「✗ 兩次不一致，請重新輸入。」


#### doAsk 小提示

| 步驟     | 互動元件                               | 備註              |
| ------ | ---------------------------------- | --------------- |
| 逐項輸入變數 | `survey.Input` / `survey.Password` | 可自訂驗證器          |
| 密碼二次確認 | 兩次 `survey.Password` 後比對           | 不一致即重新輸入        |

> 對使用者顯示摘要時，記得以 `***` 隱去密碼。

### doRun：寫入、確認 Compose 插件與執行

1. **寫入 `.env`**
    用原有的方式

2. **Caddyfile 設定**

Caddyfile 的設定取決於之前的域名和 SSL 設定：

- 如果用戶選擇使用 SSL（有網域）：
  - 系統將使用 `data/example.Caddyfile` 作為模板
  - 將替換其中的域名和電子郵件設定
  - 使用 `docker-compose-ssl.yaml` 啟動容器
  - Caddy 會自動處理 SSL 證書的獲取和更新

- 如果用戶選擇不使用 SSL：
  - 如果有指定域名但不使用 SSL，依然可能會透過反向代理使用指定域名
  - 如果沒有指定域名，將使用指定的端口（默認 8080）
  - 使用標準的 `docker-compose.yaml` 啟動容器

系統會根據這些設定自動更新 `data/Caddyfile` 檔案，用戶不需要手動編輯。

3. **執行部署**

   ```go
   cmd := exec.Command("docker", "compose", "up", "-d")
   cmd.Stdout = os.Stdout
   cmd.Stderr = os.Stderr
   if err := cmd.Run(); err != nil {
       return fmt.Errorf("執行 docker compose up 失敗: %w", err)
   }
   ```

4. **提示後續操作**

   ```go
   color.New(color.FgCyan).Println("服務已啟動，可使用：docker compose logs -f 查看日誌。")
   ```

---

## 彩色輸出參考

```go
red := color.New(color.FgRed).SprintFunc()
fmt.Printf("%s 未安裝，請先安裝 Docker。\n", red("✗"))
```

---

## Build & Run

```bash
go build -o ./install ./cmd/install
./install        # 或 go run .
```
