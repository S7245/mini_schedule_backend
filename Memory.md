# Backend Project Decisions

## 基本信息
- **项目类型**: Golang SaaS 多品牌入驻 Web 后台
- **路径**: `/Users/liushan/Documents/zkw/mini_schedule/backend`
- **创建日期**: 2026-05-19

## 关键决策

| 决策点 | 选择 |
|--------|------|
| 业务模式 | SaaS 多品牌入驻（不做教练个人入驻） |
| 数据隔离 | 单库单 Schema + `brand_id` + PostgreSQL RLS |
| MVP 功能 | 品牌入驻 + 用户管理 + 课程/计划管理 + 基础数据记录 + 认证权限 |
| Web 框架 | Gin |
| ORM | GORM v2 |
| 数据库 | PostgreSQL |
| 缓存 | Redis |
| 配置 | Viper |
| 验证 | validator |
| 日志 | slog (Go 1.21+) |
| 依赖注入 | Google Wire |
| 数据库迁移 | golang-migrate |
| 部署架构 | 三端独立 Gin 实例（brand/app/admin），独立端口 |
| 用户表 | 三端独立表：`brand_users`、`app_users`、`admin_users` |
| 权限模型 | MVP 单品牌管理员，后期 RBAC |
| 可观测性 | slog + Prometheus 基础指标（MVP 阶段） |
| 异步任务 | Goroutine + Worker Pool（MVP 阶段） |
| API 响应 | `{code, message, data}` 格式 |
| 错误码 | 字符串错误码（如 `"AUTH_INVALID_TOKEN"`） |
| 错误处理 | 自定义 `AppError{Code, Message, HTTPStatus}` + 中间件统一转换 |

## 项目结构

```
backend/
├── cmd/
│   ├── api-brand/          # 品牌商家后台（独立进程）
│   ├── api-app/            # C 端用户（独立进程）
│   └── api-admin/          # 平台管理后台（独立进程）
├── internal/
│   ├── domain/             # 领域层（共享）
│   ├── application/        # 应用层（共享）
│   ├── infrastructure/     # 基础设施层（共享）
│   └── interfaces/
│       ├── brand/          # 品牌端 Handler + Router
│       ├── app/            # C 端 Handler + Router
│       ├── admin/          # 管理端 Handler + Router
│       └── middleware/     # 共享中间件
├── pkg/                    # 公共库（错误码、响应封装等）
├── configs/
│   └── config.yaml
├── migrations/             # golang-migrate SQL 文件
├── go.mod
└── go.sum
```

## 路由规划

| 端 | 路由前缀 | 鉴权 | 用户表 |
|---|---|---|---|
| 品牌商家后台 | `/api/v1/brand/*` | JWT（手机号+密码/短信） | `brand_users` |
| C 端 App | `/api/v1/app/*` | JWT（微信登录+手机号） | `app_users` |
| 平台管理后台 | `/api/v1/admin/*` | JWT（账号密码） | `admin_users` |

## 其他注意事项

- 代码要标清注释，包括函数、结构体、变量等
- 代码要符合 Go 语言规范，如命名规范、代码格式等
- 代码要符合项目业务逻辑，如数据隔离、权限模型等
- 代码注释语言使用中文
