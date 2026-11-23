<div align="center">
<a href="https://github.com/zhizuqiu/kvpilot/"><img src="build/appicon.png" width="120"/></a>
</div>

<h1 align="center">KVPilot</h1>
<h4 align="center"><strong>English</strong> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_zh.md">简体中文</a></h4>

> **KVPilot** is an unofficial community continuation of **Tiny RDM** 1.2.7 (GPL-3.0).
> Not affiliated with, endorsed by, or sponsored by Tiny Craft.

<div align="center">

[![License](https://img.shields.io/github/license/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/releases)
![GitHub All Releases](https://img.shields.io/github/downloads/zhizuqiu/kvpilot/total)
[![GitHub stars](https://img.shields.io/github/stars/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/fork)

<strong>KVPilot is a modern lightweight cross-platform Redis desktop manager available for Mac, Windows, and Linux. It
also provides a web version that can be deployed via Docker.</strong>
</div>

<div align="center">
<img alt="KVPilot 仪表盘" src="docs/assets/03-browser.png" />

<img alt="KVPilot 键详情" src="docs/assets/02-key-detail.png" />

<img alt="KVPilot 偏好设置" src="docs/assets/04-preferences.png" />
</div>

## Feature

* Super lightweight, built on Webview2, without embedded browsers (Thanks
  to [Wails](https://github.com/wailsapp/wails)).
* Provides visually and user-friendly UI, light and dark themes (Thanks
  to [Naive UI](https://github.com/tusen-ai/naive-ui)
  and [IconPark](https://iconpark.oceanengine.com)).
* Multi-language support ([Need more languages ? Click here to contribute](.github/CONTRIBUTING.md)).
* Better connection management: supports SSH Tunnel/SSL/Sentinel Mode/Cluster Mode/HTTP proxy/SOCKS5 proxy.
* Visualize key value operations, CRUD support for Lists, Hashes, Strings, Sets, Sorted Sets, and Streams.
* Support multiple data viewing format and decode/decompression methods.
* Use SCAN for segmented loading, making it easy to list millions of keys.
* Logs list for command operation history.
* Provides command-line mode.
* Provides slow logs list.
* Segmented loading and querying for List/Hash/Set/Sorted Set.
* Provide value decode/decompression for List/Hash/Set/Sorted Set.
* Integrate with Monaco Editor
* Support real-time commands monitoring.
* Visual ACL management with permission dry-run and denial logs.
* Deployment topology for standalone, master-replica, sentinel and cluster.
* Support import/export data.
* Support publish/subscribe.
* Support import/export connection profile.
* Custom data encoder and decoder for value
  display ([Here are the instructions](https://github.com/zhizuqiu/kvpilot/blob/main/docs/custom-decoder.md)).

## Installation

Prebuilt packages for macOS (Apple Silicon / Intel), Windows and Linux are available on
the [releases page](https://github.com/zhizuqiu/kvpilot/releases).

> macOS builds are ad-hoc signed, so Gatekeeper may block the first launch. Remove the
> quarantine attribute once, then reopen:
> ``` shell
>  sudo xattr -d com.apple.quarantine /Applications/KVPilot.app
> ```

Prefer the browser? See [Docker Deployment](#docker-deployment) for the web version.

## Build Guidelines

### Prerequisites

* Go (latest version)
* Node.js >= 20
* NPM >= 9

### Install Wails

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Pull the Code

```bash
git clone https://github.com/zhizuqiu/kvpilot --depth=1
```

### Build Frontend

```bash
npm install --prefix ./frontend
```

or

```bash
cd frontend
npm install
```

### Compile and Run

```bash
wails dev
```

## Docker Deployment

> No official image is published yet. Build it locally from the repository root: `docker build -f Dockerfile.local -t kvpilot:1.2.8 .`


In addition to the desktop client, KVPilot also provides a web version that can be quickly deployed via Docker.

### Using Docker Compose (Recommended)

Create a `docker-compose.yml` file:

```yaml
services:
  kvpilot:
    image: kvpilot:1.2.8
    container_name: kvpilot
    restart: unless-stopped
    ports:
      - "8086:8086"
    environment:
      - ADMIN_USERNAME=admin
      - ADMIN_PASSWORD=change-me
    volumes:
      - ./data:/app/tinyrdm
```

Start the service:

```bash
docker compose up -d
```

Once started, visit `http://localhost:8086` and log in with the credentials configured above.

### Using Docker Command

```bash
docker run -d --name kvpilot \
  -p 8086:8086 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=change-me \
  -v ./data:/app/tinyrdm \
  kvpilot:1.2.8
```

### Environment Variables

| Variable         | Description    | Default |
|------------------|----------------|---------|
| `ADMIN_USERNAME` | Login username | -       |
| `ADMIN_PASSWORD` | Login password | -       |

## About

KVPilot is a community continuation of [Tiny RDM](https://github.com/tiny-craft/tiny-rdm) 1.2.7,
aimed at users who still rely on the open-source desktop client and need to keep up with
new Redis / KV compatible implementations.

- Scope: bug fixes, dependency and security updates, Redis protocol/command compatibility, hardening of the web mode
- Non-goals: major UI redesigns, chasing features of the closed-source successor (Redisee)
- Maintenance: issue/PR driven, see [Contributing](#contributing)
- License: GPL-3.0; unofficial derivative work, not affiliated with Tiny Craft

## Contributing

- Issues and feature requests: <https://github.com/zhizuqiu/kvpilot/issues>
- Upstream project: [Tiny RDM](https://github.com/tiny-craft/tiny-rdm); thanks to its authors
- License: [GPL-3.0](LICENSE); third-party notices in [NOTICE](NOTICE)
