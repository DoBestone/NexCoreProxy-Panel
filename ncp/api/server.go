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

	"gorm.io/gorm"
)

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
		if reqToken == "" {
			reqToken = r.URL.Query().Get("token")
		}
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
		http.Error(w, "Method not allowed", 405)
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
	jsonResp(w, map[string]any{"success": true, "data": inbounds})
}

// countClients 从 settings JSON 中计算客户端数量
func countClients(settings string) int {
	var config struct {
		Clients []any `json:"clients"`
	}
	json.Unmarshal([]byte(settings), &config)
	return len(config.Clients)
}

func (s *Server) handleInbound(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/inbound/")
	id, err := strconv.Atoi(path)
	if err != nil {
		jsonResp(w, map[string]any{"success": false, "msg": "Invalid ID"})
		return
	}
	sqlDB, _ := s.db.DB()

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
		sqlDB.Exec("DELETE FROM inbounds WHERE id=?", id)
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
			sqlDB.Exec("UPDATE inbounds SET enable=? WHERE id=?", v, id)
		}
		jsonResp(w, map[string]any{"success": true})
	default:
		http.Error(w, "Method not allowed", 405)
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
	json.Unmarshal([]byte(settings), &config)

	// 查询每个客户端的流量统计（从 client_traffics 表）
	sqlDB, _ := s.db.DB()
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
		http.Error(w, "Method not allowed", 405)
		return
	}
	go exec.Command("systemctl", "restart", "x-ui").Run()
	jsonResp(w, map[string]any{"success": true, "msg": "Restarting"})
}

func (s *Server) handleRestartXray(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	exec.Command("pkill", "-f", "xray-linux").Run()
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
	sqlDB, _ := s.db.DB()
	for key, value := range req {
		switch key {
		case "panel_port":
			p, err := strconv.Atoi(value)
			if err != nil || p < 1 || p > 65535 {
				jsonResp(w, map[string]any{"success": false, "msg": "Invalid port"})
				return
			}
			sqlDB.Exec("UPDATE settings SET value=? WHERE key='webPort'", value)
		case "admin_user":
			sqlDB.Exec("UPDATE settings SET value=? WHERE key='webUsername'", value)
		case "admin_pass":
			sqlDB.Exec("UPDATE settings SET value=? WHERE key='webPassword'", value)
		}
	}
	jsonResp(w, map[string]any{"success": true})
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
	case "up", "down":
		s.db.Raw(fmt.Sprintf("SELECT COALESCE(SUM(%s),0) FROM inbounds", field)).Scan(&total)
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
