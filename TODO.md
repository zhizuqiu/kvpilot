# TODO

本文件记录跨版本的待办事项。已完成的条目直接删除，不在文件里保留历史。

## Windows 代码签名（SignPath Foundation）

**状态**：已于 2026-11-23 提交申请，等待审核（预计 1–3 周）。

**背景**：Windows 产物目前未签名，用户首次运行会看到 SmartScreen 提示，需要
「更多信息 → 仍要运行」。项目没有商业代码签名证书，因此申请 SignPath Foundation
的免费开源签名服务。

**审核期间可能被追问**

- KVPilot 与 Tiny RDM 的关系：是延续分支，但新增了 ACL 管理、部署拓扑、
  Web 模式加固与自动发布，不是纯 fork
- 产物来源：全部由 GitHub Actions 构建，无本地构建的签名产物
- 商业计划：无付费版，GPL-3.0

**通过后的落地步骤**

1. 接受 SignPath.io 组织邀请（Foundation 计划）
2. 创建 Project，建议 slug `kvpilot`
3. 创建 Signing Policy，建议 slug `release-signing`
4. 配置仓库 secrets：`SIGNPATH_API_TOKEN`、`SIGNPATH_ORGANIZATION_ID`
5. 在 `.github/workflows/release-windows.yaml` 中接入
   `signpath/github-action-submit-signing-request`，替换当前未签名产物的上传
6. 验证签名结果：`signtool verify /pa KVPilot.exe`，并确认 Release 附件为签名版本
7. 在 Release 说明与 README 安装章节补充「Windows 版本已通过 SignPath Foundation 签名」

**未通过时的备选**

- Certum Open Source Code Signing（约 €30/年，需提交开源证明）
- 继续未签名发布，在 Release 说明中说明如何绕过 SmartScreen

**相关文件**

- 申请材料：不在仓库内（含个人联系方式），保存在本机临时目录
- 签名开关：`release-windows.yaml` 中的 `WIN_CERT_AVAILABLE` 条件
