# Changelog

本文件记录 KVPilot 的版本变更。KVPilot 是 [Tiny RDM](https://github.com/tiny-craft/tiny-rdm)
1.2.7 的社区延续分支（GPL-3.0），版本号从 1.2.8 起延续。

## [1.2.8] - 2026-11-19

KVPilot 首个版本：延续 Tiny RDM 1.2.7 的桌面与 Web 形态，先修可用性问题，不做界面重构。

### 修复

- **哨兵模式「自动查询组名」恒失败**：`SENTINEL MASTERS` 在 RESP2 下返回扁平数组，
  原实现按 RESP3 的 map 解析，类型断言恒失败导致返回空列表（界面提示 `list sentinel master fail`）。
  改用 `MapStringStringSliceCmd`，同时兼容 RESP2/RESP3，并去掉可能 panic 的裸类型断言。
- 补全 `go.sum` 中缺失的模块校验行，新克隆的仓库可直接编译。

### 变更

- 品牌重命名为 **KVPilot**：应用名、窗口标题、关于对话框、10 种语言文案、安装包与产物名；
  应用标识由 `com.tinycraft.*` 改为 `com.kvpilot.*`。
- 暂时隐藏左下角的微信公众号与 X（Twitter）入口（代码保留，`Ribbon.vue` 中
  `SHOW_WECHAT_ENTRY` 与 `SHOW_SOCIAL_ENTRY` 置为 `false`）。
- 移除「新建连接」空状态下的赞助商广告位（同时删除其数据拉取与本地缓存逻辑）。
- 检查更新改为请求本仓库的 GitHub Releases API（`api.github.com/repos/zhizuqiu/kvpilot`），
  不再请求上游 `tinyrdm.com`，避免把上游版本当作本项目更新提示；无 release 时提示"已是最新版本"。
- 匿名使用统计默认关闭，且本地构建不注入统计 ID。
- 「使用指南」「隐私策略」「自定义解码帮助」全部改为 GitHub 链接，并新增
  `docs/privacy.md`、`docs/custom-decoder.md`。
- 固定 Go 1.25.x 构建工具链，修复 macOS 26+ 上二进制因缺少 `LC_UUID` 无法启动的问题。
- 数据目录仍沿用 `TinyRDM/`，从 Tiny RDM 升级零迁移；计划在 1.3.0 迁移到 `KVPilot/`。

### 新增

- 原生 arm64 Dockerfile（`Dockerfile.local`），Apple Silicon 无需 QEMU 模拟即可构建 Web 镜像。
- `NOTICE` 说明 fork 来源、许可证与修改内容。
- 原生 arm64 镜像构建验证与 Web 模式端到端验证（登录 → 哨兵组名自动发现）。
