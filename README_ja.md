<div align="center">
<a href="https://github.com/zhizuqiu/kvpilot/"><img src="build/appicon.png" width="120"/></a>
</div>

<h1 align="center">KVPilot</h1>
<h4 align="center"><a href="/">English</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_zh.md">简体中文</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_tw.md">繁體中文</a> | <strong>日本語</strong> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ko.md">한국어</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_fr.md">Français</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_es.md">Español</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_pt.md">Português (BR)</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ru.md">Русский</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_tr.md">Türkçe</a></h4>

> **KVPilot** is an unofficial community continuation of **Tiny RDM** 1.2.7 (GPL-3.0).
> Not affiliated with, endorsed by, or sponsored by Tiny Craft.

<div align="center">

[![License](https://img.shields.io/github/license/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/releases)
![GitHub All Releases](https://img.shields.io/github/downloads/zhizuqiu/kvpilot/total)
[![GitHub stars](https://img.shields.io/github/stars/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/fork)

<strong>Tiny
RDMは、Mac、Windows、Linuxで利用可能な、モダンで軽量なクロスプラットフォームのRedisデスクトップマネージャーです。Docker経由でデプロイ可能なWeb版も提供しています。</strong>
</div>

<div align="center">
<img alt="KVPilot 仪表盘" src="docs/assets/03-browser.png" />

<img alt="KVPilot 键详情" src="docs/assets/02-key-detail.png" />

<img alt="KVPilot 偏好设置" src="docs/assets/04-preferences.png" />
</div>

## 特徴

* 超軽量、Webview2をベースにしており、埋め込みブラウザなし（[Wails](https://github.com/wailsapp/wails)に感謝）。
* 視覚的でユーザーフレンドリーなUI、ライトとダークテーマを提供（[Naive UI](https://github.com/tusen-ai/naive-ui)
  と[IconPark](https://iconpark.oceanengine.com)に感謝）。
* 多言語サポート（[もっと多くの言語が必要ですか？ここをクリックして貢献してください](.github/CONTRIBUTING.md)）。
* より良い接続管理：SSHトンネル/SSL/センチネルモード/クラスターモード/HTTPプロキシ/SOCKS5プロキシをサポート。
* キー値操作の可視化、リスト、ハッシュ、文字列、セット、ソートセット、ストリームのCRUDサポート。
* 複数のデータ表示形式とデコード/解凍方法をサポート。
* SCANを使用してセグメント化された読み込みを行い、数百万のキーを簡単にリスト化。
* コマンド操作履歴のログリスト。
* コマンドラインモードを提供。
* スローログリストを提供。
* リスト/ハッシュ/セット/ソートセットのセグメント化された読み込みとクエリ。
* リスト/ハッシュ/セット/ソートセットの値のデコード/解凍を提供。
* Monaco Editorと統合。
* リアルタイムコマンド監視をサポート。
* データのインポート/エクスポートをサポート。
* パブリッシュ/サブスクライブをサポート。
* 接続プロファイルのインポート/エクスポートをサポート。
*
値表示のためのカスタムデータエンコーダーとデコーダーをサポート（[こちらが手順です](https://github.com/zhizuqiu/kvpilot/blob/main/docs/custom-decoder.md)）。

## インストール

[こちら](https://github.com/zhizuqiu/kvpilot/releases)から無料でダウンロードできます。

> macOSにインストール後に開けない場合、**信頼されていない**または**ゴミ箱に移動**というエラーが表示された場合は、以下のコマンドを実行してから再度開いてください：
> ``` shell
>  sudo xattr -d com.apple.quarantine /Applications/Tiny\ RDM.app
> ```

## ビルドガイドライン

### 前提条件

* Go（最新バージョン）
* Node.js >= 20
* NPM >= 9

### Wailsのインストール

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### コードの取得

```bash
git clone https://github.com/zhizuqiu/kvpilot --depth=1
```

### フロントエンドのビルド

```bash
npm install --prefix ./frontend
```

または

```bash
cd frontend
npm install
```

### コンパイルと実行

```bash
wails dev
```

## Dockerデプロイ

> No official image is published yet. Build it locally from the repository root: `docker build -f Dockerfile.local -t kvpilot:1.2.8 .`


デスクトップクライアントに加えて、KVPilotはDockerで素早くデプロイできるWeb版も提供しています。

### Docker Composeを使用（推奨）

`docker-compose.yml` ファイルを作成します：

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

サービスを起動します：

```bash
docker compose up -d
```

起動後、`http://localhost:8086` にアクセスし、上記で設定したユーザー名とパスワードでログインしてください。

### Dockerコマンドを使用

```bash
docker run -d --name kvpilot \
  -p 8086:8086 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=change-me \
  -v ./data:/app/tinyrdm \
  kvpilot:1.2.8
```

### 環境変数の説明

| 変数               | 説明        | デフォルト値 |
|------------------|-----------|--------|
| `ADMIN_USERNAME` | ログインユーザー名 | -      |
| `ADMIN_PASSWORD` | ログインパスワード | -      |

## Contributing

- Issues and feature requests: <https://github.com/zhizuqiu/kvpilot/issues>
- Upstream project: [Tiny RDM](https://github.com/tiny-craft/tiny-rdm); thanks to its authors
- License: [GPL-3.0](LICENSE); third-party notices in [NOTICE](NOTICE)
