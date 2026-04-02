[English](/README.md) | [中文](/README.zh_CN.md)

# NexCoreProxy Panel

[![Release](https://img.shields.io/github/v/release/DoBestone/NexCoreProxy-Panel.svg)](https://github.com/DoBestone/NexCoreProxy-Panel/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/DoBestone/NexCoreProxy-Panel/release.yml.svg)](https://github.com/DoBestone/NexCoreProxy-Panel/actions)
[![GO Version](https://img.shields.io/github/go-mod/go-version/DoBestone/NexCoreProxy-Panel.svg)](#)
[![Downloads](https://img.shields.io/github/downloads/DoBestone/NexCoreProxy-Panel/total.svg)](https://github.com/DoBestone/NexCoreProxy-Panel/releases/latest)
[![License](https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true)](https://www.gnu.org/licenses/gpl-3.0.en.html)

**NexCoreProxy Panel** is the node-side management panel for the NexCoreProxy distributed proxy management system. It provides a web-based interface for configuring and monitoring Xray-core proxy services on each node, with built-in NCP Agent for centralized management by [NexCoreProxy Master](https://github.com/DoBestone/NexCoreProxy-Master).

> [!NOTE]
> This project is a secondary development based on [3X-UI](https://github.com/MHSanaei/3x-ui) (GPL-3.0). We gratefully acknowledge the original 3X-UI project and its contributors for their excellent work.

## Features

- Web-based Xray-core management panel (inherited from 3X-UI)
- Built-in NCP Agent for communication with NexCoreProxy Master
- REST API for remote status monitoring and inbound management
- Multi-protocol support (VMess, VLESS, Trojan, Shadowsocks, etc.)
- Multi-platform builds (Linux amd64/arm64/armv7/armv6/386/s390x + Windows)

## Quick Start

```bash
bash <(curl -Ls https://raw.githubusercontent.com/DoBestone/NexCoreProxy-Panel/main/install.sh)
```

## Architecture

```
NexCoreProxy Master  <──REST API──>  NexCoreProxy Panel (this repo)
     (Central)                         ├── Web UI (3X-UI based)
                                       ├── NCP Agent (heartbeat + registration)
                                       └── NCP API (status/inbounds/control)
```

## Based On

This project is based on [3X-UI](https://github.com/MHSanaei/3x-ui) by [MHSanaei](https://github.com/MHSanaei), which is an enhanced fork of the original [X-UI](https://github.com/vaxilu/x-ui) project. Licensed under GPL-3.0.

## Acknowledgments

- [3X-UI](https://github.com/MHSanaei/3x-ui) — The upstream project this panel is based on
- [alireza0](https://github.com/alireza0/) — Major contributor to 3X-UI
- [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) (License: **GPL-3.0**)
- [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat) (License: **GPL-3.0**)

## License

[GPL-3.0](LICENSE)
