<div align="center">
<a href="https://github.com/zhizuqiu/kvpilot/"><img src="build/appicon.png" width="120"/></a>
</div>

<h1 align="center">KVPilot</h1>
<h4 align="center"><a href="/">English</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_zh.md">简体中文</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_tw.md">繁體中文</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ja.md">日本語</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ko.md">한국어</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_fr.md">Français</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_es.md">Español</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_pt.md">Português (BR)</a> | <strong>Русский</strong> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_tr.md">Türkçe</a></h4>

> **KVPilot** is an unofficial community continuation of **Tiny RDM** 1.2.7 (GPL-3.0).
> Not affiliated with, endorsed by, or sponsored by Tiny Craft.

<div align="center">

[![License](https://img.shields.io/github/license/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/releases)
![GitHub All Releases](https://img.shields.io/github/downloads/zhizuqiu/kvpilot/total)
[![GitHub stars](https://img.shields.io/github/stars/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/fork)

<strong>KVPilot — современный легковесный кроссплатформенный менеджер Redis для Mac, Windows и Linux. Также доступна
веб-версия с возможностью развёртывания через Docker.</strong>
</div>

<div align="center">
<img alt="KVPilot dashboard" src="docs/assets/03-browser.png" />

<img alt="KVPilot preferences" src="docs/assets/04-preferences.png" />
</div>

## Возможности

* Сверхлёгкий, на базе Webview2, без встроенного браузера (Спасибо [Wails](https://github.com/wailsapp/wails))
* Визуально приятный и удобный интерфейс, светлая и тёмная темы (
  Спасибо [Naive UI](https://github.com/tusen-ai/naive-ui) и [IconPark](https://iconpark.oceanengine.com))
* Поддержка нескольких языков ([Нужно больше языков? Нажмите здесь, чтобы помочь](.github/CONTRIBUTING.md))
* Улучшенное управление подключениями: SSH-туннель/SSL/режим Sentinel/режим Cluster/HTTP-прокси/SOCKS5-прокси
* Визуализация операций с ключами, поддержка CRUD для List, Hash, String, Set, Sorted Set и Stream
* Поддержка множества форматов отображения и методов декодирования/распаковки
* Сегментированная загрузка через SCAN для удобной работы с миллионами ключей
* Журнал истории выполненных команд
* Режим командной строки
* Список медленных запросов
* Сегментированная загрузка и запросы для List/Hash/Set/Sorted Set
* Декодирование/распаковка значений для List/Hash/Set/Sorted Set
* Интеграция с Monaco Editor
* Мониторинг команд в реальном времени
* Импорт/экспорт данных
* Публикация/подписка
* Импорт/экспорт профилей подключений
* Пользовательские кодировщики и декодировщики для отображения
  значений ([Инструкция](https://github.com/zhizuqiu/kvpilot/blob/main/docs/custom-decoder.md))

## Установка

Доступно для бесплатного скачивания [здесь](https://github.com/zhizuqiu/kvpilot/releases).

> Если после установки на macOS приложение не открывается, выполните следующую команду и попробуйте снова:
> ``` shell
>  sudo xattr -d com.apple.quarantine /Applications/Tiny\ RDM.app
> ```

## Руководство по сборке

### Требования

* Go (последняя версия)
* Node.js >= 20
* NPM >= 9

### Установка Wails

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Получение кода

```bash
git clone https://github.com/zhizuqiu/kvpilot --depth=1
```

### Сборка фронтенда

```bash
npm install --prefix ./frontend
```

или

```bash
cd frontend
npm install
```

### Компиляция и запуск

```bash
wails dev
```

## Развёртывание через Docker

Помимо десктопного клиента, KVPilot предоставляет веб-версию, которую можно быстро развернуть через Docker.

### С помощью Docker Compose (рекомендуется)

Создайте файл `docker-compose.yml`:

```yaml
services:
  tinyrdm:
    image: ghcr.io/zhizuqiu/kvpilot:latest
    container_name: tinyrdm
    restart: unless-stopped
    ports:
      - "8086:8086"
    environment:
      - ADMIN_USERNAME=admin
      - ADMIN_PASSWORD=tinyrdm
    volumes:
      - ./data:/app/tinyrdm
```

Запустите сервис:

```bash
docker compose up -d
```

После запуска откройте `http://localhost:8086` и войдите с указанными выше учётными данными.

### С помощью команды Docker

```bash
docker run -d --name tinyrdm \
  -p 8086:8086 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=tinyrdm \
  -v ./data:/app/tinyrdm \
  ghcr.io/zhizuqiu/kvpilot:latest
```

### Переменные окружения

| Переменная       | Описание         | По умолчанию |
|------------------|------------------|--------------|
| `ADMIN_USERNAME` | Имя пользователя | -            |
| `ADMIN_PASSWORD` | Пароль           | -            |

## Спонсоры

Благодарим следующих поставщиков услуг за спонсорство хостинга

<table>
<tr>
<td width="200"><a href="https://flymux.com/register?promo=TINYRDM"><img alt="FlyMux" src="docs/images/flymux_logo.png"/></a></td>
<td>Отдельная благодарность FlyMux за спонсорскую поддержку этого проекта! FlyMux специализируется на предоставлении официальных и высокостабильных relay-сервисов для Claude Code и Codex, помогая разработчикам получить удобный и эффективный опыт интеграции AI-инструментов. Специально для пользователей TinyRDM: зарегистрируйтесь по <a href="https://flymux.com/register?promo=TINYRDM">этой ссылке</a>, и на ваш аккаунт автоматически будет начислен бонусный кредит $5.</td>
</tr>
<tr>
<td width="200"><a href="https://www.notidc.com/"><img src="docs/images/notidc_logo.png" alt="NotiDC"></a></td>
<td>Отдельная благодарность NotiDC за спонсорскую поддержку этого проекта! NotIDC предоставляет высокопроизводительную облачную инфраструктуру, включая облачные серверы, bare metal, CDN и решения безопасности, с глобальным покрытием сети и надёжной защитой от DDoS, помогая разработчикам эффективно развёртывать и масштабировать приложения.</td>
</tr>
</table>

## О проекте

### Официальный аккаунт WeChat

<img src="docs/images/wechat_official.png" alt="wechat" width="360" />
