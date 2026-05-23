# API 接口文档

> **Base URL**
> - 品牌商家后台: `http://localhost:8081`
> - C 端用户: `http://localhost:8082`
> - 平台管理后台: `http://localhost:8083`

> **统一响应格式**
> ```json
> {
>   "code": "OK",           // 业务错误码，"OK" 表示成功
>   "message": "success",   // 提示消息
>   "data": { ... }         // 业务数据
> }
> ```

> **分页响应格式**
> ```json
> {
>   "code": "OK",
>   "message": "success",
>   "data": {
>     "items": [],
>     "total": 100,
>     "page": 1,
>     "page_size": 20,
>     "total_page": 5
>   }
> }
> ```

> **认证方式**
> - 所有受保护接口需要在 Header 中携带 `Authorization: Bearer <access_token>`
> - 未认证返回: `401 {"code": "UNAUTHORIZED", "message": "缺少认证令牌"}`

---

## 一、平台管理后台 (`/api/v1/admin`, 端口 8083)

### 1.1 管理员登录

```
POST /api/v1/admin/login
```

**请求体**
```json
{
  "username": "admin",
  "password": "admin123"
}
```

**响应**
```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "access_token": "eyJhbG...",
    "refresh_token": "eyJhbG...",
    "user": {
      "id": 1,
      "username": "admin",
      "role": "super_admin"
    }
  }
}
```

### 1.2 创建品牌

```
POST /api/v1/admin/brands
Authorization: Bearer <token>
```

**请求体**
```json
{
  "name": "XX健身",
  "logo_url": "https://example.com/logo.png",
  "contact_name": "张三",
  "contact_phone": "13800138000"
}
```

**响应**
```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "id": 1,
    "name": "XX健身",
    "logo_url": "https://example.com/logo.png",
    "contact_name": "张三",
    "contact_phone": "13800138000",
    "status": "pending",
    "created_at": "2026-05-20T10:00:00+08:00",
    "updated_at": "2026-05-20T10:00:00+08:00"
  }
}
```

### 1.3 品牌列表

```
GET /api/v1/admin/brands?page=1&page_size=20
Authorization: Bearer <token>
```

**查询参数**
| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| page | int | 否 | 1 | 页码 |
| page_size | int | 否 | 20 | 每页数量（最大 100） |

**响应** — 标准分页格式，items 为 Brand 对象数组

### 1.4 品牌详情

```
GET /api/v1/admin/brands/:id
Authorization: Bearer <token>
```

### 1.5 更新品牌

```
PUT /api/v1/admin/brands/:id
Authorization: Bearer <token>
```

**请求体**（所有字段可选，只传需要更新的）
```json
{
  "name": "新名称",
  "logo_url": "https://...",
  "contact_name": "李四"
}
```

### 1.6 更新品牌状态

```
PATCH /api/v1/admin/brands/:id/status
Authorization: Bearer <token>
```

**请求体**
```json
{
  "status": "active"   // active | inactive | pending
}
```

### 1.7 创建管理员

```
POST /api/v1/admin/admins
Authorization: Bearer <token>
```

**请求体**
```json
{
  "username": "operator01",
  "password": "secure123",
  "role": "operator"   // super_admin | operator | support
}
```

### 1.8 管理员列表

```
GET /api/v1/admin/admins?page=1&page_size=20
Authorization: Bearer <token>
```

---

## 二、品牌商家后台 (`/api/v1/brand`, 端口 8081)

### 2.1 品牌管理员登录

```
POST /api/v1/brand/login
```

**请求体**
```json
{
  "phone": "13800138000",
  "password": "your_password"
}
```

**响应**
```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "access_token": "eyJhbG...",
    "refresh_token": "eyJhbG...",
    "user": {
      "id": 1,
      "name": "张三",
      "phone": "13800138000",
      "brand_id": 1
    }
  }
}
```

### 2.2 创建子账号

```
POST /api/v1/brand/users
Authorization: Bearer <token>
```

**请求体**
```json
{
  "phone": "13900139000",
  "password": "secure123",
  "name": "员工A"
}
```

### 2.3 C 端用户列表

```
GET /api/v1/brand/users?page=1&page_size=20
Authorization: Bearer <token>
```

### 2.4 C 端用户详情

```
GET /api/v1/brand/users/:id
Authorization: Bearer <token>
```

### 2.5 创建课程

```
POST /api/v1/brand/courses
Authorization: Bearer <token>
```

**请求体**
```json
{
  "title": "新手入门·全身燃脂",
  "description": "适合初学者的全身训练",
  "cover_url": "https://...",
  "difficulty": "beginner",    // beginner | intermediate | advanced
  "duration_min": 30,
  "type": "cardio"             // strength | cardio | flexibility | hiit
}
```

### 2.6 课程列表

```
GET /api/v1/brand/courses?page=1&page_size=20
Authorization: Bearer <token>
```

### 2.7 课程详情

```
GET /api/v1/brand/courses/:id
Authorization: Bearer <token>
```

### 2.8 更新课程

```
PUT /api/v1/brand/courses/:id
Authorization: Bearer <token>
```

**请求体**（字段同创建，所有字段可选）

### 2.9 删除课程

```
DELETE /api/v1/brand/courses/:id
Authorization: Bearer <token>
```

### 2.10 更新课程状态

```
PATCH /api/v1/brand/courses/:id/status
Authorization: Bearer <token>
```

**请求体**
```json
{
  "status": "published"   // draft | published | archived
}
```

### 2.11 训练记录列表

```
GET /api/v1/brand/trainings?page=1&page_size=20
Authorization: Bearer <token>
```

---

## 三、C 端用户 (`/api/v1/app`, 端口 8082)

### 3.1 微信登录

```
POST /api/v1/app/auth/wechat-login
```

**请求体**
```json
{
  "brand_id": 1,
  "code": "wx_auth_code",
  "nickname": "健身达人"
}
```

**响应**
```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "access_token": "eyJhbG...",
    "refresh_token": "eyJhbG...",
    "user": {
      "id": 1,
      "brand_id": 1,
      "nickname": "健身达人",
      "avatar_url": "",
      "vip_level": "free"
    },
    "is_new_user": true
  }
}
```

### 3.2 获取个人资料

```
GET /api/v1/app/profile
Authorization: Bearer <token>
```

### 3.3 更新个人资料

```
PUT /api/v1/app/profile
Authorization: Bearer <token>
```

**请求体**
```json
{
  "nickname": "新昵称",
  "avatar_url": "https://..."
}
```

### 3.4 课程列表（仅已发布）

```
GET /api/v1/app/courses?page=1&page_size=20
Authorization: Bearer <token>
```

### 3.5 课程详情

```
GET /api/v1/app/courses/:id
Authorization: Bearer <token>
```

### 3.6 创建训练记录

```
POST /api/v1/app/trainings
Authorization: Bearer <token>
```

**请求体**
```json
{
  "course_id": 1,
  "duration_min": 35,
  "calories": 280.5,
  "notes": "感觉很好"
}
```

### 3.7 我的训练记录

```
GET /api/v1/app/trainings?page=1&page_size=20
Authorization: Bearer <token>
```

---

## 四、错误码参考

| 错误码 | HTTP 状态码 | 说明 |
|--------|------------|------|
| `OK` | 200 | 成功 |
| `INTERNAL_SERVER_ERROR` | 500 | 服务器内部错误 |
| `INVALID_REQUEST` | 400 | 请求参数错误 |
| `UNAUTHORIZED` | 401 | 未认证或令牌无效 |
| `FORBIDDEN` | 403 | 无权访问 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `AUTH_INVALID_TOKEN` | 401 | 令牌格式错误 |
| `AUTH_TOKEN_EXPIRED` | 401 | 令牌已过期 |
| `AUTH_INVALID_CREDENTIALS` | 401 | 账号或密码错误 |
| `AUTH_ACCOUNT_DISABLED` | 403 | 账号已被禁用 |
| `BRAND_NOT_FOUND` | 404 | 品牌不存在 |
| `BRAND_EXISTS` | 409 | 品牌已存在 |
| `USER_NOT_FOUND` | 404 | 用户不存在 |
| `USER_EXISTS` | 409 | 用户已存在 |
| `COURSE_NOT_FOUND` | 404 | 课程不存在 |
| `TRAINING_NOT_FOUND` | 404 | 训练记录不存在 |

---

## 五、快速测试

```bash
# 1. 管理端登录
curl -X POST http://localhost:8083/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 2. 用返回的 token 访问受保护接口
TOKEN="<your_access_token>"
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/v1/admin/brands

# 3. 创建品牌
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8083/api/v1/admin/brands \
  -d '{"name":"TestGym","contact_name":"Admin","contact_phone":"13900000001"}'
```
