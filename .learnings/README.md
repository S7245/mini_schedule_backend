# backend 本地学习记录

本目录关联 `self-improving-agent` 的项目级记忆机制。

## 关联信息

- project: `/Users/liushan/Documents/zkw/mini_schedule/backend`
- local memory: `/Users/liushan/Documents/zkw/mini_schedule/backend/.learnings`
- skill router: `/Users/liushan/Documents/zkw/mini_schedule/backend/SKILLS.md`

## 开发前技能选择规则

在 `/backend` 开发前，如果需要使用 skills，先读取 `backend/SKILLS.md` 的摘要，再选择当前会话实际可用的 skill。

常见选择：

- Go 分层、包组织、命名：`go-packages`、`go-naming`、`go-style-core`
- Go 错误、Context、事务流程：`go-error-handling`、`go-context`、`go-defensive`
- Go 测试：`go-testing`
- 数据库/迁移：`database-schema-designer`、`ddia-systems`
- 架构和领域建模：`clean-architecture`、`domain-driven-design`
- 调试和回归：`diagnose`、`go-code-review`

## 文件用途

- `LEARNINGS.md`：记录后端项目经验、验证注意事项和可复用实践。
- `ERRORS.md`：记录命令失败、测试失败、迁移失败和修复线索。
- `FEATURE_REQUESTS.md`：记录后端能力补齐或 skill 改进请求。

跨项目通用经验再考虑进入 `self-improving-agent` 全局 memory。
