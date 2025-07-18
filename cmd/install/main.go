package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

const (
	exampleFile        = "example.env"
	targetFile         = ".env"
	defaultComposeFile = "docker-compose.yaml"
	sslComposeFile     = "docker-compose-ssl.yaml"
	caddyfile          = "data/Caddyfile"
	exampleCaddyfile   = "data/example.Caddyfile"
)

// Config 保存所有配置
type Config struct {
	Language           string
	Domain             string
	Email              string
	UseSSL             bool
	Port               string
	MySQLRootPassword  string
	MySQLDatabase      string
	MySQLUser          string
	MySQLPassword      string
	AdminLoginUser     string
	AdminLoginPassword string
	envVars            map[string]string
}

var (
	green   = color.New(color.FgGreen)
	red     = color.New(color.FgRed)
	yellow  = color.New(color.FgYellow)
	cyan    = color.New(color.FgCyan)
	bold    = color.New(color.Bold)
	gray    = color.New(color.FgHiBlack) // 暗色系用於 debug 訊息
	verbose = false                     // 是否顯示 debug 訊息
)

func main() {
	// 解析命令行參數
	flag.BoolVar(&verbose, "v", false, "顯示詳細的 debug 訊息")
	flag.Parse()

	bold.Println("netiCRM Self-Host 自架站台安裝程式")
	fmt.Println()

	// 檢查階段
	debugPrint("🔍 開始執行 doCheck 階段 - 系統檢查")
	checkResult := doCheck()
	if checkResult != nil {
		// 檢查是否是特殊的流程控制錯誤（實際不是錯誤，而是流程跳轉）
		if checkResult.Error() == "continue_install" {
			// 這表示用戶選擇了"備份並覆蓋設定"，需要繼續安裝流程
			debugPrint("✅ doCheck 階段完成 - 繼續安裝流程")
		} else {
			red.Printf("✗ 檢查失敗: %v\n", checkResult)
			os.Exit(1)
		}
	} else {
		debugPrint("✅ doCheck 階段完成")
	}
	fmt.Println()

	// 詢問階段
	debugPrint("📝 開始執行 doAsk 階段 - 收集設定")
	cfg, err := doAsk()
	if err != nil {
		red.Printf("✗ 設定失敗: %v\n", err)
		os.Exit(1)
	}
	debugPrint("✅ doAsk 階段完成")
	fmt.Println()

	// 執行階段
	debugPrint("🚀 開始執行 doRun 階段 - 執行安裝")
	if err := doRun(cfg); err != nil {
		red.Printf("✗ 執行失敗: %v\n", err)
		os.Exit(1)
	}
	debugPrint("✅ doRun 階段完成")
	fmt.Println()

	green.Println("✅ 安裝完成！")
}

// debugPrint 只在 verbose 模式下輸出 debug 訊息
func debugPrint(message string) {
	if verbose {
		gray.Println(message)
	}
}

// doCheck 進行所有事前檢查
func doCheck() error {
	debugPrint("  🔍 檢查現有檔案...")
	// 檢查是否有 .env 和資料庫檔案
	hasEnv := fileExists(targetFile)
	hasMariaDBData := checkMariaDBData()
	
	if hasEnv {
		debugPrint("  ✓ 發現 .env 檔案")
	}
	if hasMariaDBData {
		debugPrint("  ✓ 發現 MariaDB 資料")
	}

	if hasEnv && hasMariaDBData {
		yellow.Println("發現現有的資料庫檔案，看起來這是一個已經安裝好的網站。")
	} else if hasEnv {
		// 只有 .env 沒有資料庫
		yellow.Println("發現現有的 .env 檔案")
	}

	// 檢查 Docker
	debugPrint("  🐳 檢查 Docker 環境...")
	if err := checkDocker(); err != nil {
		yellow.Printf("⚠️  %v\n", err)

		var proceed bool
		prompt := &survey.Confirm{
			Message: "是否要繼續僅更改 .env 檔案？",
			Default: false,
		}
		if err := survey.AskOne(prompt, &proceed); err != nil {
			return err
		}

		if !proceed {
			fmt.Println("建議先安裝 Docker，安裝取消。")
			os.Exit(0)
		}
	}

	// 讀取並顯示現有 .env 配置資訊
	if hasEnv {
		existingEnv, _ := godotenv.Read(targetFile)
		domain := existingEnv["DOMAIN"]
		port := existingEnv["HTTP_PORT"]
		adminUser := existingEnv["ADMIN_LOGIN_USER"]

		// 如果有 Caddyfile，優先使用其中的域名
		if fileExists(caddyfile) {
			if caddyDomain := getDomainFromCaddyfile(); caddyDomain != "" {
				domain = caddyDomain
			}
		}

		if domain != "" || port != "" || adminUser != "" {
			fmt.Println()
			cyan.Println("現有配置：")
			
			// 檢查 netiCRM 是否運行中
			isRunning := checkNetiCRMRunning()
			if isRunning {
				green.Println("  netiCRM 網站運行中")
			} else {
				red.Println("  netiCRM 網站未運行中")
			}
			
			if domain != "" && domain != "localhost" {
				fmt.Printf("  域名 Domain: %s\n", domain)
			}
			if port != "" {
				fmt.Printf("  端口 Port: %s\n", port)
			}
			if adminUser != "" {
				fmt.Printf("  管理員帳號: %s\n", adminUser)
			}
			
			// 檢查 SSL 配置 - 移動到配置資訊的最後
			debugPrint("  🔐 檢查 SSL 配置...")
			if fileExists(caddyfile) {
				fmt.Println("  有 SSL 憑證")
			} else {
				fmt.Println("  沒有 SSL 憑證")
			}
		}
	}
	
	// 如果沒有 .env 檔案，直接進入初始設定流程
	if !hasEnv {
		debugPrint("  ✓ 沒有 .env 檔案，進入初始設定流程")
		return nil
	}
	
	// 在檢查完 .env 後顯示選擇選單
	fmt.Println()
	options := []string{
		"1. 執行 docker 啟動指令（若已啟動則不影響）",
		"2. 備份網站檔案並覆蓋設定",
		"3. 檢視初始設定管理員密碼 ADMIN_LOGIN_PASSWORD",
		"4. 結束安裝",
	}

	var choice string
	prompt := &survey.Select{
		Message: "請選擇操作（上下鍵選取，或按下數字鍵後 enter）：",
		Options: options,
	}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return err
	}

	switch choice {
	case options[0]: // 執行 docker 啟動指令
		return startDocker()
	case options[1]: // 備份並覆蓋配置
		// 當用戶選擇備份並覆蓋時，詢問是否備份
		if hasEnv && hasMariaDBData {
			if err := backupExisting(); err != nil {
				return err
			}
		} else if hasEnv {
			if err := backupFile(targetFile); err != nil {
				return err
			}
		}
		// 繼續安裝流程，返回特殊錯誤信號
		return fmt.Errorf("continue_install")
	case options[2]: // 檢視密碼
		existingEnv, _ := godotenv.Read(targetFile)
		yellow.Println("⚠️  注意：此會用明文顯示初始密碼，且可能已更改")
		var confirmShow bool
		confirmPrompt := &survey.Confirm{
			Message: "確定要顯示密碼嗎？",
			Default: false,
		}
		if err := survey.AskOne(confirmPrompt, &confirmShow); err != nil {
			return err
		}

		if confirmShow {
			if pass := existingEnv["ADMIN_LOGIN_PASSWORD"]; pass != "" {
				fmt.Printf("ADMIN_LOGIN_PASSWORD: %s\n", pass)
			} else {
				fmt.Println("密碼未設定或為空")
			}
		}
		
		// 顯示按任意鍵繼續的訊息
		fmt.Println()
		cyan.Println("按 Enter 鍵繼續...")
		fmt.Scanln()
		
		// 遞迴調用 doCheck 回到選項選單
		return doCheck()
	case options[3]: // 結束安裝
		fmt.Println("安裝取消。")
		os.Exit(0)
	}

	return nil
}

// doAsk 進行所有互動詢問
func doAsk() (*Config, error) {
	cfg := &Config{
		envVars: make(map[string]string),
	}

	// 載入預設環境變數
	debugPrint("  📄 載入預設環境變數...")
	if err := loadDefaultEnvs(cfg); err != nil {
		return nil, err
	}

	// 1. 語言選擇
	debugPrint("  🌐 詢問語言設定...")
	if err := askLanguage(cfg); err != nil {
		return nil, err
	}

	// 2. 域名和 SSL 設定
	fmt.Println()
	if cfg.Language == "zh-hant" {
		cyan.Println("若您已有域名（Domain），請先將域名以A紀錄設到本主機 IP")
		cyan.Println("本安裝程式可自動幫您設定 SSL 並綁定網域")
		cyan.Println("或依照您所選的設定綁定特定埠（Port）")
	} else {
		cyan.Println("If you already have a domain name, please point it to this host's IP address.")
		cyan.Println("This installer can automatically set up SSL and bind the domain for you,")
		cyan.Println("or bind to a specific port according to your chosen settings.")
	}

	debugPrint("  🌍 詢問域名和 SSL 設定...")
	if err := askDomainAndSSL(cfg); err != nil {
		return nil, err
	}

	// 3. MySQL 設定
	debugPrint("  🗄️ 詢問 MySQL 設定...")
	if err := askMySQL(cfg); err != nil {
		return nil, err
	}

	// 4. 管理員帳號密碼
	debugPrint("  👤 詢問管理員帳號設定...")
	if err := askAdminCredentials(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// doRun 執行寫入和啟動
func doRun(cfg *Config) error {
	debugPrint("  ⚙️ 準備環境變數...")
	// 設定環境變數
	cfg.envVars["LANGUAGE"] = cfg.Language

	if !cfg.UseSSL {
		if cfg.Domain != "" {
			cfg.envVars["DOMAIN"] = cfg.Domain
			cfg.envVars["HTTP_PORT"] = ""
		} else {
			cfg.envVars["DOMAIN"] = "localhost"
			cfg.envVars["HTTP_PORT"] = cfg.Port
		}
	} else {
		cfg.envVars["HTTP_PORT"] = ""
	}

	// MySQL 設定
	cfg.envVars["MYSQL_ROOT_PASSWORD"] = cfg.MySQLRootPassword
	if cfg.MySQLDatabase != "" {
		cfg.envVars["MYSQL_DATABASE"] = cfg.MySQLDatabase
	}
	if cfg.MySQLUser != "" {
		cfg.envVars["MYSQL_USER"] = cfg.MySQLUser
	}
	cfg.envVars["MYSQL_PASSWORD"] = cfg.MySQLPassword

	// 管理員設定
	cfg.envVars["ADMIN_LOGIN_USER"] = cfg.AdminLoginUser
	cfg.envVars["ADMIN_LOGIN_PASSWORD"] = cfg.AdminLoginPassword

	// 寫入 .env
	debugPrint("  📝 寫入 .env 檔案...")
	if err := writeEnvFile(cfg); err != nil {
		return fmt.Errorf("寫入 .env 失敗: %w", err)
	}

	// 更新 Caddyfile
	if cfg.UseSSL {
		debugPrint("  🔐 更新 Caddyfile...")
		if err := updateCaddyfile(cfg); err != nil {
			return fmt.Errorf("更新 Caddyfile 失敗: %w", err)
		}
		
		// 重新啟動 caddy 以更新 SSL 憑證
		debugPrint("  🔄 重新抓取 SSL 憑證...")
		if err := refreshSSLCertificate(); err != nil {
			yellow.Printf("⚠️  SSL 憑證重新抓取失敗: %v\n", err)
		}
	}

	// 選擇 compose 檔案
	composeFile := defaultComposeFile
	if cfg.UseSSL {
		composeFile = sslComposeFile
	}

	green.Printf("✅ .env 建立完成\n")

	// 檢查是否有 Docker
	debugPrint("  🐳 檢查 Docker 環境...")
	if err := checkDocker(); err != nil {
		yellow.Println("Docker Compose 未安裝，請手動執行：")
		fmt.Printf("docker compose -f %s up -d\n", composeFile)
		return nil
	}

	// 執行 docker compose
	debugPrint(fmt.Sprintf("  🚀 啟動 Docker 容器 (%s)...", composeFile))
	fmt.Printf("開始執行 docker compose -f %s up -d ...\n", composeFile)
	if err := dockerComposeUp(composeFile); err != nil {
		return err
	}

	cyan.Println("服務已啟動，可使用以下指令查看日誌：")
	fmt.Printf("docker compose -f %s logs -f\n", composeFile)

	return nil
}

// 輔助函數

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func checkMariaDBData() bool {
	mariadbPath := "data/mariadb_data"
	info, err := os.Stat(mariadbPath)
	if os.IsNotExist(err) || !info.IsDir() {
		return false
	}

	files, err := os.ReadDir(mariadbPath)
	return err == nil && len(files) > 0
}

func getDomainFromCaddyfile() string {
	data, err := os.ReadFile(caddyfile)
	if err != nil {
		return ""
	}

	// 使用正則表達式尋找域名
	// 支援多種格式：
	// - example.com {
	// - https://example.com {
	// - example.com:443 {
	// - example.com, www.example.com {
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳過註解和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 尋找包含 { 的行
		if strings.Contains(line, "{") {
			// 提取 { 之前的部分
			parts := strings.Split(line, "{")
			if len(parts) > 0 {
				domainPart := strings.TrimSpace(parts[0])
				// 移除協議前綴
				domainPart = strings.TrimPrefix(domainPart, "https://")
				domainPart = strings.TrimPrefix(domainPart, "http://")
				// 移除端口
				if idx := strings.Index(domainPart, ":"); idx != -1 {
					domainPart = domainPart[:idx]
				}
				// 如果有多個域名（逗號分隔），取第一個
				if strings.Contains(domainPart, ",") {
					domains := strings.Split(domainPart, ",")
					domainPart = strings.TrimSpace(domains[0])
				}
				// 驗證是否為有效域名
				if domainPart != "" && strings.Contains(domainPart, ".") {
					return domainPart
				}
			}
		}
	}

	return ""
}

// getAllDomainsFromCaddyfile 取得 Caddyfile 中的所有域名
func getAllDomainsFromCaddyfile() []string {
	data, err := os.ReadFile(caddyfile)
	if err != nil {
		return nil
	}

	var domains []string
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳過註解和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 尋找包含 { 的行
		if strings.Contains(line, "{") {
			// 提取 { 之前的部分
			parts := strings.Split(line, "{")
			if len(parts) > 0 {
				domainPart := strings.TrimSpace(parts[0])
				// 移除協議前綴
				domainPart = strings.TrimPrefix(domainPart, "https://")
				domainPart = strings.TrimPrefix(domainPart, "http://")
				// 移除端口
				if idx := strings.Index(domainPart, ":"); idx != -1 {
					domainPart = domainPart[:idx]
				}
				// 如果有多個域名（逗號分隔），分別加入
				if strings.Contains(domainPart, ",") {
					domainList := strings.Split(domainPart, ",")
					for _, d := range domainList {
						d = strings.TrimSpace(d)
						if d != "" && strings.Contains(d, ".") {
							domains = append(domains, d)
						}
					}
				} else {
					// 驗證是否為有效域名
					if domainPart != "" && strings.Contains(domainPart, ".") {
						domains = append(domains, domainPart)
					}
				}
			}
		}
	}

	return domains
}

func backupFile(path string) error {
	backupPath := path + ".bak"
	count := 0

	for fileExists(backupPath) {
		count++
		backupPath = fmt.Sprintf("%s.bak%d", path, count)
	}

	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("無法備份 %s: %v", path, err)
	}

	green.Printf("已將 %s 備份為 %s\n", path, backupPath)
	return nil
}

func backupExisting() error {
	// 備份 .env
	if err := backupFile(targetFile); err != nil {
		return err
	}

	// 詢問是否備份資料庫
	if checkMariaDBData() {
		var backupDB bool
		prompt := &survey.Confirm{
			Message: "是否要備份資料庫、網站檔案（data/mariadb_data、data/www 資料夾）？",
			Default: true,
		}
		if err := survey.AskOne(prompt, &backupDB); err != nil {
			return err
		}

		if backupDB {
			if err := backupFile("data/mariadb_data"); err != nil {
				return err
			}

			// 同時備份 data/www
			if fileExists("data/www") {
				if err := backupFile("data/www"); err != nil {
					yellow.Printf("警告: 無法備份 data/www: %v\n", err)
				}
			}
		}
	}

	return nil
}

func startDocker() error {
	hasCaddyfile := fileExists(caddyfile)

	var composeFile string
	if hasCaddyfile {
		cyan.Println("使用 SSL 配置啟動...")
		composeFile = sslComposeFile
	} else {
		cyan.Println("使用非 SSL 配置啟動...")
		composeFile = defaultComposeFile
	}

	return dockerComposeUp(composeFile)
}

func checkDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("Docker 未安裝")
	}

	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		return fmt.Errorf("Docker Compose 插件未安裝或未啟用")
	}

	return nil
}

func dockerComposeUp(composeFile string) error {
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("執行 docker compose up 失敗: %w", err)
	}

	green.Println("網站已成功啟動！")
	os.Exit(0)
	return nil
}

func loadDefaultEnvs(cfg *Config) error {
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		return fmt.Errorf("讀取 %s 失敗: %w", exampleFile, err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			cfg.envVars[key] = val
		}
	}

	return nil
}

func askLanguage(cfg *Config) error {
	prompt := &survey.Select{
		Message: "What's your language? / 請選擇語言（上下鍵選取，或按下數字鍵後 enter）：",
		Options: []string{"1. English", "2. Taiwan Traditional Chinese 台灣繁體中文"},
	}

	var choice string
	if err := survey.AskOne(prompt, &choice); err != nil {
		return err
	}

	if choice == "1. English" {
		cfg.Language = "en"
	} else {
		cfg.Language = "zh-hant"
	}

	return nil
}

func askDomainAndSSL(cfg *Config) error {
	// 檢查是否已有 Caddyfile
	hasCaddyfile := fileExists(caddyfile)
	
	// SSL 詢問
	sslPrompt := "Do you have a domain and want to set up SSL automatically?"
	if cfg.Language == "zh-hant" {
		sslPrompt = "您是否有網域並希望自動設定 SSL？"
	}

	var useSSL bool
	prompt := &survey.Confirm{
		Message: sslPrompt,
		Default: false,
	}
	if err := survey.AskOne(prompt, &useSSL); err != nil {
		return err
	}
	cfg.UseSSL = useSSL

	if useSSL {
		// 如果已有 Caddyfile，詢問是否要修改
		if hasCaddyfile {
			existingDomains := getAllDomainsFromCaddyfile()
			if len(existingDomains) > 0 {
				var modifyPrompt string
				if cfg.Language == "zh-hant" {
					modifyPrompt = fmt.Sprintf("發現現有的 Caddyfile 包含域名：%s\n是否要修改 Caddyfile？", strings.Join(existingDomains, ", "))
				} else {
					modifyPrompt = fmt.Sprintf("Found existing Caddyfile with domains: %s\nDo you want to modify the Caddyfile?", strings.Join(existingDomains, ", "))
				}
				
				var modifyCaddyfile bool
				modifyConfirm := &survey.Confirm{
					Message: modifyPrompt,
					Default: false,
				}
				if err := survey.AskOne(modifyConfirm, &modifyCaddyfile); err != nil {
					return err
				}
				
				if !modifyCaddyfile {
					if cfg.Language == "zh-hant" {
						fmt.Println("保留現有的 Caddyfile 設定。")
					} else {
						fmt.Println("Keeping existing Caddyfile configuration.")
					}
					return nil
				}
			}
		}

		// SSL 路徑
		domainPrompt := "Please enter your domain name (e.g., example.com):"
		emailPrompt := "Please enter your email (for Let's Encrypt SSL certificate):"
		if cfg.Language == "zh-hant" {
			domainPrompt = "請輸入您的域名 (例如 example.com)："
			emailPrompt = "請輸入您的電子郵件 (用於 Let's Encrypt SSL 證書)："
		}

		// 域名
		domainInput := &survey.Input{
			Message: domainPrompt,
		}
		if err := survey.AskOne(domainInput, &cfg.Domain, survey.WithValidator(survey.Required)); err != nil {
			return err
		}

		// Email
		emailInput := &survey.Input{
			Message: emailPrompt,
		}
		if err := survey.AskOne(emailInput, &cfg.Email); err != nil {
			return err
		}
	} else {
		// 非 SSL 路徑
		domainPrompt := "Domain (leave blank for no domain):"
		if cfg.Language == "zh-hant" {
			domainPrompt = "網站網址 (domain，可留空)："
		}

		domainInput := &survey.Input{
			Message: domainPrompt,
		}
		if err := survey.AskOne(domainInput, &cfg.Domain); err != nil {
			return err
		}

		if cfg.Domain == "" {
			portPrompt := "Please enter Port (default 8080):"
			if cfg.Language == "zh-hant" {
				portPrompt = "請輸入 Port (預設 8080)："
			}

			portInput := &survey.Input{
				Message: portPrompt,
				Default: "8080",
			}
			if err := survey.AskOne(portInput, &cfg.Port); err != nil {
				return err
			}
		}
	}

	return nil
}

func askMySQL(cfg *Config) error {
	modifyPrompt := "Modify MySQL parameters?"
	if cfg.Language == "zh-hant" {
		modifyPrompt = "是否要修改 MySQL 參數？"
	}

	var modify bool
	prompt := &survey.Confirm{
		Message: modifyPrompt,
		Default: false,
	}
	if err := survey.AskOne(prompt, &modify); err != nil {
		return err
	}

	if !modify {
		// 自動產生密碼
		cfg.MySQLRootPassword = randomPass(13)
		cfg.MySQLPassword = randomPass(13)
		// Database 和 User 保留預設值
		return nil
	}

	// ROOT 密碼
	if err := askPasswordWithConfirm(cfg, "MYSQL_ROOT_PASSWORD", &cfg.MySQLRootPassword, 13); err != nil {
		return err
	}

	// Database
	dbPrompt := "MYSQL_DATABASE (leave blank for default):"
	if cfg.Language == "zh-hant" {
		dbPrompt = "MYSQL_DATABASE (留空使用預設值)："
	}
	dbInput := &survey.Input{
		Message: dbPrompt,
	}
	if err := survey.AskOne(dbInput, &cfg.MySQLDatabase); err != nil {
		return err
	}

	// User
	userPrompt := "MYSQL_USER (leave blank for default):"
	if cfg.Language == "zh-hant" {
		userPrompt = "MYSQL_USER (留空使用預設值)："
	}
	userInput := &survey.Input{
		Message: userPrompt,
	}
	if err := survey.AskOne(userInput, &cfg.MySQLUser); err != nil {
		return err
	}

	// User 密碼
	if err := askPasswordWithConfirm(cfg, "MYSQL_PASSWORD", &cfg.MySQLPassword, 13); err != nil {
		return err
	}

	return nil
}

func askAdminCredentials(cfg *Config) error {
	// Username
	userPrompt := "ADMIN_LOGIN_USER (leave blank for 'admin'):"
	if cfg.Language == "zh-hant" {
		userPrompt = "ADMIN_LOGIN_USER (留空使用 'admin')："
	}

	userInput := &survey.Input{
		Message: userPrompt,
		Default: "admin",
	}
	if err := survey.AskOne(userInput, &cfg.AdminLoginUser); err != nil {
		return err
	}

	// Password
	if err := askPasswordWithConfirm(cfg, "ADMIN_LOGIN_PASSWORD", &cfg.AdminLoginPassword, 11); err != nil {
		return err
	}

	return nil
}

func askPasswordWithConfirm(cfg *Config, field string, target *string, defaultLen int) error {
	passPrompt := fmt.Sprintf("%s (leave blank for random password):", field)
	confirmPrompt := fmt.Sprintf("Please re-enter %s to confirm:", field)
	mismatchMsg := "✗ Passwords do not match. Please re-enter."

	if cfg.Language == "zh-hant" {
		passPrompt = fmt.Sprintf("%s (留空自動產生)：", field)
		confirmPrompt = fmt.Sprintf("請再次輸入%s密碼確認：", field)
		mismatchMsg = "✗ 兩次密碼不一致，請重新輸入。"
	}

	for {
		var password string
		passwordInput := &survey.Password{
			Message: passPrompt,
		}
		if err := survey.AskOne(passwordInput, &password); err != nil {
			return err
		}

		if password == "" {
			*target = randomPass(defaultLen)
			return nil
		}

		var confirm string
		confirmInput := &survey.Password{
			Message: confirmPrompt,
		}
		if err := survey.AskOne(confirmInput, &confirm); err != nil {
			return err
		}

		if password == confirm {
			*target = password
			return nil
		}

		red.Println(mismatchMsg)
	}
}

func randomPass(length int) string {
	// 定義字符集
	const (
		lowercase = "abcdefghijklmnopqrstuvwxyz"
		uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digits    = "0123456789"
		symbols   = "!@#$%^&*"
	)
	allChars := lowercase + uppercase + digits + symbols

	// 確保密碼包含各種字符
	var password strings.Builder

	// 至少包含一個小寫字母
	password.WriteByte(lowercase[randInt(len(lowercase))])
	// 至少包含一個大寫字母
	password.WriteByte(uppercase[randInt(len(uppercase))])
	// 至少包含一個數字
	password.WriteByte(digits[randInt(len(digits))])
	// 至少包含一個符號
	password.WriteByte(symbols[randInt(len(symbols))])

	// 填充剩餘長度
	for i := 4; i < length; i++ {
		password.WriteByte(allChars[randInt(len(allChars))])
	}

	// 打亂密碼順序
	runes := []rune(password.String())
	for i := len(runes) - 1; i > 0; i-- {
		j := randInt(i + 1)
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}

func randInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

func writeEnvFile(cfg *Config) error {
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		// 如果無法讀取範例檔，直接寫入
		var lines []string
		for key, val := range cfg.envVars {
			lines = append(lines, fmt.Sprintf("%s=\"%s\"", key, val))
		}
		content := strings.Join(lines, "\n") + "\n"
		return os.WriteFile(targetFile, []byte(content), 0644)
	}

	// 基於範例檔案更新
	lines := strings.Split(string(data), "\n")
	var newContent strings.Builder
	written := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 保留註解和空行
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			fmt.Fprintln(&newContent, line)
			continue
		}

		// 處理環境變數
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])

			if val, ok := cfg.envVars[key]; ok && val != "" {
				fmt.Fprintf(&newContent, "%s=\"%s\"\n", key, val)
				written[key] = true
			} else {
				fmt.Fprintln(&newContent, line)
			}
		} else {
			fmt.Fprintln(&newContent, line)
		}
	}

	// 添加未寫入的變數
	for key, val := range cfg.envVars {
		if !written[key] && val != "" {
			fmt.Fprintf(&newContent, "%s=\"%s\"\n", key, val)
		}
	}

	return os.WriteFile(targetFile, []byte(newContent.String()), 0644)
}

func updateCaddyfile(cfg *Config) error {
	// 檢查 example.Caddyfile 是否存在
	if !fileExists(exampleCaddyfile) {
		return fmt.Errorf("%s 不存在", exampleCaddyfile)
	}

	// 如果 Caddyfile 已存在，先備份
	if fileExists(caddyfile) {
		if err := backupFile(caddyfile); err != nil {
			return err
		}
	}

	// 讀取範例檔案
	data, err := os.ReadFile(exampleCaddyfile)
	if err != nil {
		return err
	}

	// 檢查是否有現有的域名需要保留
	var existingDomains []string
	if fileExists(caddyfile + ".bak") {
		existingDomains = getAllDomainsFromBackupCaddyfile(caddyfile + ".bak")
	}

	// 建立新的 Caddyfile 內容
	content := string(data)
	
	// 準備域名列表
	var allDomains []string
	for _, domain := range existingDomains {
		// 確保不重複新增現有域名
		if domain != cfg.Domain {
			allDomains = append(allDomains, domain)
		}
	}
	// 加入新域名
	allDomains = append(allDomains, cfg.Domain)
	
	// 建立域名字串
	domainString := strings.Join(allDomains, " , ")
	
	// 替換內容
	content = strings.ReplaceAll(content, "your.domain.name", domainString)
	if cfg.Email != "" {
		content = strings.ReplaceAll(content, "your-email@domain.com", cfg.Email)
	}

	// 確保 data 目錄存在
	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("無法建立 data 目錄: %w", err)
	}

	// 寫入檔案
	if err := os.WriteFile(caddyfile, []byte(content), 0644); err != nil {
		return err
	}

	green.Printf("✅ Caddyfile 已更新\n")
	return nil
}

// getAllDomainsFromBackupCaddyfile 從備份的 Caddyfile 中獲取域名
func getAllDomainsFromBackupCaddyfile(backupFile string) []string {
	data, err := os.ReadFile(backupFile)
	if err != nil {
		return nil
	}

	var domains []string
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳過註解和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 尋找包含 { 的行
		if strings.Contains(line, "{") {
			// 提取 { 之前的部分
			parts := strings.Split(line, "{")
			if len(parts) > 0 {
				domainPart := strings.TrimSpace(parts[0])
				// 移除協議前綴
				domainPart = strings.TrimPrefix(domainPart, "https://")
				domainPart = strings.TrimPrefix(domainPart, "http://")
				// 移除端口
				if idx := strings.Index(domainPart, ":"); idx != -1 {
					domainPart = domainPart[:idx]
				}
				// 如果有多個域名（逗號分隔），分別加入
				if strings.Contains(domainPart, ",") {
					domainList := strings.Split(domainPart, ",")
					for _, d := range domainList {
						d = strings.TrimSpace(d)
						if d != "" && strings.Contains(d, ".") {
							domains = append(domains, d)
						}
					}
				} else {
					// 驗證是否為有效域名
					if domainPart != "" && strings.Contains(domainPart, ".") {
						domains = append(domains, domainPart)
					}
				}
			}
		}
	}

	return domains
}

// checkNetiCRMRunning 檢查 netiCRM 容器是否在運行
func checkNetiCRMRunning() bool {
	// 檢查是否有 Docker
	if err := checkDocker(); err != nil {
		return false
	}

	// 使用 docker compose ps 檢查容器狀態
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// 如果有任何輸出，說明有容器在運行
	return len(output) > 0 && string(output) != "[]\n"
}

// refreshSSLCertificate 重新抓取 SSL 憑證
func refreshSSLCertificate() error {
	// 檢查是否有 Docker
	if err := checkDocker(); err != nil {
		return err
	}

	green.Println("正在重新抓取 SSL 憑證...")
	
	// 執行 docker compose -f docker-compose-ssl.yaml up -d --force-recreate caddy
	cmd := exec.Command("docker", "compose", "-f", sslComposeFile, "up", "-d", "--force-recreate", "caddy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("執行 caddy 重新啟動失敗: %w", err)
	}

	green.Println("✅ SSL 憑證重新抓取完成")
	return nil
}

// showExistingConfigOptions 顯示現有配置的選項菜單
func showExistingConfigOptions(existingEnv map[string]string) error {
	domain := existingEnv["DOMAIN"]
	port := existingEnv["HTTP_PORT"]
	adminUser := existingEnv["ADMIN_LOGIN_USER"]

	// 如果有 Caddyfile，嘗試從中獲取域名
	if fileExists(caddyfile) {
		if caddyDomain := getDomainFromCaddyfile(); caddyDomain != "" {
			domain = caddyDomain
		}
	}

	fmt.Println()
	cyan.Println("現有配置：")
	if domain != "" && domain != "localhost" {
		fmt.Printf("  域名 Domain: %s\n", domain)
	}
	if port != "" {
		fmt.Printf("  端口 Port: %s\n", port)
	}
	if adminUser != "" {
		fmt.Printf("  管理員帳號: %s\n", adminUser)
	}
	fmt.Println()

	options := []string{
		"1. 執行 docker 啟動指令（若已啟動則不影響）",
		"2. 備份網站檔案並覆蓋設定",
		"3. 檢視初始設定管理員密碼 ADMIN_LOGIN_PASSWORD",
		"4. 結束安裝",
	}

	var choice string
	prompt := &survey.Select{
		Message: "請選擇操作（上下鍵選取，或按下數字鍵後 enter）：",
		Options: options,
	}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return err
	}

	switch choice {
	case options[0]: // 執行 docker 啟動指令
		return startDocker()
	case options[1]: // 備份並覆蓋配置
		if err := backupExisting(); err != nil {
			return err
		}
		// 繼續安裝流程，返回特殊錯誤信號
		return fmt.Errorf("continue_install")
	case options[2]: // 檢視密碼
		yellow.Println("⚠️  注意：此會用明文顯示初始密碼，且可能已更改")
		var confirmShow bool
		confirmPrompt := &survey.Confirm{
			Message: "確定要顯示密碼嗎？",
			Default: false,
		}
		if err := survey.AskOne(confirmPrompt, &confirmShow); err != nil {
			return err
		}

		if confirmShow {
			if pass := existingEnv["ADMIN_LOGIN_PASSWORD"]; pass != "" {
				fmt.Printf("ADMIN_LOGIN_PASSWORD: %s\n", pass)
			} else {
				fmt.Println("密碼未設定或為空")
			}
		}
		os.Exit(0)
	case options[3]: // 結束安裝
		fmt.Println("安裝取消。")
		os.Exit(0)
	}
	
	return nil
}
