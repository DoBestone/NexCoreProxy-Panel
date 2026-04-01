// Package install provides the built-in installation command for NexCoreProxy Panel.
// Usage: x-ui install --port 54321 --user admin --pass MyPass --api-port 54322 --gen-token
package install

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"nexcoreproxy-panel/config"
	"nexcoreproxy-panel/database"
	"nexcoreproxy-panel/ncp/agent"
	"nexcoreproxy-panel/web/service"
)

// Run executes the install subcommand.
func Run(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)

	var (
		port     int
		username string
		password string
		apiPort  int
		genToken bool
		help     bool
	)

	fs.IntVar(&port, "port", 0, "面板端口 (1-65535)")
	fs.StringVar(&username, "user", "", "管理员用户名")
	fs.StringVar(&password, "pass", "", "管理员密码")
	fs.IntVar(&apiPort, "api-port", 54322, "NCP API 端口")
	fs.BoolVar(&genToken, "gen-token", false, "生成 API Token")
	fs.BoolVar(&help, "help", false, "显示帮助")

	fs.Parse(args)

	if help {
		fmt.Println("NexCoreProxy Panel 安装配置")
		fmt.Println()
		fmt.Println("用法: x-ui install [选项]")
		fmt.Println()
		fs.PrintDefaults()
		return
	}

	// Initialize database
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		fmt.Printf("数据库初始化失败: %v\n", err)
		os.Exit(1)
	}

	settingService := service.SettingService{}
	userService := service.UserService{}

	configured := false

	// Set port
	if port > 0 {
		if port < 1 || port > 65535 {
			fmt.Println("错误: 端口必须在 1-65535 之间")
			os.Exit(1)
		}
		if err := settingService.SetPort(port); err != nil {
			fmt.Printf("设置端口失败: %v\n", err)
		} else {
			fmt.Printf("面板端口: %d\n", port)
			configured = true
		}
	}

	// Set credentials
	if username != "" || password != "" {
		if err := userService.UpdateFirstUser(username, password); err != nil {
			fmt.Printf("设置账号失败: %v\n", err)
		} else {
			if username != "" {
				fmt.Printf("管理员用户名: %s\n", username)
			}
			if password != "" {
				fmt.Println("管理员密码: 已设置")
			}
			configured = true
		}
	}

	// Set NCP API port
	db := database.GetDB()
	db.Exec("INSERT OR REPLACE INTO settings (`key`, value) VALUES ('ncpAPIPort', ?)", strconv.Itoa(apiPort))
	fmt.Printf("API 端口: %d\n", apiPort)

	// Generate token
	if genToken {
		agent.CmdGenToken()
		configured = true
	}

	// Install systemd service
	installSystemdService()

	if configured {
		fmt.Println()
		fmt.Println("安装配置完成！")
		fmt.Println("启动服务: systemctl start x-ui")
		fmt.Println("查看状态: systemctl status x-ui")
	}
}

func installSystemdService() {
	serviceContent := `[Unit]
Description=NexCoreProxy Panel
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/x-ui/x-ui
WorkingDirectory=/usr/local/x-ui/
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`
	err := os.WriteFile("/etc/systemd/system/x-ui.service", []byte(serviceContent), 0644)
	if err != nil {
		// Not fatal — might not have permissions during dev
		return
	}
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "x-ui").Run()
}
