// Package api provides the NexCoreProxy management REST API server.
// It runs on a separate port from the panel and uses token-based auth.
// Response format matches Master's AgentAPIStatus/AgentAPIInbounds structs.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"nexcoreproxy-panel/ncp/agent"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// bcryptHash generates a bcrypt hash for the password.
func bcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Server is the NCP API HTTP server.
type Server struct {
	db         *gorm.DB
	port       int
	httpServer *http.Server
}

// NewServer creates a new NCP API server.
func NewServer(db *gorm.DB, port int) *Server {
	return &Server{db: db, port: port}
}

// Start starts the API server in a goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/info", s.auth(s.handleInfo))
	mux.HandleFunc("/api/inbounds", s.auth(s.handleInbounds))
	mux.HandleFunc("/api/inbound/", s.auth(s.handleInbound))
	mux.HandleFunc("/api/clients/", s.auth(s.handleClients))
	mux.HandleFunc("/api/restart", s.auth(s.handleRestart))
	mux.HandleFunc("/api/restart-xray", s.auth(s.handleRestartXray))
	mux.HandleFunc("/api/settings", s.auth(s.handleSettings))
	mux.HandleFunc("/api/xray-config", s.auth(s.handleXrayConfig))
	mux.HandleFunc("/api/relay-outbound", s.auth(s.handleRelayOutbound))

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("NCP API 启动，端口: %d", s.port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("NCP API 错误: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the API server.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// ========== Middleware ==========

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := agent.GetToken()
		if token == "" {
			jsonResp(w, map[string]any{"success": false, "msg": "Token not configured"})
			return
		}
		reqToken := r.Header.Get("X-API-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(reqToken)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			jsonResp(w, map[string]any{"success": false, "msg": "Invalid token"})
			return
		}
		next(w, r)
	}
}

func jsonResp(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ========== Handlers ==========

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"xrayVersion":   s.getXrayVersion(),
		"cpu":           GetCPU(),
		"mem":           GetMemory(),
		"disk":          GetDisk(),
		"uptime":        GetUptime(),
		"uploadTotal":   s.getTotalTraffic("up"),
		"downloadTotal": s.getTotalTraffic("down"),
	}
	jsonResp(w, map[string]any{"success": true, "data": status})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"panel_port":   agent.GetPort(),
		"admin_user":   agent.GetUsername(),
		"api_port":     s.port,
		"service":      s.getServiceStatus(),
		"xray":         s.getXrayStatus(),
		"xray_version": s.getXrayVersion(),
		"inbounds":     s.getInboundCount(),
		"total_up":     s.getTotalTraffic("up"),
		"total_down":   s.getTotalTraffic("down"),
	}
	jsonResp(w, map[string]any{"success": true, "data": info})
}

func (s *Server) handleInbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": err.Error()})
		return
	}
	rows, err := sqlDB.Query("SELECT id, port, protocol, enable, up, down, remark, tag, settings FROM inbounds")
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": err.Error()})
		return
	}
	defer rows.Close()

	inbounds := []map[string]any{}
	for rows.Next() {
		var id, port int
		var protocol, remark, tag, settings string
		var enable bool
		var up, down int64
		if err := rows.Scan(&id, &port, &protocol, &enable, &up, &down, &remark, &tag, &settings); err != nil {
			continue
		}
		// 计算客户端数量
		totalClient := countClients(settings)
		inbounds = append(inbounds, map[string]any{
			"id": id, "port": port, "protocol": protocol,
			"enable": enable, "up": up, "down": down,
			"remark": remark, "tag": tag, "totalClient": totalClient,
		})
	}
	if err := rows.Err(); err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "读取数据失败"})
		return
	}
	jsonResp(w, map[string]any{"success": true, "data": inbounds})
}

// countClients 从 settings JSON 中计算客户端数量
func countClients(settings string) int {
	var config struct {
		Clients []any `json:"clients"`
	}
	if err := json.Unmarshal([]byte(settings), &config); err != nil {
		return 0
	}
	return len(config.Clients)
}

func (s *Server) handleInbound(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/inbound/")
	id, err := strconv.Atoi(path)
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "Invalid ID"})
		return
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "数据库连接失败"})
		return
	}

	switch r.Method {
	case "GET":
		var port int
		var protocol, settings, remark string
		var enable bool
		err := sqlDB.QueryRow("SELECT port, protocol, enable, settings, remark FROM inbounds WHERE id=?", id).
			Scan(&port, &protocol, &enable, &settings, &remark)
		if err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "Not found"})
			return
		}
		jsonResp(w, map[string]any{"success": true, "data": map[string]any{
			"id": id, "port": port, "protocol": protocol,
			"enable": enable, "settings": settings, "remark": remark,
		}})
	case "DELETE":
		if _, err := sqlDB.Exec("DELETE FROM inbounds WHERE id=?", id); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "删除失败"})
			return
		}
		jsonResp(w, map[string]any{"success": true, "msg": "Deleted"})
	case "PUT":
		var req struct {
			Enable *bool `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "Invalid JSON"})
			return
		}
		if req.Enable != nil {
			v := 0
			if *req.Enable {
				v = 1
			}
			if _, err := sqlDB.Exec("UPDATE inbounds SET enable=? WHERE id=?", v, id); err != nil {
				jsonResp(w, map[string]any{"success": false, "msg": "更新失败"})
				return
			}
		}
		jsonResp(w, map[string]any{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/clients/")
	inboundId, err := strconv.Atoi(path)
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "Invalid ID"})
		return
	}

	// 获取入站 settings（包含客户端配置）
	var settings string
	s.db.Raw("SELECT settings FROM inbounds WHERE id = ?", inboundId).Scan(&settings)
	if settings == "" {
		jsonResp(w, map[string]any{"success": false, "msg": "Inbound not found"})
		return
	}

	// 解析客户端基本信息
	var config struct {
		Clients []struct {
			ID         string `json:"id"`
			Email      string `json:"email"`
			Enable     bool   `json:"enable"`
			ExpiryTime int64  `json:"expiryTime"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(settings), &config); err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "解析入站配置失败"})
		return
	}

	// 查询每个客户端的流量统计（从 client_traffics 表）
	sqlDB, err := s.db.DB()
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "数据库连接失败"})
		return
	}
	clients := []map[string]any{}
	for i, c := range config.Clients {
		var up, down int64
		if c.Email != "" {
			sqlDB.QueryRow(
				"SELECT COALESCE(up,0), COALESCE(down,0) FROM client_traffics WHERE inbound_id = ? AND email = ?",
				inboundId, c.Email,
			).Scan(&up, &down)
		}
		clients = append(clients, map[string]any{
			"id":         i + 1,
			"email":      c.Email,
			"enable":     c.Enable,
			"up":         up,
			"down":       down,
			"expiryTime": c.ExpiryTime,
		})
	}

	jsonResp(w, map[string]any{"success": true, "data": clients})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		if err := exec.Command("systemctl", "restart", "x-ui").Run(); err != nil {
			log.Printf("[NCP] 重启 x-ui 失败: %v", err)
		}
	}()
	jsonResp(w, map[string]any{"success": true, "msg": "Restarting"})
}

func (s *Server) handleRestartXray(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := exec.Command("pkill", "-f", "xray-linux").Run(); err != nil {
		log.Printf("[NCP] 终止 xray 进程失败: %v", err)
	}
	jsonResp(w, map[string]any{"success": true, "msg": "Xray restarted"})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonResp(w, map[string]any{"success": true, "data": map[string]any{
			"panel_port": agent.GetPort(),
			"admin_user": agent.GetUsername(),
		}})
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "Invalid JSON"})
		return
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "数据库连接失败"})
		return
	}
	for key, value := range req {
		switch key {
		case "panel_port":
			p, err := strconv.Atoi(value)
			if err != nil || p < 1 || p > 65535 {
				jsonResp(w, map[string]any{"success": false, "msg": "Invalid port"})
				return
			}
			if _, err := sqlDB.Exec("UPDATE settings SET value=? WHERE key='webPort'", value); err != nil {
				jsonResp(w, map[string]any{"success": false, "msg": "更新端口失败"})
				return
			}
		case "admin_user":
			if len(value) < 3 {
				jsonResp(w, map[string]any{"success": false, "msg": "Username too short"})
				return
			}
			if _, err := sqlDB.Exec("UPDATE settings SET value=? WHERE key='webUsername'", value); err != nil {
				jsonResp(w, map[string]any{"success": false, "msg": "更新用户名失败"})
				return
			}
		case "admin_pass":
			if len(value) < 6 {
				jsonResp(w, map[string]any{"success": false, "msg": "Password too short (min 6)"})
				return
			}
			// 密码必须经过 bcrypt 哈希后存储
			hashedPass, err := bcryptHash(value)
			if err != nil {
				jsonResp(w, map[string]any{"success": false, "msg": "Failed to hash password"})
				return
			}
			if _, err := sqlDB.Exec("UPDATE settings SET value=? WHERE key='webPassword'", hashedPass); err != nil {
				jsonResp(w, map[string]any{"success": false, "msg": "更新密码失败"})
				return
			}
		}
	}
	jsonResp(w, map[string]any{"success": true})
}

// ========== Xray Config & Relay ==========

// handleXrayConfig GET/POST xrayTemplateConfig from settings table
func (s *Server) handleXrayConfig(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := s.db.DB()
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "数据库连接失败"})
		return
	}

	switch r.Method {
	case "GET":
		var value string
		if err := sqlDB.QueryRow("SELECT value FROM settings WHERE key='xrayTemplateConfig'").Scan(&value); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "读取配置失败"})
			return
		}
		jsonResp(w, map[string]any{"success": true, "data": value})
	case "POST":
		var req struct {
			Config string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "Invalid JSON"})
			return
		}
		// 验证 JSON 格式
		var check map[string]any
		if err := json.Unmarshal([]byte(req.Config), &check); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "配置不是合法的 JSON"})
			return
		}
		if _, err := sqlDB.Exec("UPDATE settings SET value=? WHERE key='xrayTemplateConfig'", req.Config); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "更新配置失败"})
			return
		}
		// 重启 Xray 使配置生效
		if err := exec.Command("pkill", "-f", "xray-linux").Run(); err != nil {
			log.Printf("[NCP] 重启 xray 失败: %v", err)
		}
		jsonResp(w, map[string]any{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRelayOutbound POST=添加 relay outbound+routing, DELETE=移除
func (s *Server) handleRelayOutbound(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := s.db.DB()
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "数据库连接失败"})
		return
	}

	// 读取当前 xrayTemplateConfig
	var configStr string
	if err := sqlDB.QueryRow("SELECT value FROM settings WHERE key='xrayTemplateConfig'").Scan(&configStr); err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "读取 Xray 配置失败"})
		return
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "解析 Xray 配置失败"})
		return
	}

	switch r.Method {
	case "POST":
		var req struct {
			InboundTag string         `json:"inboundTag"`
			Outbound   map[string]any `json:"outbound"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "Invalid JSON"})
			return
		}
		if req.InboundTag == "" || req.Outbound == nil {
			jsonResp(w, map[string]any{"success": false, "msg": "inboundTag 和 outbound 必填"})
			return
		}

		outboundTag, _ := req.Outbound["tag"].(string)
		if outboundTag == "" {
			jsonResp(w, map[string]any{"success": false, "msg": "outbound.tag 必填"})
			return
		}

		// 追加 outbound
		outbounds, _ := config["outbounds"].([]any)
		outbounds = append(outbounds, req.Outbound)
		config["outbounds"] = outbounds

		// 追加 routing rule
		routing, _ := config["routing"].(map[string]any)
		if routing == nil {
			routing = map[string]any{}
		}
		rules, _ := routing["rules"].([]any)
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []any{req.InboundTag},
			"outboundTag": outboundTag,
		})
		routing["rules"] = rules
		config["routing"] = routing

		// 写回
		newConfig, _ := json.Marshal(config)
		if _, err := sqlDB.Exec("UPDATE settings SET value=? WHERE key='xrayTemplateConfig'", string(newConfig)); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "保存配置失败"})
			return
		}
		if err := exec.Command("pkill", "-f", "xray-linux").Run(); err != nil {
			log.Printf("[NCP] 重启 xray 失败: %v", err)
		}
		jsonResp(w, map[string]any{"success": true})

	case "DELETE":
		var req struct {
			OutboundTag string `json:"outboundTag"`
			InboundTag  string `json:"inboundTag"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "Invalid JSON"})
			return
		}
		if req.OutboundTag == "" {
			jsonResp(w, map[string]any{"success": false, "msg": "outboundTag 必填"})
			return
		}

		// 保护内置 outbound 标签
		protectedTags := map[string]bool{"direct": true, "blocked": true, "block": true}
		if protectedTags[req.OutboundTag] {
			jsonResp(w, map[string]any{"success": false, "msg": "不允许删除内置 outbound: " + req.OutboundTag})
			return
		}

		// 移除 outbound
		if outbounds, ok := config["outbounds"].([]any); ok {
			filtered := make([]any, 0, len(outbounds))
			for _, ob := range outbounds {
				if obMap, ok := ob.(map[string]any); ok {
					if tag, _ := obMap["tag"].(string); tag == req.OutboundTag {
						continue
					}
				}
				filtered = append(filtered, ob)
			}
			config["outbounds"] = filtered
		}

		// 移除关联 routing rules
		if routing, ok := config["routing"].(map[string]any); ok {
			if rules, ok := routing["rules"].([]any); ok {
				filtered := make([]any, 0, len(rules))
				for _, rule := range rules {
					ruleMap, ok := rule.(map[string]any)
					if !ok {
						filtered = append(filtered, rule)
						continue
					}
					if tag, _ := ruleMap["outboundTag"].(string); tag == req.OutboundTag {
						continue
					}
					filtered = append(filtered, rule)
				}
				routing["rules"] = filtered
			}
		}

		newConfig, _ := json.Marshal(config)
		if _, err := sqlDB.Exec("UPDATE settings SET value=? WHERE key='xrayTemplateConfig'", string(newConfig)); err != nil {
			jsonResp(w, map[string]any{"success": false, "msg": "保存配置失败"})
			return
		}
		if err := exec.Command("pkill", "-f", "xray-linux").Run(); err != nil {
			log.Printf("[NCP] 重启 xray 失败: %v", err)
		}
		jsonResp(w, map[string]any{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ========== DB helpers ==========

func (s *Server) getInboundCount() int {
	var count int64
	s.db.Raw("SELECT COUNT(*) FROM inbounds").Scan(&count)
	return int(count)
}

func (s *Server) getTotalTraffic(field string) int64 {
	var total int64
	switch field {
	case "up":
		s.db.Raw("SELECT COALESCE(SUM(up),0) FROM inbounds").Scan(&total)
	case "down":
		s.db.Raw("SELECT COALESCE(SUM(down),0) FROM inbounds").Scan(&total)
	}
	return total
}

func (s *Server) getServiceStatus() string {
	out, _ := exec.Command("systemctl", "is-active", "x-ui").CombinedOutput()
	return strings.TrimSpace(string(out))
}

func (s *Server) getXrayStatus() string {
	out, _ := exec.Command("pgrep", "-f", "xray-linux").CombinedOutput()
	if strings.TrimSpace(string(out)) != "" {
		return "running"
	}
	return "stopped"
}

func (s *Server) getXrayVersion() string {
	arch := getArch()
	out, _ := exec.Command(fmt.Sprintf("/usr/local/x-ui/bin/xray-linux-%s", arch), "version").CombinedOutput()
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return ""
}

func getArch() string {
	out, _ := exec.Command("uname", "-m").CombinedOutput()
	switch strings.TrimSpace(string(out)) {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return strings.TrimSpace(string(out))
	}
}
