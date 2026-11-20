# AGENTS.md

本文件是 AI 工具和协作者进入 KVPilot 仓库后的统一入口规范，包含目录路由、通用约束、TDD 研发模式和构建方式。

规范冲突时的优先级：**本文件 > 通用最佳实践**。如果本文件与某个技能的默认流程冲突，按本文件执行。

## 1. 项目定位

KVPilot 是 Tiny RDM 的社区延续版，GPL-3.0 授权，面向 Redis 及 Redis 兼容存储的跨平台桌面客户端，同时提供 Web 模式。

- 上游仓库：`upstream` = `tiny-craft/tiny-rdm`（只读，push 已禁用）
- 本项目仓库：`origin` = `zhizuqiu/kvpilot`
- 技术栈：后端 Go 1.25 + Wails v2；前端 Vue 3 + Vite + Naive UI

## 2. 目录路由

| 路径 | 职责 |
| --- | --- |
| `main.go` | 桌面端入口（`//go:build !web`），Wails 应用装配与 service 绑定 |
| `main_web.go` | Web 端入口（`//go:build web`），仅启动 HTTP 服务 |
| `backend/api/` | Gin 路由层，每个文件对应一组路由；`router.go` 负责装配 |
| `backend/services/` | 业务逻辑层，是主要研发区域 |
| `backend/storage/` | 连接配置、偏好设置的本地持久化 |
| `backend/types/` | 跨层共享的数据结构；`JSResp` 是统一返回体 |
| `backend/utils/` | 无业务语义的工具包，最容易被单测覆盖 |
| `frontend/src/components/` | 按区域划分：`content/`、`content_value/`、`sidebar/`、`dialogs/`、`common/`、`icons/` |
| `frontend/src/utils/api.js` | Web 模式的 HTTP/WebSocket 适配层 |
| `frontend/src/langs/` | 11 种语言的文案 |
| `frontend/src/consts/` | 前端常量，例如浏览器标签类型 |
| `docker/`、`Dockerfile*` | 部署物料与镜像构建 |
| `docs/` | 面向用户的使用文档 |

## 3. 通用约束

### 3.1 Git

1. **不要自行提交。** 研发完成、测试通过后停在工作区，等明确指示再 `git commit`。
2. Commit message 使用英文 Conventional Commits：`<type>(<scope>): <description>`，例如 `feat(acl): add ACL management panel`。
3. 提交前必须确认暂存区只包含本次任务相关文件，不得混入无关改动。
4. 不得 `push`，除非明确要求。
5. 不得使用 `git reset --hard`、宽范围 `git checkout`、`git add -A` 之类可能吞掉未提交改动的操作。
6. 不修改上游历史代码的格式与风格来"顺手清理"，改动范围要能一句话说清。

### 3.2 数据安全

1. **不得擅自删除用户数据**：Docker 容器、volume、本地连接配置（`~/Library/Preferences/TinyRDM/connections.yaml` 等）、`preferences.yaml`。需要清理时先说明影响范围并征得同意。
2. 不得把真实密码、密钥、token 写进仓库文件、注释、日志或截图。
3. 需要破坏性操作（重建、down -v、覆盖配置）时，先备份到明确路径并告知位置。

### 3.3 前端 UI

1. 保持与现有界面一致：沿用 Naive UI 组件、现有栅格和间距、现有 CSS 变量与配色。新增元素先看同类页面是怎么做的。
2. **禁止紫色、蓝紫渐变等"AI 感"配色**，也禁止引入新的 UI 组件库或图标库。
3. 本项目 `$dialog` 是自定义封装，只支持 `$dialog.warning(content, onConfirm)`，不支持 Naive UI 的选项对象写法。
4. 用户可见文案必须同时更新 `frontend/src/langs/` 下各语言文件（至少 `en-us`、`zh-cn`、`zh-tw`，其余语言保持键对齐）。
5. vue-i18n 文案中**禁止出现裸 `@`**（例如 `+@all`），会抛 `SyntaxError` 并导致整页空白；需要表达时改用自然语言描述。
6. 截图、演示数据不得包含深色模式界面、真实业务数据或地理位置信息。
7. 新增可交互元素必须考虑空态、加载态、错误态，不能只做成功路径。

### 3.4 双模式架构（桌面 + Web）

前端默认调用 Wails 生成的 `wailsjs/go/services/*` 绑定；Web 模式下由 `vite.config.js` 的 alias 重定向到 `frontend/src/utils/api.js`。

因此**新增后端能力必须在三处同步落地**，缺一处 Web 模式就会静默失效：

1. `backend/services/xxx_service.go`：实现业务方法，返回 `types.JSResp`
2. `backend/api/xxx_api.go`：注册 Gin 路由，解析请求并调用 service
3. `frontend/src/utils/api.js`：导出同名函数，走 `post`/`get`/`del` 适配器

## 4. 研发模式：TDD

**铁律：任何新功能或 bug 修复，都必须先写出会失败的测试，再写实现。** 没有失败测试的改动不允许提交。

### 4.1 循环

1. **红**：先写测试并运行，确认它因"功能缺失"而失败，而不是因为语法错误、导入错误或环境问题失败。把失败信息看清楚。
2. **绿**：写满足测试的最少代码，不做超前设计。
3. **重构**：测试保持全绿的前提下整理实现，去掉重复和临时变量。
4. 一个测试对应一个行为。测试名直接描述行为，而不是描述函数名。

### 4.2 各层测试边界

| 层 | 位置 | 方式 |
| --- | --- | --- |
| `backend/utils/` | 同包 `xxx_test.go` | 纯单元测试，表驱动 |
| `backend/services/` | 同包 `xxx_test.go` | 依赖 Redis 的逻辑见 4.3 |
| `backend/storage/` | 同包 `xxx_test.go` | 使用 `t.TempDir()` 隔离文件 |
| 前端纯函数 | `frontend/src/utils/__tests__/*.spec.js` | Vitest |
| Vue 组件行为 | 组件同目录 `__tests__/*.spec.jsx` | Vitest + `@vue/test-utils`，JSX 书写 |

仅涉及视觉样式、纯文案、依赖版本升级的改动可以豁免测试，但必须在交付说明里写明豁免原因。

### 4.3 依赖 Redis 的测试

1. 优先用 `miniredis` 覆盖命令语义与边界，测试不依赖外部环境。
2. 需要真实拓扑（哨兵/集群/ACL）的测试，从环境变量读取地址：`KVPILOT_TEST_REDIS_ADDR`、
   `KVPILOT_TEST_REDIS_PASSWORD`，集群再加 `KVPILOT_TEST_REDIS_CLUSTER=1`；
   未设置地址时用 `t.Skip` 跳过，禁止让默认 `go test ./...` 连外网或内网主机。
   文件名统一用 `*_integration_test.go`，用例名以 `TestIntegration` 开头，便于单独运行。
3. 集群相关逻辑必须覆盖"部分节点失败"和"节点间数据不一致"两类场景，这是历史上最容易出错的地方。
4. go-redis 的 `ForEachShard` 是**并发执行**的（每个节点一个 goroutine）：回调内不得直接读写共享变量，
   累加或 append 必须加锁；依赖"回调按顺序执行"的写法一律视为缺陷。
   涉及分片读写的代码必须在 `go test -race` 下跑一遍集群集成测试。
5. 涉及写命令的集成测试必须使用唯一命名的临时对象（例如 `kvpilot_probe_<纳秒时间戳>`），
   用 `t.Cleanup` 清理，禁止修改 `default` 或任何已有用户/数据。
6. 客户端的 `Close` 也要注册到 `t.Cleanup` 且先于业务清理注册（`t.Cleanup` 后进先出），
   不要用 `defer client.Close()`：`defer` 会先执行，导致清理时连接已关闭、错误被静默吞掉。
7. 测试之间不得共享状态：每个用例自己建 key、自己清理。

### 4.4 测试质量

1. 每个测试用例必须有中文注释，说明**验证目标或边界条件**，不要复述代码字面意思。
2. 不写恒真断言、不写只断言"没报错"的测试。
3. 不引入 flaky 因素：不依赖 `sleep` 等待、不依赖执行顺序、不依赖当前时间。
4. 修复 bug 时，测试必须先复现该 bug；修复后该测试永久保留，作为回归保护。

## 5. 构建与验证

### 5.1 依赖工具

- Go 1.25（`go.mod` 要求 `go 1.25.0`）
- Node.js + npm（前端依赖在 `frontend/`）
- Wails CLI（仅桌面端构建需要）
- 本地 macOS 若 `go` 版本偏低，使用 `GOTOOLCHAIN=go1.25.6` 与 `GOFLAGS=-mod=mod`

前端测试栈已接入：Vitest + `@vue/test-utils` + jsdom，配置在 `frontend/vite.config.js` 的 `test` 段，
测试文件匹配 `src/**/__tests__/*.spec.{js,jsx}`。已启用 `@vitejs/plugin-vue-jsx`，
组件测试可直接用 JSX 书写；注意该插件只处理 `.jsx`/`.tsx`，`.js` 里写 JSX 会解析失败。

### 5.2 常用命令

```bash
# 后端编译与全量单测（提交前必跑）
go build ./...
go test ./...

# 真实 Redis 集成测试（默认跳过，需显式指定地址）
KVPILOT_TEST_REDIS_ADDR=127.0.0.1:6379 KVPILOT_TEST_REDIS_PASSWORD=xxx \
    go test ./backend/services/ -run Integration -v
# 集群模式额外加 KVPILOT_TEST_REDIS_CLUSTER=1

# 前端单测（提交前必跑）
cd frontend && npm run test

# 前端构建
cd frontend && npx vite build

# 桌面端开发 / 构建
wails dev
wails build

# Web 模式本地运行
cd frontend && VITE_WEB=true npx vite build
go build -tags web -o /tmp/kvpilot-web . && PORT=8088 /tmp/kvpilot-web
```

### 5.3 交付前自检

1. `go build ./...` 与 `go test ./...` 通过。
2. `cd frontend && npm run test` 通过。
3. `npx vite build` 通过，无新增编译告警。
4. Go 侧改动文件通过 `gofmt`（只格式化本次改动文件，不批量重排历史文件）。
5. 新增后端能力已按 3.4 在三处同步落地；涉及集群分片读写的改动已在 `-race` 下验证。
6. 交付说明中列出：修改文件路径、核心改动、验证方式；能用 `redis-cli` 或 curl 复现的给出具体命令与预期结果。

## 6. 部署与远程环境

1. 远程主机上的部署物料按项目分目录存放，包含 compose 文件、配置与 README，不与其他项目混放。
2. 部署前先确认目标主机已有的容器、端口和目录占用，避免与本项目容器重名或抢端口。
3. 执行 `down`、删除 volume、覆盖配置等破坏性操作前，先确认数据可丢弃并告知影响范围。

## 7. 文档

1. 面向用户的能力说明写进 `README*.md` 与 `docs/`。
2. 命令行为、兼容性差异、边界取舍写进代码注释或对应 service 文件的顶部注释。
3. 文档中的命令与端口必须可执行、与代码一致，改代码时同步改文档。
