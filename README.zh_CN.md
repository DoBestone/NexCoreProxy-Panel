[English](/README.md) | [中文](/README.zh_CN.md)

# NexCoreProxy Panel

[![Release](https://img.shields.io/github/v/release/DoBestone/NexCoreProxy-Panel.svg)](https://github.com/DoBestone/NexCoreProxy-Panel/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/DoBestone/NexCoreProxy-Panel/release.yml.svg)](https://github.com/DoBestone/NexCoreProxy-Panel/actions)
[![GO Version](https://img.shields.io/github/go-mod/go-version/DoBestone/NexCoreProxy-Panel.svg)](#)
[![Downloads](https://img.shields.io/github/downloads/DoBestone/NexCoreProxy-Panel/total.svg)](https://github.com/DoBestone/NexCoreProxy-Panel/releases/latest)
[![License](https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true)](https://www.gnu.org/licenses/gpl-3.0.en.html)

**NexCoreProxy Panel** 是 NexCoreProxy 分布式代理管理系统的节点端管理面板。提供基于 Web 的界面，用于配置和监控每个节点上的 Xray-core 代理服务，内置 NCP Agent 实现与 [NexCoreProxy Master](https://github.com/DoBestone/NexCoreProxy-Master) 的集中管理。

> [!NOTE]
> 本项目基于 [3X-UI](https://github.com/MHSanaei/3x-ui)（GPL-3.0 协议）进行二次开发。感谢 3X-UI 项目及其贡献者的优秀工作。

## 功能特性

- 基于 Web 的 Xray-core 管理面板（继承自 3X-UI）
- 内置 NCP Agent，与 NexCoreProxy Master 通信
- REST API，支持远程状态监控和入站管理
- 多协议支持（VMess、VLESS、Trojan、Shadowsocks 等）
- 多平台构建（Linux amd64/arm64/armv7/armv6/386/armv5/s390x）

## 快速开始

```bash
bash <(curl -Ls https://raw.githubusercontent.com/DoBestone/NexCoreProxy-Panel/main/install.sh)
```

## 系统架构

```
NexCoreProxy Master  <──REST API──>  NexCoreProxy Panel（本仓库）
     （中控端）                          ├── Web UI（基于 3X-UI）
                                        ├── NCP Agent（心跳 + 注册）
                                        └── NCP API（状态/入站/控制）
```

## 基于

本项目基于 [MHSanaei](https://github.com/MHSanaei) 的 [3X-UI](https://github.com/MHSanaei/3x-ui) 进行二次开发，3X-UI 是原始 [X-UI](https://github.com/vaxilu/x-ui) 项目的增强版本。遵循 GPL-3.0 协议。

## 致谢

- [3X-UI](https://github.com/MHSanaei/3x-ui) — 本面板的上游项目
- [alireza0](https://github.com/alireza0/) — 3X-UI 主要贡献者
- [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules)（许可证：**GPL-3.0**）
- [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat)（许可证：**GPL-3.0**）

## 许可证

[GPL-3.0](LICENSE)
