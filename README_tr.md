<div align="center">
<a href="https://github.com/zhizuqiu/kvpilot/"><img src="build/appicon.png" width="120"/></a>
</div>

<h1 align="center">KVPilot</h1>
<h4 align="center"><a href="/">English</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_zh.md">简体中文</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_tw.md">繁體中文</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ja.md">日本語</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ko.md">한국어</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_fr.md">Français</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_es.md">Español</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_pt.md">Português (BR)</a> | <a href="https://github.com/zhizuqiu/kvpilot/blob/main/README_ru.md">Русский</a> | <strong>Türkçe</strong></h4>

> **KVPilot** is an unofficial community continuation of **Tiny RDM** 1.2.7 (GPL-3.0).
> Not affiliated with, endorsed by, or sponsored by Tiny Craft.

<div align="center">

[![License](https://img.shields.io/github/license/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/releases)
![GitHub All Releases](https://img.shields.io/github/downloads/zhizuqiu/kvpilot/total)
[![GitHub stars](https://img.shields.io/github/stars/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/zhizuqiu/kvpilot)](https://github.com/zhizuqiu/kvpilot/fork)

<strong>KVPilot, Mac, Windows ve Linux için kullanılabilen modern, hafif ve çapraz platform bir Redis masaüstü
yöneticisidir. Docker ile dağıtılabilen bir web sürümü de sunmaktadır.</strong>
</div>

<div align="center">
<img alt="KVPilot dashboard" src="docs/assets/03-browser.png" />

<img alt="KVPilot preferences" src="docs/assets/04-preferences.png" />
</div>

## Özellikler

* Ultra hafif, Webview2 tabanlı, gömülü tarayıcı yok ([Wails](https://github.com/wailsapp/wails)'e teşekkürler)
* Görsel ve kullanıcı dostu arayüz, açık ve koyu tema desteği ([Naive UI](https://github.com/tusen-ai/naive-ui)
  ve [IconPark](https://iconpark.oceanengine.com)'a teşekkürler)
* Çoklu dil desteği ([Daha fazla dil mi gerekiyor? Katkıda bulunmak için tıklayın](.github/CONTRIBUTING.md))
* Gelişmiş bağlantı yönetimi: SSH Tüneli/SSL/Sentinel Modu/Cluster Modu/HTTP proxy/SOCKS5 proxy desteği
* Anahtar-değer işlemlerinin görselleştirilmesi, List, Hash, String, Set, Sorted Set ve Stream için CRUD desteği
* Çoklu veri görüntüleme formatı ve çözme/sıkıştırma açma yöntemleri desteği
* SCAN ile segmentli yükleme, milyonlarca anahtarı kolayca listeleme
* Komut işlem geçmişi günlük listesi
* Komut satırı modu
* Yavaş günlük listesi
* List/Hash/Set/Sorted Set için segmentli yükleme ve sorgulama
* List/Hash/Set/Sorted Set değerleri için çözme/sıkıştırma açma
* Monaco Editor entegrasyonu
* Gerçek zamanlı komut izleme desteği
* Veri içe/dışa aktarma desteği
* Yayınla/abone ol desteği
* Bağlantı profili içe/dışa aktarma desteği
* Değer görüntüleme için özel veri kodlayıcı ve çözücü ([Talimatlar burada](https://github.com/zhizuqiu/kvpilot/blob/main/docs/custom-decoder.md))

## Kurulum

[Buradan](https://github.com/zhizuqiu/kvpilot/releases) ücretsiz olarak indirilebilir.

> macOS'ta kurulumdan sonra açamıyorsanız, aşağıdaki komutu çalıştırıp tekrar açın:
> ``` shell
>  sudo xattr -d com.apple.quarantine /Applications/Tiny\ RDM.app
> ```

## Derleme Kılavuzu

### Gereksinimler

* Go (en son sürüm)
* Node.js >= 20
* NPM >= 9

### Wails Kurulumu

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Kodu Çekme

```bash
git clone https://github.com/zhizuqiu/kvpilot --depth=1
```

### Frontend Derleme

```bash
npm install --prefix ./frontend
```

veya

```bash
cd frontend
npm install
```

### Derleme ve Çalıştırma

```bash
wails dev
```

## Docker ile Dağıtım

Masaüstü istemcisinin yanı sıra, KVPilot Docker ile hızlıca dağıtılabilen bir web sürümü de sunmaktadır.

### Docker Compose Kullanımı (önerilen)

Bir `docker-compose.yml` dosyası oluşturun:

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

Servisi başlatın:

```bash
docker compose up -d
```

Başlatıldıktan sonra `http://localhost:8086` adresini ziyaret edin ve yukarıda yapılandırılan kimlik bilgileriyle giriş
yapın.

### Docker Komutu Kullanımı

```bash
docker run -d --name tinyrdm \
  -p 8086:8086 \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=tinyrdm \
  -v ./data:/app/tinyrdm \
  ghcr.io/zhizuqiu/kvpilot:latest
```

### Ortam Değişkenleri

| Değişken         | Açıklama            | Varsayılan |
|------------------|---------------------|------------|
| `ADMIN_USERNAME` | Giriş kullanıcı adı | -          |
| `ADMIN_PASSWORD` | Giriş şifresi       | -          |

## Sponsorluk

Barındırma sponsorluğu sağlayan aşağıdaki hizmet sağlayıcılarına teşekkür ederiz

<table>
<tr>
<td width="200"><a href="https://flymux.com/register?promo=TINYRDM"><img alt="FlyMux" src="docs/images/flymux_logo.png"/></a></td>
<td>Bu projeye sponsor olduğu için FlyMux'a özel teşekkürler! FlyMux, Claude Code ve Codex için resmi ve yüksek kararlılığa sahip relay hizmetleri sunmaya odaklanır; geliştiriciler için sorunsuz ve verimli bir entegrasyon deneyimi oluşturur. TinyRDM kullanıcılarına özel olarak, <a href="https://flymux.com/register?promo=TINYRDM">bu bağlantı</a> üzerinden kayıt olduğunuzda hesabınıza otomatik olarak $5 bonus kredi tanımlanır.</td>
</tr>
<tr>
<td width="200"><a href="https://www.notidc.com/"><img src="docs/images/notidc_logo.png" alt="NotiDC"></a></td>
<td>Bu projeye sponsor olduğu için NotiDC'a özel teşekkürler! NotIDC, bulut sunucuları, bare metal, CDN ve güvenlik çözümleri dahil yüksek performanslı bulut altyapısı sunar; küresel ağ kapsamı ve güvenilir DDoS korumasıyla geliştiricilerin uygulamalarını verimli şekilde dağıtmasına ve ölçeklemesine yardımcı olur.</td>
</tr>
</table>

## Hakkında

### WeChat Resmi Hesabı

<img src="docs/images/wechat_official.png" alt="wechat" width="360" />
