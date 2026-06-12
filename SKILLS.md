# 项目可用 SKILLS 参考指南

> 基于当前项目技术栈：Go + Gin + GORM + PostgreSQL + Redis 的 SaaS 多品牌入驻后

---

| Skill | Summary | Description |
|-------|---------|-------------|
| **Go 语言核心** | | |
| `go-code-review` | Go 代码审查 | 审查 Go 代码是否符合社区风格标准，适用于 PR 提交前后 |
| `go-concurrency` | Go 并发编程 | goroutines、channels、mutexes，适用于 Worker Pool 异步任务 |
| `go-context` | Context 使用 | context.Context 传递、取消、超时，适用于请求链路和数据库操作 |
| `go-control-flow` | 控制流 | if/for/switch 最佳实践，守卫子句和变量作用域 |
| `go-data-structures` | 数据结构 | slices、maps、arrays 的正确使用（nil vs 字面量、append 等） |
| `go-declarations` | 声明初始化 | var vs :=、结构体/常量/枚举声明规范 |
| `go-defensive` | 防御性编程 | API 边界防御、复制 slice/map、defer 清理、避免可变全局变量 |
| `go-error-handling` | 错误处理 | sentinel errors、fmt.Errorf(%w)、errors.Is/As，与项目 AppError 封装高度相关 |
| `go-functional-options` | 函数式选项 | 构造函数多可选参数场景，适用于配置初始化 |
| `go-functions` | 函数设计 | 函数签名、返回值设计、Printf 命名规范 |
| `go-generics` | 泛型 | 泛型函数/类型、约束选择，适用于通用工具函数 |
| `go-interfaces` | 接口设计 | 接口定义/实现、mock 边界、组合嵌入 |
| `go-linting` | Linting 配置 | golangci-lint 配置、CI/CD 集成 |
| `go-logging` | 日志（slog） | 结构化日志、slog 配置、请求级上下文日志（项目使用 slog） |
| `go-naming` | 命名规范 | 包/类型/函数/变量/常量命名 |
| `go-packages` | 包组织 | 包结构、import 管理、依赖管理 |
| `go-performance` | 性能优化 | 字符串拼接、基准测试、性能瓶颈排查 |
| `go-style-core` | 核心风格 | 格式化、缩进、裸返回、分号等兜底风格规则 |
| `go-testing` | 测试 | 表驱动测试、子测试、并行测试、cmp.Diff 断言 |
| `go-documentation` | 文档编写 | 包/类型/函数/方法注释规范（项目要求中文注释） |
| **数据库与架构** | | |
| `database-schema-designer` | 数据库模式设计 | SQL/NoSQL 表结构设计、规范化、索引策略、迁移（PostgreSQL RLS 场景） |
| `supabase-postgres-best-practices` | PostgreSQL 最佳实践 | 查询优化、Schema 设计、索引策略（项目使用 PostgreSQL） |
| `ddia-systems` | 数据系统设计 | 存储引擎、复制、分区、事务、一致性模型 |
| `clean-architecture` | 清洁架构 | 依赖倒置、分层架构、用例边界（项目 internal/domain/application/infrastructure 分层） |
| `domain-driven-design` | 领域驱动设计 | 限界上下文、聚合根、通用语言（多品牌业务域建模） |
| `system-design` | 分布式系统设计 | 负载均衡、缓存、消息队列、水平扩展（三端独立部署） |
| `release-it` | 生产稳定性 | 断路器、重试、超时、健康检查 |
| **代码质量与工具** | | |
| `clean-code` | 代码整洁 | 命名、小函数、SRP、注释纪律、错误处理 |
| `refactoring-patterns` | 重构模式 | 提取方法、替换条件、消除代码坏味道 |
| `software-design-philosophy` | 设计哲学 | 深模块、信息隐藏、战略编程 |
| `pragmatic-programmer` | 编程元原则 | DRY、正交性、探针、契约设计 |
| **测试与调试** | | |
| `tdd` | 测试驱动开发 | Red-Green-Refactor 循环、集成测试 |
| `diagnose` | 诊断调试 | 复现→最小化→假设→修复→回归测试 |
| `review` | 代码审查 | 安全性、正确性、性能审查 |
| **文档与规范** | | |
| `coding-guidelines` | 编码指南 | Rust 编码规范（不适用本项目） |
| `writing-plans` | 编写计划 | 多步任务规范、验收标准（适用于 PRD 和功能规划） |
| `prd` | PRD 管理 | 用户故事、验收标准、任务拆解 |