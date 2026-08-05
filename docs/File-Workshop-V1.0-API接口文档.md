# File Workshop V1.0 API 接口文档

> 文档编号：FW-API-V1.0  
> 文档版本：V0.2
> 文档状态：按模块持续编制  
> 最近更新：2026-08-05  
> 当前已收录：公共健康检查、模块 01 身份认证、模块 02 用户管理
> 机器契约：`backend/api/openapi.yaml`

## 1. 文档定位

本文档是后端开发、前后端联调和接口测试共同使用的可读接口说明，按照设计文档的系统模块 01～16 持续追加。每完成一个模块，必须在同一开发周期补齐该模块章节；全部后端模块完成后只进行全量复核和版本冻结。

OpenAPI 是机器可读的唯一权威契约。本文档必须与 OpenAPI、生成代码和实际实现保持一致；如有冲突，应停止测试和联调，先修复契约或文档偏差。

## 2. 模块收录状态

| 顺序 | 模块 | 文档状态 | 接口数量 | 最近验证 |
|---:|---|---|---:|---|
| 公共 | 健康检查 | 已完成 | 2 | 2026-08-05 |
| 01 | 身份认证 | 已完成 | 4 | 2026-08-05 |
| 02 | 用户管理 | 已完成 | 13 | 2026-08-05 |
| 03～16 | 后续系统模块 | 未收录 | 0 | — |

## 3. 全局接口约定

### 3.1 地址和数据格式

- 本地开发地址：`http://127.0.0.1:8080`
- 业务 API 基础路径：`/api/v1`
- 健康检查是部署探针例外，使用 `/health/*`。
- 普通请求和响应使用 `application/json`。
- JSON 字段统一使用 `camelCase`。
- 时间使用 ISO 8601/RFC 3339，服务端内部统一 UTC。
- 浏览器请求允许来源由 `FILE_WORKSHOP_AUTH_ALLOWED_ORIGINS` 配置；本地默认允许 `http://127.0.0.1:5173` 和 `http://localhost:5173`。

### 3.2 Request ID

客户端可以通过 `X-Request-ID` 传入合法追踪标识；服务端始终在响应头返回最终使用的 `X-Request-ID`，错误响应正文同时返回 `requestId`。

### 3.3 统一错误结构

```json
{
  "code": "AUTH_REQUIRED",
  "message": "认证信息无效或已过期",
  "requestId": "019fd14d-c956-7f0e-a061-e5ee440d77b1",
  "details": {}
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `code` | string | 是 | 稳定、机器可读的错误码 |
| `message` | string | 是 | 可直接展示或转换的用户可读消息 |
| `requestId` | string | 是 | 请求追踪标识 |
| `details` | object | 否 | 结构化补充信息，不包含堆栈、SQL、路径或敏感配置 |

### 3.4 认证入口

| 名称 | 传输方式 | 用途 |
|---|---|---|
| Bearer Token | `Authorization: Bearer <accessToken>` | 外部 API 和非浏览器客户端访问受保护接口 |
| Access Cookie | `file_workshop_access` | 浏览器短期访问 JWT；`HttpOnly`、`SameSite`，生产环境必须 `Secure` |
| Refresh Cookie | `file_workshop_refresh` | 只允许刷新和注销；`HttpOnly`、路径 `/api/v1/auth`，生产环境必须 `Secure` |

认证响应使用 `Cache-Control: no-store`。浏览器不得把 Access Token 或 Refresh Token 持久化到 LocalStorage；Refresh Token 不进入 JSON 响应正文。

## 4. 公共接口：健康检查

### 4.1 存活检查

`GET /health/live`  
Operation ID：`getLiveness`  
认证：不需要

只确认 HTTP 进程能够响应，不访问 PostgreSQL、Redis 或 MinIO。

成功状态：`200 OK`

```json
{
  "status": "ok",
  "service": "file-workshop-server",
  "timestamp": "2026-08-05T09:00:00Z",
  "requestId": "019fd14d-c923-7404-b98c-bfb993aecb66",
  "checks": {}
}
```

### 4.2 就绪检查

`GET /health/ready`  
Operation ID：`getReadiness`  
认证：不需要

PostgreSQL 是必需依赖；Redis 是可降级依赖；MinIO 暂缓期间返回 `disabled`。

| HTTP 状态 | 条件 |
|---:|---|
| `200` | 节点可接收流量；正文 `status` 为 `ok` 或 `degraded` |
| `503` | PostgreSQL 等必需依赖不可用 |

`checks` 中每个组件包含：

| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | `ok`、`unavailable`、`disabled` | 组件状态 |
| `latencyMs` | int64 | 检查耗时毫秒数 |
| `message` | string，可选 | 不包含敏感信息的故障摘要 |

## 5. 模块 01：身份认证

### 5.1 模块边界和当前范围

本模块只建立用户身份和数据库会话上下文，不判断用户对文件、文件夹或空间的访问权限。

当前接口包含密码登录、当前会话、Refresh Token 轮换和退出。MFA 已明确暂缓，LDAP/OIDC/AD 等外部身份源待后续条件具备后接入，因此当前文档和路由中均不提供伪实现接口。

当前密码策略为：Argon2id 哈希、长度 12～128 字符、不得与用户名相同、拒绝常见弱密码；不实施密码历史限制。

### 5.2 登录

`POST /api/v1/auth/login`  
Operation ID：`login`  
认证：不需要  
Content-Type：`application/json`

请求正文：

| 字段 | 类型 | 必填 | 约束 | 说明 |
|---|---|---|---|---|
| `username` | string | 是 | 1～128 字符 | 用户名；服务端执行 NFKC 和 Unicode Case Folding 规范化 |
| `password` | string | 是 | 1～128 字符 | 登录密码；不得写入日志 |
| `deviceId` | string | 否 | 1～256 字符 | 客户端生成的设备标识，不作为认证因子 |

请求示例：

```http
POST /api/v1/auth/login HTTP/1.1
Host: 127.0.0.1:8080
Content-Type: application/json
Origin: http://127.0.0.1:5173

{
  "username": "root",
  "password": "<用户输入的密码>",
  "deviceId": "goland-api-test"
}
```

成功状态：`200 OK`

成功响应头：

- `X-Request-ID`
- `Cache-Control: no-store`
- 两个 `Set-Cookie`：Access Cookie 和 Refresh Cookie

成功响应正文：

```json
{
  "accessToken": "<short-lived-jwt>",
  "tokenType": "Bearer",
  "expiresIn": 900,
  "user": {
    "userId": "019fd14d-c900-7000-8000-000000000001",
    "username": "root",
    "displayName": "Root",
    "systemRole": "SYSTEM_ADMIN",
    "locale": "zh-CN",
    "timezone": "Asia/Shanghai"
  },
  "session": {
    "sessionId": "019fd14d-c900-7000-8000-000000000002",
    "status": "ACTIVE",
    "deviceId": "goland-api-test",
    "createdAt": "2026-08-05T09:00:00Z",
    "expiresAt": "2026-08-12T09:00:00Z"
  },
  "requestId": "019fd14d-c924-77b0-a9bc-61eafa37061e"
}
```

注意：示例 UUID 和 Token 仅表示格式。数据库登录用户 `root` 不等于业务用户；业务账号由用户管理模块创建。

错误响应：

| HTTP 状态 | 错误码 | 条件 |
|---:|---|---|
| `400` | `INVALID_REQUEST` | JSON、必填字段或长度不符合契约 |
| `401` | `AUTH_INVALID_CREDENTIALS` | 用户不存在、密码错误、账号不可用；不区分具体原因 |
| `403` | `AUTH_ORIGIN_REJECTED` | 浏览器 Origin 不在允许列表 |
| `423` | `AUTH_ACCOUNT_LOCKED` | 管理锁定或连续失败达到阈值；`details.retryAfterSeconds` 给出等待时间 |
| `429` | `AUTH_RATE_LIMITED` | 同一 IP 短时请求过多；响应头返回 `Retry-After` |

业务副作用：

- 在同一 PostgreSQL 事务创建 `user_sessions` 和第一代 `session_refresh_tokens`。
- 更新用户最后登录时间和凭据最后使用时间。
- 在 `login_attempts` 记录成功或失败事实。
- 数据库只保存 Refresh Token 的 SHA-256 摘要。

### 5.3 轮换 Refresh Token

`POST /api/v1/auth/refresh`  
Operation ID：`refreshSession`  
认证：必须携带 `file_workshop_refresh` Cookie  
请求正文：无

请求示例：

```http
POST /api/v1/auth/refresh HTTP/1.1
Host: 127.0.0.1:8080
Cookie: file_workshop_refresh=<refresh-token>
Origin: http://127.0.0.1:5173
```

成功状态：`200 OK`。响应正文结构与登录成功响应一致，并通过 `Set-Cookie` 返回下一代 Access Cookie 和 Refresh Cookie。

| HTTP 状态 | 错误码 | 条件 |
|---:|---|---|
| `401` | `AUTH_REQUIRED` | Cookie 缺失、Token 无效、过期、已撤销或发生重放 |
| `403` | `AUTH_ORIGIN_REJECTED` | Origin 不在允许列表 |

业务副作用：

- 单事务锁定旧 Refresh Token。
- 将旧 Token 从 `ACTIVE` 更新为 `USED`，插入同一 Token Family 的下一代 Token。
- 检测到已使用 Token 被重放时，将其标记为 `REUSED`，撤销整个 Session 和该 Session 的所有活动 Refresh Token。
- 并发使用同一个旧 Token 时，只允许一个请求完成轮换，另一个请求触发重放防护并撤销会话。

### 5.4 注销当前会话

`POST /api/v1/auth/logout`  
Operation ID：`logout`  
认证：Bearer Token、Access Cookie 或 Refresh Cookie 任一可用于定位会话  
请求正文：无

成功状态：`204 No Content`。重复注销、Token 已失效或没有可定位会话时仍返回成功。

成功响应会清除 Access Cookie 和 Refresh Cookie。

| HTTP 状态 | 错误码 | 条件 |
|---:|---|---|
| `403` | `AUTH_ORIGIN_REJECTED` | Origin 不在允许列表 |

业务副作用：将数据库 Session 更新为 `REVOKED`，并撤销该 Session 的全部活动 Refresh Token。

### 5.5 获取当前会话

`GET /api/v1/auth/session`  
Operation ID：`getCurrentSession`  
认证：Bearer Token 或 Access Cookie

Bearer 请求示例：

```http
GET /api/v1/auth/session HTTP/1.1
Host: 127.0.0.1:8080
Authorization: Bearer <access-token>
```

成功状态：`200 OK`

```json
{
  "user": {
    "userId": "019fd14d-c900-7000-8000-000000000001",
    "username": "root",
    "displayName": "Root",
    "systemRole": "SYSTEM_ADMIN",
    "locale": "zh-CN",
    "timezone": "Asia/Shanghai"
  },
  "session": {
    "sessionId": "019fd14d-c900-7000-8000-000000000002",
    "status": "ACTIVE",
    "deviceId": "goland-api-test",
    "createdAt": "2026-08-05T09:00:00Z",
    "expiresAt": "2026-08-12T09:00:00Z",
    "lastSeenAt": "2026-08-05T09:05:00Z"
  },
  "requestId": "019fd14d-c94f-74c7-a9fb-486d9ae4bbe3"
}
```

| HTTP 状态 | 错误码 | 条件 |
|---:|---|---|
| `401` | `AUTH_REQUIRED` | JWT 缺失、签名/Issuer/Audience/时间无效，用户不可用，或数据库 Session 已过期/撤销 |

该接口同时校验访问 JWT 和数据库 Session；仅有格式正确的 JWT 不代表会话仍有效。

### 5.6 认证接口测试依据

模块 01 正式接口测试至少覆盖：

1. 正确用户名和密码登录成功，响应不包含 Refresh Token 正文。
2. 不存在用户与错误密码均返回相同 `401/AUTH_INVALID_CREDENTIALS`。
3. 连续第 5 次失败返回 `423`，锁定窗口内拒绝登录，成功登录后旧失败窗口重置。
4. 非允许 Origin 的登录、刷新和注销返回 `403`；允许 Origin 的 CORS 预检返回 `204` 和凭据头。
5. Access Token 验证算法、Issuer、Audience、过期时间、用户状态和 Session 状态。
6. Refresh Token 成功轮换，旧 Token 不再可用。
7. 旧 Refresh Token 重放后整个 Session 和活动 Token Family 被撤销。
8. 两个并发刷新请求最多一个完成轮换，并最终触发重放撤销。
9. 注销后 Access Token、Access Cookie 和 Refresh Cookie 均不能继续建立有效会话。
10. 登录、刷新、会话和注销响应不泄露密码、Refresh Token 摘要、SQL、堆栈和本地路径。

现有自动化证据：

- `backend/internal/modules/identity/**/*_test.go`
- `backend/internal/platform/httpserver/router_test.go`
- `backend/tests/integration/identity_http_test.go`
- `backend/scripts/verify.ps1`
- `backend/scripts/verify-integration.ps1`

## 6. 模块 02：用户管理

### 6.1 模块边界和授权

用户管理接口分为本人入口和系统管理员入口：

- `/api/v1/users/me*` 只允许访问当前认证用户自己的资料和会话。
- `/api/v1/admin/users*` 只允许活动 `SYSTEM_ADMIN` 使用；普通用户统一返回 `403/AUTH_FORBIDDEN`。
- 所有接口均接受 Bearer Token 或 `file_workshop_access` Cookie，并同时校验数据库用户和 Session 状态。
- 带 `Origin` 的写请求必须来自允许列表，否则返回 `403/AUTH_ORIGIN_REJECTED`。
- 普通响应不返回规范化字段、密码哈希、Refresh Token 摘要或内部安全版本。

用户状态为 `ACTIVE`、`DISABLED`、`LOCKED`、`DELETED`。`DELETED` 是终态；禁用、锁定、逻辑删除和密码重置都会撤销活动会话。系统必须至少保留一个活动 `SYSTEM_ADMIN`。

### 6.2 用户响应结构

`User` 字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `userId` | UUID | 是 | 用户稳定 ID |
| `username` | string | 是 | 原始用户名，创建后不可修改且逻辑删除后不复用 |
| `employeeNo` | string | 否 | 工号 |
| `displayName` | string | 是 | 显示名称 |
| `email` | email | 否 | 邮箱 |
| `phone` | string | 否 | 电话 |
| `systemRole` | `USER`、`SYSTEM_ADMIN` | 是 | 系统角色 |
| `status` | `ACTIVE`、`DISABLED`、`LOCKED`、`DELETED` | 是 | 用户状态 |
| `locale` | string | 是 | 语言区域 |
| `timezone` | IANA 时区 | 是 | 用户时区 |
| `lastLoginAt` | date-time | 否 | 最近成功登录时间 |
| `createdAt`、`updatedAt` | date-time | 是 | 创建和更新时间 |
| `deletedAt` | date-time | 否 | 逻辑删除时间，仅 `DELETED` 时存在 |
| `rowVersion` | int64 | 是 | 乐观锁版本，所有修改请求必须提交当前值 |

单用户响应统一为：

```json
{
  "user": {
    "userId": "019fd195-8000-7000-8000-000000000001",
    "username": "worker001",
    "employeeNo": "FW-001",
    "displayName": "一车间操作员",
    "email": "worker001@example.com",
    "systemRole": "USER",
    "status": "ACTIVE",
    "locale": "zh-CN",
    "timezone": "Asia/Shanghai",
    "createdAt": "2026-08-05T10:00:00Z",
    "updatedAt": "2026-08-05T10:00:00Z",
    "rowVersion": 1
  },
  "requestId": "019fd195-8100-7000-8000-000000000001"
}
```

### 6.3 获取和修改本人资料

`GET /api/v1/users/me`
Operation ID：`getCurrentUser`
认证：Bearer Token 或 Access Cookie

成功返回 `200` 和 `UserResponse`；认证失败返回 `401/AUTH_REQUIRED`。

`PATCH /api/v1/users/me`
Operation ID：`updateCurrentUser`
认证：Bearer Token 或 Access Cookie

请求至少包含一个可修改字段，并必须包含 `rowVersion`：

```json
{
  "displayName": "一车间操作员",
  "email": "worker001@example.com",
  "phone": "13800000000",
  "locale": "zh-CN",
  "timezone": "Asia/Shanghai",
  "rowVersion": 3
}
```

空字符串用于清除 `email` 或 `phone`。本人接口不能修改用户名、工号、系统角色或状态。

| HTTP 状态 | 错误码 | 条件 |
|---:|---|---|
| `400` | `INVALID_REQUEST` | 字段为空、邮箱、时区、长度或请求体无效 |
| `401` | `AUTH_REQUIRED` | 认证或数据库 Session 无效 |
| `403` | `AUTH_ORIGIN_REJECTED` | Origin 不允许 |
| `409` | `USER_VERSION_CONFLICT` | `rowVersion` 已过期 |

成功返回 `200` 和更新后的 `UserResponse`，`rowVersion` 增加 1，并写入 `USER_UPDATED` Outbox。

### 6.4 本人会话管理

`GET /api/v1/users/me/sessions?page=1&pageSize=50`
Operation ID：`listCurrentUserSessions`
认证：Bearer Token 或 Access Cookie

响应包含 `items`、`page`、`pageSize`、`total`、`requestId`。每个会话包含 `sessionId`、`status`、`isCurrent`、设备/IP/客户端摘要、创建/过期/最近访问/撤销时间和 `rowVersion`。分页非法返回 `400/INVALID_REQUEST`。

`DELETE /api/v1/users/me/sessions/{sessionId}`
Operation ID：`revokeCurrentUserSession`
认证：Bearer Token 或 Access Cookie

只能撤销本人会话；成功或重复撤销返回 `204`。会话不属于本人或不存在时返回 `404/USER_NOT_FOUND`，认证失败返回 `401/AUTH_REQUIRED`，Origin 不允许返回 `403/AUTH_ORIGIN_REJECTED`。成功后在同一事务撤销 Session 和活动 Refresh Token，并写入 `AUTH_SESSION_REVOKED` Outbox；撤销当前会话会使当前 Access Token 立即失效。

### 6.5 管理员分页查询和创建用户

`GET /api/v1/admin/users?page=1&pageSize=50&status=ACTIVE&systemRole=USER`
Operation ID：`listUsers`
认证：仅 `SYSTEM_ADMIN`

- `page` 默认 1；`pageSize` 默认 50、最大 200。
- `status` 和 `systemRole` 可选，枚举值必须符合数据库设计。
- 按 `createdAt DESC, userId DESC` 稳定排序。
- 响应包含 `items`、`page`、`pageSize`、准确 `total` 和 `requestId`。

`POST /api/v1/admin/users`
Operation ID：`createUser`
认证：仅 `SYSTEM_ADMIN`
必须请求头：`Idempotency-Key`，1～128 字符

```json
{
  "username": "worker001",
  "password": "<初始密码>",
  "employeeNo": "FW-001",
  "displayName": "一车间操作员",
  "email": "worker001@example.com",
  "phone": "13800000000",
  "systemRole": "USER",
  "locale": "zh-CN",
  "timezone": "Asia/Shanghai"
}
```

`username`、`password`、`displayName` 必填；角色默认 `USER`，语言默认 `zh-CN`，时区默认 `Asia/Shanghai`。密码使用模块 01 的 Argon2id 和 12～128 字符策略，不实施密码历史限制。

成功返回 `201`、`Location: /api/v1/admin/users/{userId}` 和 `UserResponse`。用户、PASSWORD 凭据、主体安全版本、`USER_CREATED` Outbox 和幂等结果在同一事务写入。

| HTTP 状态 | 错误码 | 条件 |
|---:|---|---|
| `400` | `INVALID_REQUEST` | 字段、密码策略、邮箱、时区或 Idempotency-Key 无效 |
| `401` | `AUTH_REQUIRED` | 未认证 |
| `403` | `AUTH_FORBIDDEN`、`AUTH_ORIGIN_REJECTED` | 非管理员或 Origin 不允许 |
| `409` | `USER_CONFLICT` | 用户名、工号或邮箱重复 |
| `409` | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求体 |

同一管理员使用相同幂等键和相同请求体重试时，返回同一个用户，不创建重复凭据或 Outbox。

### 6.6 管理员读取和修改用户

`GET /api/v1/admin/users/{userId}`
Operation ID：`getUser`

成功返回 `200/UserResponse`；不存在返回 `404/USER_NOT_FOUND`；普通用户返回 `403/AUTH_FORBIDDEN`。

`PATCH /api/v1/admin/users/{userId}`
Operation ID：`updateUser`

请求必须包含当前 `rowVersion`，并至少包含 `displayName`、`email`、`phone`、`locale`、`timezone`、`employeeNo`、`systemRole` 之一。`email`、`phone`、`employeeNo` 使用空字符串清除；用户名和状态不能由该接口修改。

成功返回 `200/UserResponse`。角色变化同时递增 `global_authorization_version`，写入 `USER_ROLE_CHANGED` 和 `USER_UPDATED` Outbox。

除公共的 `400/401/403/404` 外，冲突错误包括：

- `409/USER_VERSION_CONFLICT`：并发版本过期；
- `409/USER_CONFLICT`：邮箱或工号唯一值冲突；
- `409/USER_STATE_CONFLICT`：用户已经逻辑删除；
- `409/USER_LAST_SYSTEM_ADMIN`：操作会移除最后一个活动系统管理员。

### 6.7 用户状态操作

以下接口仅允许 `SYSTEM_ADMIN`，请求体统一为：

```json
{
  "rowVersion": 4,
  "reason": "人员离岗"
}
```

| 接口 | Operation ID | 成功响应 | 事务副作用 |
|---|---|---|---|
| `POST /api/v1/admin/users/{userId}/disable` | `disableUser` | `200/UserResponse` | 状态改为 `DISABLED`，递增安全版本，撤销会话和 Refresh Token，写 `USER_DISABLED` |
| `POST /api/v1/admin/users/{userId}/enable` | `enableUser` | `200/UserResponse` | `DISABLED`/`LOCKED` 改为 `ACTIVE`，递增安全版本，写 `USER_ENABLED` |
| `POST /api/v1/admin/users/{userId}/lock` | `lockUser` | `200/UserResponse` | 状态改为 `LOCKED`，递增安全版本，撤销会话和 Refresh Token，写 `AUTH_ACCOUNT_LOCKED` |
| `DELETE /api/v1/admin/users/{userId}` | `deleteUser` | `204` | 状态改为 `DELETED` 并设置 `deletedAt`，撤销凭据、会话和 Token，不物理删除引用事实 |

用户已经处于目标状态且请求携带当前 `rowVersion` 时可无副作用成功；`DELETED` 不能重新启用。所有操作校验最后一个活动系统管理员保护，并可能返回 `USER_VERSION_CONFLICT`、`USER_STATE_CONFLICT` 或 `USER_LAST_SYSTEM_ADMIN`。

### 6.8 管理员重置密码

`PUT /api/v1/admin/users/{userId}/password`
Operation ID：`resetUserPassword`
认证：仅 `SYSTEM_ADMIN`

```json
{
  "password": "<新密码>",
  "rowVersion": 5
}
```

成功返回 `204`；密码使用 Argon2id 更新活动 PASSWORD 凭据，不读取或写入 `user_password_history`。用户 `rowVersion` 递增，所有活动会话与 Refresh Token 被撤销，并写入 `AUTH_PASSWORD_CHANGED` Outbox。

除公共错误外，可能返回 `409/USER_PASSWORD_CREDENTIAL_NOT_FOUND`、`USER_VERSION_CONFLICT` 或 `USER_STATE_CONFLICT`。

### 6.9 模块 02 接口测试依据

正式接口测试至少覆盖：

1. 本人只能读取和修改允许的资料字段，不能跨用户读取。
2. 普通用户访问全部 `/admin/users*` 接口均返回 `403/AUTH_FORBIDDEN`。
3. `page/pageSize` 默认值、非法值、最大值、空页、最后一页和稳定排序。
4. 创建用户同键同请求只产生一名用户；同键不同请求返回 `IDEMPOTENCY_CONFLICT`。
5. 用户名、工号和邮箱唯一冲突；用户名逻辑删除后仍不能复用。
6. 两个相同 `rowVersion` 的并发更新最多一个成功，另一个返回 `USER_VERSION_CONFLICT`。
7. `ACTIVE/DISABLED/LOCKED/DELETED` 合法和非法转换，以及 `DELETED` 终态。
8. 禁用、锁定、删除和密码重置后，旧 Access Token 与 Refresh Token 立即失效。
9. 至少保留一个活动 `SYSTEM_ADMIN`，并发角色/状态变更不能绕过保护。
10. 逻辑删除只更新用户、凭据和会话状态，不级联删除文件、版本、授权、共享或审计引用。
11. 本人会话分页、越权撤销、重复撤销和撤销当前会话。
12. 响应和日志不包含密码、哈希、Token 摘要、SQL 或内部路径；用户变更事务包含对应 Outbox。

现有自动化证据：

- `backend/internal/modules/users/domain/*_test.go`
- `backend/internal/modules/users/application/*_test.go`
- `backend/tests/integration/users_http_test.go`
- `backend/tests/migration/initial_schema_test.go`
- `backend/scripts/verify.ps1`
- `backend/scripts/verify-integration.ps1`

## 7. 文档维护与冻结规则

1. 新模块先更新 `backend/api/openapi.yaml`，再生成代码和实现。
2. 模块开发完成时，在本文档追加对应模块章节和接口测试要点，并更新第 2 章状态。
3. 接口发生变化时，同步修改 OpenAPI、实现、测试和本文档，不保留长期漂移的别名或旧结构。
4. 后端 16 个模块完成后，逐项核对路由、Operation ID、认证、字段、状态码、错误码和示例，形成 V1.0 冻结版本。
5. 正式接口测试以冻结版本为准；冻结前的模块测试以本文档当前模块章节和 OpenAPI 为准。
