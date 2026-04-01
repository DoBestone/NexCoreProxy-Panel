#!/bin/bash
#
# NexCoreProxy Panel 安装脚本
#
# 用法:
#   bash install.sh                              # 交互式安装（随机端口/账号/密码）
#   bash install.sh -p 54321 -u admin -k mypass  # 指定参数自动安装
#
# 参数:
#   -p, --port      面板端口 (默认: 随机 10000-65000)
#   -u, --user      管理员用户名 (默认: 随机 8 位)
#   -k, --pass      管理员密码 (默认: 随机 16 位)
#   --api-port      NCP API 端口 (默认: 54322)
#   --force         强制重装，跳过确认
#   -h, --help      显示帮助
#

set -e

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
blue='\033[0;34m'
plain='\033[0m'

XUI_DIR="/usr/local/x-ui"
XUI_SERVICE="/etc/systemd/system/x-ui.service"
XUI_DB="${XUI_DIR}/db/x-ui.db"
GITHUB_REPO="DoBestone/NexCoreProxy"
RELEASE_BASE="https://github.com/${GITHUB_REPO}/releases/download"

# ========== 基础检查 ==========

[[ $EUID -ne 0 ]] && echo -e "${red}错误: 请使用 root 权限运行${plain}" && exit 1

get_arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo 'amd64' ;;
        armv8* | arm64 | aarch64) echo 'arm64' ;;
        *) echo -e "${red}不支持的架构: $(uname -m)${plain}" && exit 1 ;;
    esac
}

get_os_release() {
    if [[ -f /etc/os-release ]]; then
        source /etc/os-release
        echo "$ID"
    else
        echo "unknown"
    fi
}

gen_random_string() {
    LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | head -c "$1"
}

gen_random_port() {
    shuf -i 10000-62000 -n 1
}

# ========== 参数解析 ==========

PANEL_PORT=""
PANEL_USER=""
PANEL_PASS=""
API_PORT="54322"
FORCE_INSTALL=false

show_help() {
    echo "NexCoreProxy Panel 安装脚本"
    echo ""
    echo "用法: bash install.sh [选项]"
    echo ""
    echo "选项:"
    echo "  -p, --port <端口>     面板端口 (默认: 随机)"
    echo "  -u, --user <用户名>   管理员用户名 (默认: 随机)"
    echo "  -k, --pass <密码>     管理员密码 (默认: 随机)"
    echo "  --api-port <端口>     NCP API 端口 (默认: 54322)"
    echo "  --force               强制重装，跳过确认"
    echo "  -h, --help            显示帮助"
    echo ""
    echo "示例:"
    echo "  bash install.sh -p 54321 -u admin -k MyPass123"
    echo "  bash install.sh --force"
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -p|--port)   PANEL_PORT="$2"; shift 2 ;;
        -u|--user)   PANEL_USER="$2"; shift 2 ;;
        -k|--pass)   PANEL_PASS="$2"; shift 2 ;;
        --api-port)  API_PORT="$2"; shift 2 ;;
        --force)     FORCE_INSTALL=true; shift ;;
        -h|--help)   show_help ;;
        *)           echo -e "${red}未知参数: $1${plain}"; show_help ;;
    esac
done

# 填充默认值
[[ -z "$PANEL_PORT" ]] && PANEL_PORT=$(gen_random_port)
[[ -z "$PANEL_USER" ]] && PANEL_USER=$(gen_random_string 8)
[[ -z "$PANEL_PASS" ]] && PANEL_PASS=$(gen_random_string 16)

# ========== 检测旧安装 ==========

check_existing_install() {
    if [[ -d "$XUI_DIR" ]] || [[ -f "$XUI_SERVICE" ]]; then
        echo -e "${yellow}检测到已有安装:${plain}"
        [[ -d "$XUI_DIR" ]] && echo -e "  目录: ${XUI_DIR}"
        [[ -f "$XUI_SERVICE" ]] && echo -e "  服务: ${XUI_SERVICE}"

        if [[ "$FORCE_INSTALL" != true ]]; then
            echo ""
            read -rp "是否卸载旧版本并重新安装? [y/N]: " confirm
            if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
                echo -e "${yellow}已取消安装${plain}"
                exit 0
            fi
        fi

        echo -e "${green}正在清理旧安装...${plain}"
        # 停止服务
        systemctl stop x-ui 2>/dev/null || true
        systemctl disable x-ui 2>/dev/null || true

        # 备份数据库（以防万一）
        if [[ -f "$XUI_DB" ]]; then
            local backup="/tmp/x-ui-db-backup-$(date +%Y%m%d%H%M%S).db"
            cp -f "$XUI_DB" "$backup" 2>/dev/null || true
            echo -e "  数据库已备份到: ${backup}"
        fi

        # 清理文件
        rm -rf "$XUI_DIR"
        rm -f "$XUI_SERVICE"
        rm -f /usr/bin/x-ui
        rm -f /usr/bin/ncp-agent
        systemctl daemon-reload 2>/dev/null || true

        echo -e "${green}旧安装已清理完毕${plain}"
    fi
}

# ========== 安装依赖 ==========

install_deps() {
    local release=$(get_os_release)
    echo -e "${green}安装基础依赖...${plain}"
    case "$release" in
        ubuntu|debian|armbian)
            apt-get update -qq && apt-get install -y -qq curl tar >/dev/null 2>&1
            ;;
        centos|fedora|rhel|almalinux|rocky|amzn)
            dnf install -y -q curl tar >/dev/null 2>&1 || yum install -y curl tar >/dev/null 2>&1
            ;;
        alpine)
            apk update && apk add curl tar >/dev/null 2>&1
            ;;
        *)
            apt-get update -qq && apt-get install -y -qq curl tar >/dev/null 2>&1 || true
            ;;
    esac
}

# ========== 下载安装 ==========

install_panel() {
    local arch=$(get_arch)
    local tmp_dir="/tmp/ncp-panel-install"
    rm -rf "$tmp_dir"
    mkdir -p "$tmp_dir"

    echo -e "${green}检测架构: ${arch}${plain}"

    # 获取最新版本
    echo -e "${green}获取最新版本...${plain}"
    local tag_version
    tag_version=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [[ -z "$tag_version" ]]; then
        # 回退: 直接下载 main 分支的编译产物
        tag_version="latest"
        echo -e "${yellow}无法获取版本号，使用 latest${plain}"
    fi

    local download_url="${RELEASE_BASE}/${tag_version}/x-ui-linux-${arch}.tar.gz"
    echo -e "${green}下载: ${download_url}${plain}"

    curl -fSL -o "${tmp_dir}/x-ui.tar.gz" "$download_url" 2>/dev/null
    if [[ $? -ne 0 ]]; then
        echo -e "${red}下载失败，请检查网络连接或 GitHub 访问${plain}"
        rm -rf "$tmp_dir"
        exit 1
    fi

    # 解压安装
    echo -e "${green}解压安装...${plain}"
    mkdir -p "$XUI_DIR"
    tar -xzf "${tmp_dir}/x-ui.tar.gz" -C "${XUI_DIR%/x-ui}/"

    chmod +x "${XUI_DIR}/x-ui"
    chmod +x "${XUI_DIR}/bin/xray-linux-${arch}" 2>/dev/null || true

    # 创建 ncp-agent 符号链接（向后兼容）
    ln -sf "${XUI_DIR}/x-ui" /usr/bin/ncp-agent

    # 创建日志目录
    mkdir -p /var/log/x-ui

    rm -rf "$tmp_dir"
    echo -e "${green}文件安装完成${plain}"
}

# ========== 配置面板 ==========

configure_panel() {
    echo -e "${green}配置面板...${plain}"

    # 使用内置 install 命令一次性完成配置
    "${XUI_DIR}/x-ui" install \
        --port "$PANEL_PORT" \
        --user "$PANEL_USER" \
        --pass "$PANEL_PASS" \
        --api-port "$API_PORT" \
        --gen-token

    echo -e "${green}配置完成${plain}"
}

# ========== 安装 systemd 服务 ==========

install_service() {
    echo -e "${green}安装系统服务...${plain}"

    cat > "$XUI_SERVICE" << 'SERVICEEOF'
[Unit]
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
SERVICEEOF

    systemctl daemon-reload
    systemctl enable x-ui
    systemctl start x-ui

    # 等待服务启动
    sleep 2

    if systemctl is-active --quiet x-ui; then
        echo -e "${green}服务启动成功${plain}"
    else
        echo -e "${yellow}服务可能需要稍等片刻才能完全启动${plain}"
    fi
}

# ========== 读取 API Token ==========

get_api_token() {
    local token
    token=$("${XUI_DIR}/x-ui" ncp get-token 2>/dev/null)
    echo "$token"
}

# ========== 获取服务器 IP ==========

get_server_ip() {
    local ip=""
    for url in "https://api4.ipify.org" "https://ipv4.icanhazip.com" "https://4.ident.me"; do
        ip=$(curl -s --max-time 3 "$url" 2>/dev/null | tr -d '[:space:]')
        [[ -n "$ip" ]] && break
    done
    echo "${ip:-unknown}"
}

# ========== 主流程 ==========

main() {
    echo -e "${green}╔═══════════════════════════════════════════╗${plain}"
    echo -e "${green}║    NexCoreProxy Panel 安装程序            ║${plain}"
    echo -e "${green}╚═══════════════════════════════════════════╝${plain}"
    echo ""

    check_existing_install
    install_deps
    install_panel
    configure_panel
    install_service

    local server_ip=$(get_server_ip)
    local api_token=$(get_api_token)

    echo ""
    echo -e "${green}╔═══════════════════════════════════════════╗${plain}"
    echo -e "${green}║         安装完成！                        ║${plain}"
    echo -e "${green}╠═══════════════════════════════════════════╣${plain}"
    echo -e "${green}║${plain} 面板地址:  ${blue}http://${server_ip}:${PANEL_PORT}${plain}"
    echo -e "${green}║${plain} 用户名:    ${blue}${PANEL_USER}${plain}"
    echo -e "${green}║${plain} 密码:      ${blue}${PANEL_PASS}${plain}"
    echo -e "${green}║${plain} 面板端口:  ${blue}${PANEL_PORT}${plain}"
    echo -e "${green}║${plain} API 端口:  ${blue}${API_PORT}${plain}"
    if [[ -n "$api_token" ]]; then
    echo -e "${green}║${plain} API Token: ${blue}${api_token}${plain}"
    fi
    echo -e "${green}╠═══════════════════════════════════════════╣${plain}"
    echo -e "${green}║${plain} 管理命令:                                ${green}║${plain}"
    echo -e "${green}║${plain}   systemctl start x-ui    # 启动         ${green}║${plain}"
    echo -e "${green}║${plain}   systemctl stop x-ui     # 停止         ${green}║${plain}"
    echo -e "${green}║${plain}   systemctl restart x-ui  # 重启         ${green}║${plain}"
    echo -e "${green}║${plain}   ncp-agent status        # 查看状态     ${green}║${plain}"
    echo -e "${green}╚═══════════════════════════════════════════╝${plain}"
    echo ""
    echo -e "${yellow}请妥善保存以上凭据！${plain}"
}

main
