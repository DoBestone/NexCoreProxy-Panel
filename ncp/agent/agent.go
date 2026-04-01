// Package agent provides CLI management commands for NexCoreProxy panel.
// It operates on the panel's SQLite database via GORM.
package agent

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"nexcoreproxy-panel/database"
)

const (
	InstallDir = "/usr/local/x-ui"
	TokenFile  = InstallDir + "/API_TOKEN"
)

// Run dispatches CLI subcommands. Called when binary is invoked as `ncp-agent`
// or via `x-ui ncp <subcommand>`.
func Run(args []string) {
	if len(args) == 0 {
		showHelp()
		os.Exit(1)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "status":
		cmdStatus()
	case "info":
		cmdInfo()
	case "restart":
		cmdRestart()
	case "restart-xray":
		cmdRestartXray()
	case "set-port":
		requireArg(rest, "请指定端口")
		cmdSetPort(rest[0])
	case "set-user":
		requireArg(rest, "请指定用户名")
		cmdSetUser(rest[0])
	case "set-pass":
		requireArg(rest, "请指定密码")
		cmdSetPass(rest[0])
	case "get-port":
		fmt.Println(GetPort())
	case "get-user":
		fmt.Println(GetUsername())
	case "list-inbounds":
		cmdListInbounds()
	case "del-inbound":
		requireArg(rest, "请指定入站ID")
		cmdDelInbound(rest[0])
	case "enable-inbound":
		requireArg(rest, "请指定入站ID")
		cmdToggleInbound(rest[0], true)
	case "disable-inbound":
		requireArg(rest, "请指定入站ID")
		cmdToggleInbound(rest[0], false)
	case "gen-token":
		CmdGenToken()
	case "get-token":
		CmdGetToken()
	case "reset-token":
		CmdGenToken()
	case "version":
		cmdVersion()
	default:
		showHelp()
		os.Exit(1)
	}
}

func requireArg(args []string, msg string) {
	if len(args) < 1 {
		fmt.Println("错误:", msg)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("NexCoreProxy Panel 管理工具")
	fmt.Println()
	fmt.Println("用法: ncp-agent <命令> [参数]")
	fmt.Println("  或: x-ui ncp <命令> [参数]")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  status              查看服务状态")
	fmt.Println("  info                查看面板信息")
	fmt.Println("  restart             重启面板服务")
	fmt.Println("  restart-xray        重启 Xray")
	fmt.Println("  set-port <端口>      设置面板端口")
	fmt.Println("  set-user <用户名>    设置管理员用户名")
	fmt.Println("  set-pass <密码>      设置管理员密码")
	fmt.Println("  get-port            获取面板端口")
	fmt.Println("  get-user            获取管理员用户名")
	fmt.Println("  list-inbounds       列出所有入站")
	fmt.Println("  del-inbound <id>    删除入站")
	fmt.Println("  enable-inbound <id> 启用入站")
	fmt.Println("  disable-inbound <id> 禁用入站")
	fmt.Println("  gen-token           生成 API Token")
	fmt.Println("  get-token           查看 API Token")
	fmt.Println("  reset-token         重置 API Token")
	fmt.Println("  version             查看版本")
}

// ========== Commands ==========

func cmdStatus() {
	if isServiceActive() {
		fmt.Println("状态: 运行中")
		fmt.Printf("端口: %s\n", GetPort())
		fmt.Printf("用户: %s\n", GetUsername())
	} else {
		fmt.Println("状态: 未运行")
	}
}

func cmdInfo() {
	fmt.Println("=== NexCoreProxy Panel 信息 ===")
	fmt.Println()
	fmt.Printf("版本: %s\n", getVersion())
	status := "未运行"
	if isServiceActive() {
		status = "运行中"
	}
	fmt.Printf("服务状态: %s\n", status)
	fmt.Printf("面板端口: %s\n", GetPort())
	fmt.Printf("管理员: %s\n", GetUsername())
	fmt.Println()
	fmt.Printf("安装目录: %s\n", InstallDir)
}

func cmdRestart() {
	exec.Command("systemctl", "restart", "x-ui").Run()
	fmt.Println("面板重启成功")
}

func cmdRestartXray() {
	exec.Command("pkill", "-f", "xray-linux").Run()
	fmt.Println("Xray 已重启")
}

func cmdSetPort(port string) {
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		fmt.Println("错误: 端口必须是 1-65535 的数字")
		return
	}
	db := database.GetDB()
	db.Exec("UPDATE settings SET value = ? WHERE `key` = 'webPort'", port)
	fmt.Printf("端口已设置为: %s\n", port)
	fmt.Println("需要重启服务生效: ncp-agent restart")
}

func cmdSetUser(user string) {
	db := database.GetDB()
	db.Exec("UPDATE settings SET value = ? WHERE `key` = 'webUsername'", user)
	fmt.Printf("用户名已设置为: %s\n", user)
}

func cmdSetPass(pass string) {
	db := database.GetDB()
	db.Exec("UPDATE settings SET value = ? WHERE `key` = 'webPassword'", pass)
	fmt.Println("密码已设置")
}

func cmdListInbounds() {
	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Println("错误: 数据库未连接")
		return
	}

	fmt.Println("ID  | 端口  | 协议      | 启用 | 备注")
	fmt.Println("----|-------|-----------|------|------")

	rows, err := sqlDB.Query("SELECT id, port, protocol, enable, remark FROM inbounds")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, port int
		var protocol, remark string
		var enable bool
		if err := rows.Scan(&id, &port, &protocol, &enable, &remark); err != nil {
			continue
		}
		enableStr := "是"
		if !enable {
			enableStr = "否"
		}
		fmt.Printf("%-3d | %-5d | %-9s | %-4s | %s\n", id, port, protocol, enableStr, remark)
	}
}

func cmdDelInbound(id string) {
	if _, err := strconv.Atoi(id); err != nil {
		fmt.Println("错误: ID 必须是数字")
		return
	}
	db := database.GetDB()
	db.Exec("DELETE FROM inbounds WHERE id = ?", id)
	fmt.Printf("入站 %s 已删除\n", id)
}

func cmdToggleInbound(id string, enable bool) {
	if _, err := strconv.Atoi(id); err != nil {
		fmt.Println("错误: ID 必须是数字")
		return
	}
	enableInt := 0
	if enable {
		enableInt = 1
	}
	db := database.GetDB()
	db.Exec("UPDATE inbounds SET enable = ? WHERE id = ?", enableInt, id)
	action := "启用"
	if !enable {
		action = "禁用"
	}
	fmt.Printf("入站 %s 已%s\n", id, action)
}

func cmdVersion() {
	fmt.Printf("NexCoreProxy Panel: %s\n", getVersion())
}

// ========== Token Management ==========

// CmdGenToken generates a new API token and stores it.
func CmdGenToken() {
	token := GenerateRandomString(32)
	// Store in both DB and file for backward compatibility
	db := database.GetDB()
	db.Exec("INSERT OR REPLACE INTO settings (`key`, value) VALUES ('ncpAPIToken', ?)", token)
	os.WriteFile(TokenFile, []byte(token), 0600)
	fmt.Printf("API Token 已生成: %s\n", token)
}

// CmdGetToken prints the current API token.
func CmdGetToken() {
	token := GetToken()
	if token == "" {
		fmt.Println("未生成 API Token，请执行: ncp-agent gen-token")
		return
	}
	fmt.Println(token)
}

// GetToken returns the current API token from DB or file.
func GetToken() string {
	db := database.GetDB()
	var token string
	db.Raw("SELECT value FROM settings WHERE `key` = 'ncpAPIToken'").Scan(&token)
	if token != "" {
		return token
	}
	// Fallback: read from file (migration support)
	data, err := os.ReadFile(TokenFile)
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(string(data))
	if t != "" {
		// Migrate to DB
		db.Exec("INSERT OR REPLACE INTO settings (`key`, value) VALUES ('ncpAPIToken', ?)", t)
	}
	return t
}

// ========== Helpers ==========

// GetPort returns the panel web port.
func GetPort() string {
	db := database.GetDB()
	var port string
	db.Raw("SELECT value FROM settings WHERE `key` = 'webPort'").Scan(&port)
	if port == "" {
		return "54321"
	}
	return port
}

// GetUsername returns the admin username.
func GetUsername() string {
	db := database.GetDB()
	var user string
	db.Raw("SELECT value FROM settings WHERE `key` = 'webUsername'").Scan(&user)
	if user == "" {
		return "admin"
	}
	return user
}

func getVersion() string {
	data, err := os.ReadFile(InstallDir + "/VERSION")
	if err != nil {
		return "未知"
	}
	return strings.TrimSpace(string(data))
}

func isServiceActive() bool {
	out, _ := exec.Command("systemctl", "is-active", "x-ui").Output()
	return strings.TrimSpace(string(out)) == "active"
}

// GenerateRandomString generates a cryptographically secure random string.
func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			b[i] = charset[0]
			continue
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
