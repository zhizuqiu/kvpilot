# TODO

本文件记录跨版本的待办事项。已完成的条目直接删除，不在文件里保留历史。

## Windows 代码签名（SignPath Foundation）

**状态**：2026-11-23 申请被拒。

**拒绝原因（原文要点）**：Foundation 面向已有一定公共信任度的项目，评估依据是社区采用度
（star、fork、贡献者）、外部文章与独立讨论（Reddit / Stack Overflow / YouTube 等）、
机构背书，以及持续活跃度的证据；当前项目这些信号不足。官方说明这不代表对项目质量或
潜力的判断，欢迎项目获得更广泛认知后重新申请。

**重新申请前的自查清单**

- [ ] GitHub star 与 fork 达到一定规模（数百 star 量级更稳妥）
- [ ] 存在外部独立提及：技术文章、Reddit / Hacker News / V2EX 讨论、视频演示等
- [ ] 有外部贡献者（非维护者本人的 PR 或 issue）
- [ ] 有持续发布节奏（多个版本、时间跨度足够）
- [ ] 英文文档与落地页完善

**提升可见度的待办**

- [ ] 发布公告：Reddit（r/redis、r/selfhosted）、Hacker News Show HN、V2EX、掘金/知乎
- [ ] 向 awesome 列表提 PR：awesome-redis、awesome-selfhosted、awesome-developer-tools
- [ ] 写一篇英文技术文章（延续分支的取舍、ACL 与拓扑设计），发布到 dev.to / Medium
- [ ] 补充英文使用场景与截图，保持仓库 About 关键词
- [ ] 建立 GitHub Pages 落地页，展示安装方式与 Docker 拉取量
- [ ] 标注 good first issue，吸引外部贡献者

**通过签名后的落地步骤**

1. 接受 SignPath.io 组织邀请
2. 创建 Project（`kvpilot`）与 Signing Policy（`release-signing`）
3. 配置 secrets：`SIGNPATH_API_TOKEN`、`SIGNPATH_ORGANIZATION_ID`
4. 在 `release-windows.yaml` 接入 `signpath/github-action-submit-signing-request`
5. 校验签名（`signtool verify /pa`）并替换 Release 附件
6. Release 说明与 README 注明已签名

**当前可选替代方案**

| 方案 | 成本 | 说明 |
| --- | --- | --- |
| 保持未签名 | 0 | README 已说明 SmartScreen 绕过方式 |
| Certum Open Source Code Signing | 约数十欧元/年 | 需提供开源项目证明，价格与政策以官网为准 |
| SignPath 付费订阅 | 按官网计费 | 可立即使用，不受 Foundation 可见度要求限制 |
| Azure Trusted Signing | 约每月 $10 | 需企业资质与经营年限证明，个人通常不适用 |

**相关文件**

- 申请材料：不在仓库内（含个人联系方式）
- 签名开关：`release-windows.yaml` 中的 `WIN_CERT_AVAILABLE` 条件

## 客户端运维能力

按优先级排列，完成后删除对应条目。

### P3 慢日志与落盘

- [ ] 慢日志页支持 `SLOWLOG RESET`，执行前确认并提示不可恢复
- [ ] 提供手动落盘：逐节点 `BGSAVE`，展示 `LASTSAVE`、`rdb_last_bgsave_status` 与耗时
- [ ] 落盘操作区分「已触发」与「已完成」，避免把后台任务当成立即成功

### P3 Redis 层巡检

- [ ] 规则集（只读）：内存碎片率、`maxmemory` 与 `evicted_keys`、持久化状态、复制延迟与链路状态、`blocked_clients`、慢日志阈值、危险配置（`protected-mode no`、`save ""`、`appendonly no`、空密码、危险命令未重命名）、集群状态
- [ ] 输出分项结果、命中项的建议动作，并支持跳转到状态/配置/拓扑页签
- [ ] 支持按严重级别过滤，结果可导出

### 待评估

- [ ] 配置基线：定义团队标准参数集，与当前实例对比并列出偏离项，复用配置页的差异展示
- [ ] 危险操作预览：`FLUSHDB`、按模式批量删除、多节点 `CONFIG SET`、会话 kill 前展示影响范围并要求勾选确认
- [ ] 从节点读流量检查：对比主从 `INFO commandstats`，识别未承接读请求的副本
- [ ] 本地操作审计：写操作记录到本地 JSONL（时间、连接、命令、结果），复用现有脱敏规则
- [ ] 热 key 分析：需要 LFU 前提或独立采样方案