## 2026-05-27

- 在 Codex sandbox 内运行 `go test ./...` 可能因为写入 `/Users/liushan/Library/Caches/go-build` 被拒绝；复跑时需要提升权限。
- 后端商业化迁移可用临时 PostgreSQL 验证，sandbox 会阻止 `initdb` 创建共享内存；启动本地临时 PG 需要提升权限。
