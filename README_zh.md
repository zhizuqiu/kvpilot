<div align="center">
<a href="https://github.com/zhizuqiu/kvpilot/"><img src="build/appicon.png" width="120"/></a>
</div>

<h1 align="center">KVPilot</h1>
<h4 align="center"><a href="https://github.com/zhizuqiu/kvpilot/blob/main/README.md">English</a> | <strong>简体中文</strong></h4>

> **KVPilot** 是 **Tiny RDM** 1.2.7 的非官方社区延续分支（GPL-3.0）。
> 当前版本：**1.3.0**。
> 与 Tiny Craft 无隶属、背书或赞助关系。

<div align="center">

[![License](https://img.shields.io/github/license/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/releases)
![GitHub All Releases](https://img.shields.io/github/downloads/zhizuqiu/kvpilot/total)
[![GitHub stars](https://img.shields.io/github/stars/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/fork)

<strong>一个现代化轻量级的跨平台Redis桌面客户端，支持Mac、Windows和Linux，同时提供Web版本，可通过Docker快速部署</strong>
</div>

<div align="center">
<img alt="KVPilot 仪表盘" src="docs/assets/03-browser.png" />

<img alt="KVPilot 键详情" src="docs/assets/02-key-detail.png" />

<img alt="KVPilot 槽位管理" src="docs/assets/05-slot-management.png" />
</div>

## 功能特性

* 极度轻量，基于Webview2，无内嵌浏览器（感谢[Wails](https://github.com/wailsapp/wails)）
* 界面精美易用，提供浅色/深色主题（感谢[Naive UI](https://github.com/tusen-ai/naive-ui)
  和 [IconPark](https://iconpark.oceanengine.com)）
* 多国语言支持：英文/简中/繁中/日/韩/法/西/葡/俄/土（[需要更多语言支持？点我贡献语言](.github/CONTRIBUTING_zh.md)）
* 更好用的连接管理：支持SSH隧道/SSL/哨兵模式/集群模式/HTTP代理/SOCKS5代理
* 可视化键值操作，增删查改一应俱全
* 支持多种数据查看格式以及转码/解压方式
* 采用SCAN分段加载，可轻松处理数百万键列表
* 操作命令执行日志展示
* 提供命令行操作
* 提供慢日志展示
* List/Hash/Set/Sorted Set的分段加载和查询
* List/Hash/Set/Sorted Set值的转码显示
* 内置高级编辑器Monaco Editor
* 支持命令实时监控
* 支持导入/导出数据
* 支持发布订阅
* 支持导入/导出连接配置
* 自定义数据展示编码/解码([这是操作指引](https://github.com/zhizuqiu/kvpilot/blob/main/docs/custom-decoder.md))
* 部署拓扑视图，支持单机/主从/哨兵/集群
* 可视化 ACL 管理，支持权限试算与拒绝日志
* 可视化配置管理：查看和编辑 Redis 参数，集群支持逐节点对比
* 会话管理，保护主从/订阅连接不被误杀
* 大 key 扫描与键空间统计（类型/TTL/前缀），支持 CSV 导出
* 只读集群巡检：按严重级别给出原因与建议
* 维护操作：按主节点 BGSAVE（带进度）、SLOWLOG 重置
* 集群槽位管理：逐槽键数量分布图、区间粘贴与框选批量迁移、按槽位数/键数量的均衡迁移

## 集群运维

KVPilot 提供只读集群巡检和可控的槽位操作：

* **槽位分布**：每个主节点一块，按键数量着色，支持对数/线性色阶和悬停详情
* **批量迁移**：在分布图点击槽位并选择目标主节点，加入批量列表；槽位号支持逗号分隔和区间，可直接粘贴槽位范围列的文本（如 `0-6, 8, 10-12`）
* **框选**：macOS 按住 ⌘ Command，其他平台按住 Alt 拖动，可矩形框选多个槽位
* **均衡迁移**：按槽位数或键数量生成计划，可调容差，支持暂停/继续/取消
* **安全说明**：槽位操作只针对主节点；已完成的槽位不回滚；取消在槽位之间生效；扫描时无响应的节点会被标记并从图中排除

## 安装

提供 macOS（Apple Silicon / Intel）、Windows 和 Linux 安装包，可在[发布页](https://github.com/zhizuqiu/kvpilot/releases)下载。

> macOS 版本为 ad-hoc 签名，首次打开可能被系统拦截。执行一次下面的命令后再启动即可：
> ``` shell
>  sudo xattr -d com.apple.quarantine /Applications/KVPilot.app
> ```

> Windows 版本暂未代码签名，首次运行可能弹出 SmartScreen 提示，选择**更多信息 → 仍要运行**即可。
> 代码签名正在申请中，通过后会替换为已签名的安装包，提示随之消失。

想用浏览器访问？参见 [Docker 部署](#docker-部署)。

## 构建客户端

### 运行环境要求

* Go >= 1.25
* Node.js >= 20
* NPM >= 9

### 安装wails

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 拉取代码

```bash
git clone https://github.com/zhizuqiu/kvpilot --depth=1
```

### 构建前端代码

```bash
npm install --prefix ./frontend
```

或者

```bash
cd frontend
npm install
```

### 编译运行开发版本

```bash
wails dev
```

### 测试与本地 Web 开发

运行测试：

```bash
go test ./...
cd frontend && npm run test
```

本地 Web 模式分别启动后端和前端开发服务器：

```bash
# 终端 1：后端监听 :8088
go build -tags web -o /tmp/kvpilot-web . && PORT=8088 /tmp/kvpilot-web

# 终端 2：前端开发服务器，把 /api 和 /ws 代理到 :8088
cd frontend && VITE_WEB=true npm run dev
```

## Docker 部署

镜像发布在 Docker Hub：[`zhizuqiu/kvpilot`](https://hub.docker.com/r/zhizuqiu/kvpilot)，
提供 `linux/amd64` 与 `linux/arm64` 两个架构。除桌面客户端外，Web 版也由同一个镜像提供。

### 使用 Docker Compose（推荐）

创建 `docker-compose.yml` 文件：

```yaml
services:
  kvpilot:
    image: zhizuqiu/kvpilot:latest
    container_name: kvpilot
    restart: unless-stopped
    ports:
      - "8086:8086"
    environment:
      - ADMIN_USERNAME=admin
      - ADMIN_PASSWORD=change-me
      - SESSION_TTL=24h
    volumes:
      - ./data:/app/tinyrdm
```

启动服务：

```bash
docker compose up -d
```

启动后访问 `http://localhost:8086`，使用上面配置的用户名密码登录。
Web 前端默认使用 `./` 相对资源/API/WebSocket 地址，可直接部署在根路径或 `/kvpilot/` 子路径。
需要覆盖构建基路径时，可在构建镜像时传入 `--build-arg VITE_BASE_PATH=...`。

### 使用 Docker 命令

```bash
docker run -d --name kvpilot \
  -p 8086:8086 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=change-me \
  -v ./data:/app/tinyrdm \
  zhizuqiu/kvpilot:latest
```

### 环境变量说明

| 变量               | 说明    | 默认值 |
|------------------|-------|-----|
| `ADMIN_USERNAME` | 登录用户名 | -   |
| `ADMIN_PASSWORD` | 登录密码  | -   |
| `SESSION_TTL` | 登录会话有效期，Go duration 格式（例如 `24h`） | `24h` |
| `PORT` | 容器内后端监听端口，对外仍由 nginx 暴露 `8086` | `8088` |

连接配置和偏好设置保存在 `/app/tinyrdm`，升级或重建容器前请保留该目录。Web 模式使用
单管理员账号，暴露到网络时建议放在 HTTPS 反向代理之后。

## 关于

KVPilot 是 [Tiny RDM](https://github.com/tiny-craft/tiny-rdm) 1.2.7 的社区延续分支，
面向仍在使用开源桌面客户端、且需要跟进新 Redis / KV 兼容实现的用户。

- 项目范围：缺陷修复、依赖安全升级、Redis 协议与命令兼容、集群槽位运维、Web 模式加固
- 当前版本：1.3.0（基于 Tiny RDM 1.2.7）
- 非目标：不做界面大改、不追上游闭源版本（Redisee）的功能
- 维护方式：Issue / PR 驱动，见 [参与贡献](#参与贡献)
- 许可证：GPL-3.0，非官方衍生版本，与 Tiny Craft 无隶属关系

## 参与贡献

- 问题反馈与功能建议：<https://github.com/zhizuqiu/kvpilot/issues>
- 本项目上游为 [Tiny RDM](https://github.com/tiny-craft/tiny-rdm)，感谢原作者的开源工作
- 许可证：[GPL-3.0](LICENSE)，第三方组件许可见 [NOTICE](NOTICE)
