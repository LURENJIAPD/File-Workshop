# File Workshop V1.0 API 接口文档

> 文档编号：FW-API-V1.0  
> 文档版本：V0.12
> 文档状态：按模块持续编制  
> 最近更新：2026-08-10
> 当前已收录：公共健康检查、模块 01 身份认证、模块 02 用户管理、模块 03 组织与空间、模块 04 权限与管理委派、模块 05 文件目录、模块 06 文件传输与存储控制面、模块 07 版本与并发基础接口、模块 08 共享基础接口、模块 09 回收与生命周期元数据闭环、模块 10 PostgreSQL 元数据搜索基础接口、模块 16 后台任务基础调度与运维接口
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
| 03 | 组织与空间 | 已完成当前模块边界 | 23 | 2026-08-05 |
| 04 | 权限与管理委派 | 已完成当前模块边界 | 14 | 2026-08-05 |
| 05 | 文件目录 | 已完成当前模块边界 | 6 | 2026-08-10 |
| 06 | 文件传输与存储 | 已收录上传控制面当前边界 | 4 | 2026-08-10 |
| 07 | 版本与并发 | 已收录版本与锁基础接口 | 7 | 2026-08-10 |
| 08 | 共享 | 已收录用户/组织/LINK 基础接口 | 7 | 2026-08-10 |
| 09 | 回收与生命周期 | 已收录元数据回收、恢复和清理发起接口 | 4 | 2026-08-10 |
| 10 | 搜索 | 已收录 PostgreSQL 元数据搜索基础接口 | 1 | 2026-08-10 |
| 11～15 | 后续系统模块 | 未收录 | 0 | — |
| 16 | 后台任务 | 已完成基础调度与管理员运维接口 | 4 | 2026-08-10 |

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

只确认 HTTP 进程能够响应，不访问 PostgreSQL、Redis 或对象存储。

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

PostgreSQL 是必需依赖；Redis 是可降级依赖；对象存储暂缓期间返回 `disabled`。当前组件键名固定为 `postgresql`、`redis`、`objectStorage`，不得使用具体产品名作为对外键名。

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

## 7. 模块 03：组织与空间

### 7.1 模块边界和通用规则

本模块维护组织树、组织成员关系、个人/组织/公共空间、容量配额和组织变更计划。除“本人组织”和“本人个人空间”外，管理接口仅允许 `SYSTEM_ADMIN`。本模块不计算文件访问权限；权限、委派和资源最终授权由模块 04 负责。

所有列表使用 `page/pageSize`，默认 `1/50`，`pageSize` 最大 200，并返回 `items/page/pageSize/total/requestId`。所有修改和状态转换使用当前 `rowVersion`；版本过期返回 `409/ROW_VERSION_CONFLICT`。创建组织、添加成员、建立个人空间、创建公共空间、创建变更计划和添加计划操作要求 `Idempotency-Key`。

组织状态为 `ACTIVE/DISABLED/ARCHIVED/DELETED`；成员类型为 `PRIMARY/MEMBER`，成员状态为 `ACTIVE/INACTIVE`；空间类型为 `PERSONAL/ORGANIZATION/PUBLIC`，空间状态为 `ACTIVE/FROZEN/ARCHIVED/DELETED`。同一用户最多存在一个活动 `PRIMARY` 关系，组织关系还受 `effectiveFrom/effectiveUntil` 有效期约束。

### 7.2 本人组织和个人空间

| 接口 | Operation ID | 成功响应 | 说明 |
|---|---|---:|---|
| `GET /api/v1/users/me/organizations?page=1&pageSize=50` | `listCurrentUserOrganizations` | `200/OrganizationMembershipListResponse` | 只返回当前时间有效且状态为 `ACTIVE` 的本人关系 |
| `GET /api/v1/users/me/personal-space` | `getCurrentUserPersonalSpace` | `200/SpaceResponse` | 返回本人唯一个人空间；尚未建立时返回 `404/SPACE_NOT_FOUND` |

数据库设计没有定义个人空间默认配额，当前不自行编造默认值。个人空间通过管理员受控接口显式提供名称、配额和配置；后续由模块 16 接入 `USER_CREATED` 消费和可配置默认策略。

### 7.3 组织管理

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/admin/organizations?page=1&pageSize=50` | `listOrganizations` | `200/OrganizationListResponse` | 可按 `parentOrganizationId/status` 筛选，按 `sortOrder/normalizedName/organizationId` 稳定排序 |
| `POST /api/v1/admin/organizations` | `createOrganization` | `201/OrganizationResponse` | 同事务创建组织、闭包关系、安全版本和唯一组织空间 |
| `GET /api/v1/admin/organizations/{organizationId}` | `getOrganization` | `200/OrganizationResponse` | 不存在返回 `404/ORGANIZATION_NOT_FOUND` |
| `PATCH /api/v1/admin/organizations/{organizationId}` | `updateOrganization` | `200/OrganizationResponse` | 可修改 `name/code/typeLabel/sortOrder`，必须携带 `rowVersion` |
| `POST /api/v1/admin/organizations/{organizationId}/move` | `moveOrganization` | `200/OrganizationResponse` | `newParentOrganizationId` 省略表示移至根；事务内重建闭包并拒绝循环 |
| `PUT /api/v1/admin/organizations/{organizationId}/status` | `changeOrganizationStatus` | `200/OrganizationResponse` | 请求包含 `status/rowVersion/reason`；删除前检查子组织、成员、空间、委派、迁移和保留事实 |

创建组织请求示例：

```json
{
  "parentOrganizationId": "019fd200-0000-7000-8000-000000000001",
  "name": "装配一车间",
  "code": "ASSEMBLY-01",
  "typeLabel": "车间",
  "sortOrder": 10,
  "spaceQuotaBytes": 107374182400
}
```

组织响应字段严格对应数据库设计中的组织事实：`organizationId/parentOrganizationId/name/normalizedName/code/normalizedCode/typeLabel/sortOrder/pathCache/depth/treeVersion/status/createdByUserId/createdAt/updatedAt/deletedAt/rowVersion`。组织空间配额由创建请求的 `spaceQuotaBytes` 写入 `spaces.quota_bytes`，不是组织表新增字段。

### 7.4 组织成员关系

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/admin/organizations/{organizationId}/members?page=1&pageSize=50` | `listOrganizationMembers` | `200/OrganizationMembershipListResponse` | 可按 `status` 筛选 |
| `POST /api/v1/admin/organizations/{organizationId}/members` | `addOrganizationMember` | `201/OrganizationMembershipResponse` | 用户和组织必须活动；要求 `Idempotency-Key`；活动主职全局唯一 |
| `PATCH /api/v1/admin/organizations/{organizationId}/members/{membershipId}` | `updateOrganizationMember` | `200/OrganizationMembershipResponse` | 可修改类型、职务、状态和结束时间，使用乐观锁 |
| `DELETE /api/v1/admin/organizations/{organizationId}/members/{membershipId}` | `removeOrganizationMember` | `204` | 逻辑停用关系并设置有效期，不物理删除历史事实 |

成员响应包含 `membershipId/userId/organizationId/membershipType/jobTitle/status/effectiveFrom/effectiveUntil/createdByUserId/createdAt/updatedAt/rowVersion`。成员变更同步递增组织和用户的成员安全版本，并产生组织成员 Outbox 事件。

### 7.5 空间管理和配额

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `POST /api/v1/admin/users/{userId}/personal-space` | `provisionUserPersonalSpace` | `201/SpaceResponse` | 活动用户最多一个个人空间；请求显式提供 `name/quotaBytes/config` |
| `GET /api/v1/admin/spaces?page=1&pageSize=50` | `listSpaces` | `200/SpaceListResponse` | 可按 `spaceType/status/organizationId/ownerUserId` 筛选 |
| `POST /api/v1/admin/spaces` | `createPublicSpace` | `201/SpaceResponse` | 创建命名公共空间，要求 `Idempotency-Key` |
| `GET /api/v1/admin/spaces/{spaceId}` | `getSpace` | `200/SpaceResponse` | 不存在返回 `404/SPACE_NOT_FOUND` |
| `PATCH /api/v1/admin/spaces/{spaceId}` | `updateSpace` | `200/SpaceResponse` | 可修改名称、配额和版本化 JSON 配置；配额不得低于已用量与预留量之和 |
| `PUT /api/v1/admin/spaces/{spaceId}/status` | `changeSpaceStatus` | `200/SpaceResponse` | 请求包含 `status/rowVersion/reason`；个人空间禁止直接删除 |

空间响应包含 `spaceId/spaceType/name/normalizedName/ownerUserId/organizationId/rootFolderId/quotaBytes/usedBytes/reservedBytes/aclVersion/securityEpoch/configSchemaVersion/config/status/createdByUserId/createdAt/updatedAt/deletedAt/rowVersion`。配额预留、消费和释放是模块内部应用服务能力，不开放可绕过上传流程的公共 REST API；数据库条件更新保证 `usedBytes + reservedBytes <= quotaBytes`，并发预留不会超卖。

### 7.6 组织变更计划

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/admin/organization-change-plans?page=1&pageSize=50` | `listOrganizationChangePlans` | `200/OrganizationChangePlanListResponse` | 可按计划状态筛选 |
| `POST /api/v1/admin/organization-change-plans` | `createOrganizationChangePlan` | `201/OrganizationChangePlanResponse` | 创建 `DRAFT` 计划，要求当前 `expectedTreeVersion` 和 `Idempotency-Key` |
| `GET /api/v1/admin/organization-change-plans/{planId}` | `getOrganizationChangePlan` | `200/OrganizationChangePlanResponse` | 同时返回按 `sequenceNumber` 排序的操作 |
| `POST /api/v1/admin/organization-change-plans/{planId}/operations` | `addOrganizationChangeOperation` | `201/OrganizationChangePlanResponse` | 仅 `DRAFT` 可添加；要求 `Idempotency-Key`，同键重放不重复插入 |
| `POST /api/v1/admin/organization-change-plans/{planId}/transition` | `transitionOrganizationChangePlan` | `200/OrganizationChangePlanResponse` | `VALIDATE/APPROVE/EXECUTE/CANCEL`，必须携带计划 `rowVersion` |

计划类型为 `MOVE/MERGE/SPLIT/BULK_RESTRUCTURE`，状态为 `DRAFT/VALIDATED/APPROVED/EXECUTING/COMPLETED/CANCELLED/FAILED`。当前可同步执行不涉及文件内容的 `MOVE_NODE`；`MERGE_NODE/CREATE_NODE/MOVE_MEMBER/MOVE_SPACE_CONTENT` 的数据结构和校验入口已经建立，涉及文件事实的执行由模块 05 与模块 16 接入，当前执行请求返回 `409/ORGANIZATION_PLAN_OPERATION_DEFERRED`，不会伪装完成。

### 7.7 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN/AUTH_ORIGIN_REJECTED` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `ORGANIZATION_NOT_FOUND`、`ORGANIZATION_MEMBERSHIP_NOT_FOUND`、`SPACE_NOT_FOUND`、`ORGANIZATION_CHANGE_PLAN_NOT_FOUND` | 指定事实不存在 |
| 409 | `ROW_VERSION_CONFLICT` | `rowVersion` 已过期 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求体 |
| 409 | `ORGANIZATION_CONFLICT` | 唯一约束、引用约束或其他组织/空间事实冲突 |
| 409 | `ORGANIZATION_TREE_CYCLE` | 移动会形成组织循环 |
| 409 | `RESOURCE_DELETE_BLOCKED` | 仍有子组织、成员、空间或受保护事实 |
| 409 | `SPACE_QUOTA_EXCEEDED` | 配额不足或目标配额低于已占用量 |
| 409 | `RESOURCE_STATE_CONFLICT` | 当前状态不允许操作 |
| 409 | `ORGANIZATION_PLAN_OPERATION_DEFERRED` | 操作依赖尚未完成的文件或 Worker 执行器 |

正式接口测试至少覆盖：分页边界和稳定排序；普通用户访问管理接口被拒绝；创建幂等重放与冲突；组织闭包完整性、并发移动、防环和乐观锁；活动主职唯一性、成员有效期和逻辑停用；个人/组织/公共空间唯一边界；配额并发预留不超卖；空间状态和删除阻断；变更计划校验、审批、执行、取消与延期操作；事务 Outbox 和安全版本同步更新。

现有自动化证据：

- `backend/internal/modules/organizations/domain/validation_test.go`
- `backend/tests/integration/organizations_http_test.go`
- `backend/tests/migration/initial_schema_test.go`
- `backend/scripts/verify.ps1`
- `backend/scripts/verify-integration.ps1`

## 8. 模块 04：权限与管理委派

### 8.1 模块边界和判定顺序

模块 04 是全系统唯一资源授权判定入口。系统角色、管理委派和普通文件 ACL 分开建模；组织成员关系不自动产生文件权限。V1.0 只支持显式 `ALLOW`，无匹配授权时默认拒绝，不提供 `DENY` 接口。

最终判定顺序为：个人空间所有者 → `SYSTEM_ADMIN` → 组织管理委派 → 用户及其当前有效组织主体的直接/继承 ACL → 默认拒绝。个人空间所有者拥有该空间完整文件操作权限；组织管理委派只作用于组织空间，不允许据此访问员工个人空间。`SYSTEM_ADMIN` 对个人空间或预览、下载、恢复、永久清理等敏感动作必须同时提供 `privilegedReason` 并设置 `privilegedAccessConfirmed=true`，响应同时标记 `privilegedAccessRequired`。

资源类型为 `SPACE/FOLDER/DOCUMENT`，动作集合严格使用 `LIST/READ_METADATA/PREVIEW/DOWNLOAD/UPLOAD/CREATE_FOLDER/WRITE_CONTENT/RENAME/MOVE/DELETE/RESTORE/PURGE/SHARE/LOCK/MANAGE_VERSION/MANAGE_PERMISSION`。Document Version 不单独建立写权限，后续版本接口复用 Document 判定。

### 8.2 管理委派接口

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/admin-delegations?page=1&pageSize=50` | `listAdminDelegations` | `200/AdminDelegationListResponse` | 普通用户只看本人收到或创建的委派；系统管理员可查看全部；可按 `organizationId/status` 筛选 |
| `POST /api/v1/admin-delegations` | `createAdminDelegation` | `201/AdminDelegationResponse` | 根委派只允许 `SYSTEM_ADMIN`；继续委派必须引用本人当前有效、允许继续委派且含 `DELEGATE_ADMIN` 的父委派；要求 `Idempotency-Key` |
| `GET /api/v1/admin-delegations/{delegationId}` | `getAdminDelegation` | `200/AdminDelegationResponse` | 非系统管理员只能读取本人收到或创建的委派，不可见时返回 404 |
| `POST /api/v1/admin-delegations/{delegationId}/revoke` | `revokeAdminDelegation` | `200/AdminDelegationResponse` | 请求包含 `reason/rowVersion`；撤销父委派时后代立即置为 `INVALIDATED` 并递增相关用户和组织安全版本 |
| `GET /api/v1/organizations/{organizationId}/administrators?page=1&pageSize=50` | `listOrganizationAdministrators` | `200/AdminDelegationListResponse` | 返回直接或从祖先组织继承且整条父链均有效的管理员；普通用户须具有 `DELEGATE_ADMIN` |
| `POST /api/v1/admin-delegations/evaluate` | `evaluateAdminDelegation` | `200/AdminDelegationEvaluationResponse` | 判断当前用户对组织的单项管理能力；未授权返回 `allowed=false/source=NONE` |

管理范围为 `SELF/SUBTREE`，能力为 `MANAGE_SPACE_CONTENT/MANAGE_SPACE_PERMISSION/MANAGE_SPACE_MEMBERS/MANAGE_SPACE_RECYCLE_BIN/FORCE_UNLOCK/VIEW_SPACE_AUDIT/DELEGATE_ADMIN`。子委派的组织范围、有效期、能力集合和继续委派能力不得超过父委派；数据库延迟约束与应用服务同时校验。委派不能授予 `SYSTEM_ADMIN`，也不能向上或横向扩权。

### 8.3 普通 ACL 接口

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/permissions/resources/{resourceType}/{resourceId}?page=1&pageSize=50` | `listResourcePermissionGrants` | `200/PermissionGrantListResponse` | 只返回目标资源上的直接 ACL；调用者必须具有 `MANAGE_PERMISSION` |
| `POST /api/v1/permissions/grants` | `createPermissionGrant` | `201/PermissionGrantResponse` | 主体只能是 `USER/ORGANIZATION`；资源只能是 `SPACE/FOLDER/DOCUMENT`；要求至少一个动作和 `Idempotency-Key` |
| `PATCH /api/v1/permissions/grants/{grantId}` | `updatePermissionGrant` | `200/PermissionGrantResponse` | 可修改动作、继承、结束时间和原因，必须携带当前 `rowVersion`；Document 授权禁止向后代继承 |
| `POST /api/v1/permissions/grants/{grantId}/revoke` | `revokePermissionGrant` | `200/PermissionGrantResponse` | 请求包含 `reason/rowVersion`，逻辑撤销并记录撤销人和时间 |
| `POST /api/v1/permissions/evaluate` | `evaluatePermission` | `200/PermissionEvaluationResponse` | 判断当前登录用户的一项资源动作；不存在或无权统一返回 `allowed=false`，不泄露资源细节 |
| `POST /api/v1/permissions/batch-evaluate` | `batchEvaluatePermissions` | `200/BatchPermissionEvaluationResponse` | 一次 1～100 项，保持请求顺序；用于列表最终授权过滤，不使用无界请求 |
| `POST /api/v1/permissions/resources/{resourceType}/{resourceId}/break-inheritance` | `breakPermissionInheritance` | `200/PermissionInheritanceResponse` | 仅支持 Folder/Document；要求 `MANAGE_PERMISSION` 和当前 `rowVersion`；设置为 `BREAK` |
| `POST /api/v1/permissions/resources/{resourceType}/{resourceId}/restore-inheritance` | `restorePermissionInheritance` | `200/PermissionInheritanceResponse` | 仅支持 Folder/Document；恢复为 `INHERIT` |

Space 授权只有在 `inheritToDescendants=true` 时才能作用于后代；Folder 授权同理。目标资源或中间 Folder 的 `BREAK` 会阻断更高祖先 ACL，但不会阻断目标资源直接 ACL、个人空间所有者、系统管理员或有效管理委派。直接和继承授权取并集。

### 8.4 响应、缓存和事务副作用

权限判定结果包含 `resourceType/resourceId/action/allowed/source/matchedGrantIds/privilegedAccessRequired`。`source` 取值为 `PERSONAL_OWNER/SYSTEM_ADMIN/ADMIN_DELEGATION/DIRECT_GRANT/INHERITED_GRANT/NONE`。

委派和 ACL 写入在同一 PostgreSQL 事务中完成业务事实、资源 ACL/空间安全纪元、用户或组织安全版本、幂等记录和 Outbox。Redis 只保存 30 秒版本化判定缓存；Key 包含主体成员、委派、直接授权、共享、全局授权版本以及空间 ACL/安全纪元和相关组织安全版本。任何权限变更都会进入新版本 Key；Redis 读取、写入或连接失败时直接回源 PostgreSQL，不把缓存作为授权事实源，也不因缓存故障放行。

### 8.5 错误码和接口测试依据

除公共错误外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `RESOURCE_NOT_FOUND` | 委派、授权或资源不存在，或对调用者不可见 |
| 409 | `ROW_VERSION_CONFLICT` | 并发更新使用了过期版本 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求 |
| 409 | `ADMIN_DELEGATION_SCOPE_EXCEEDED` | 子委派范围、能力、有效期或父链不合法 |
| 409 | `AUTHORIZATION_CONFLICT` | 委派或 ACL 当前状态不允许操作 |

正式接口测试至少覆盖：默认拒绝；个人空间所有者；系统管理员原因和二次确认；普通用户越权；根委派、`SELF/SUBTREE`、能力子集、有效期、递归委派和父撤销立即失效；用户/组织主体授权；直接与继承并集；目标及中间节点断开继承；授权更新和撤销；分页边界；乐观锁；幂等冲突；Redis 缓存命中、版本变化立即失效和 Redis 故障回源。

现有自动化证据：

- `backend/internal/modules/permissions/domain/validation_test.go`
- `backend/internal/modules/permissions/application/evaluation_test.go`
- `backend/tests/integration/permissions_http_test.go`
- `backend/tests/migration/initial_schema_test.go`
- `backend/scripts/verify.ps1`
- `backend/scripts/verify-integration.ps1`

## 9. 模块 05：文件目录

### 9.1 模块边界和权限入口

本模块建立 `namespace_entries`、`folders`、`documents` 对应的目录事实层，负责统一命名空间、Folder/Document 稳定身份、根目录懒创建、目录列表、详情、重命名和移动。当前不上传二进制、不生成对象 Key、不创建版本内容、不扣减容量，也不执行扫描、预览或索引；这些由模块 06/07/10/12/16 接入。

所有目录读写入口都必须调用模块 04 最终授权服务，不在 Handler、Repository 或前端重复解释 ACL。根目录尚未创建时，空间根列表和首次创建会以 Space 为授权资源；根目录创建后，目录读写以 Folder/Document 为授权资源。移动目录项会递增 `spaces.security_epoch`，使继承路径变化后的权限缓存自然失效。

### 9.2 目录接口

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/spaces/{spaceId}/entries?page=1&pageSize=50` | `listDirectoryEntries` | `200/DirectoryEntryListResponse` | 列出根目录或指定 `parentFolderId` 子项；支持 `entryType`、`lifecycleStatus`；稳定排序为 `entryType/normalizedName/entryId`；无根目录时返回空列表 |
| `POST /api/v1/spaces/{spaceId}/folders` | `createFolder` | `201/DirectoryEntryResponse` | 要求 `Idempotency-Key`；请求体为 `name/parentFolderId`；首次在空间下创建时懒创建根文件夹 |
| `POST /api/v1/spaces/{spaceId}/documents` | `createDocument` | `201/DirectoryEntryResponse` | 要求 `Idempotency-Key`；请求体为 `name/parentFolderId/classification/metadata`；仅创建 Document 占位，`availabilityStatus=BLOCKED`，`currentVersionId=null` |
| `GET /api/v1/entries/{entryId}` | `getDirectoryEntry` | `200/DirectoryEntryResponse` | 读取 Folder 或 Document 元数据；调用者必须具备 `READ_METADATA` |
| `PATCH /api/v1/entries/{entryId}` | `renameDirectoryEntry` | `200/DirectoryEntryResponse` | 请求体为 `name/rowVersion`；根文件夹不可重命名；文件夹重命名会刷新后代路径缓存 |
| `POST /api/v1/entries/{entryId}/move` | `moveDirectoryEntry` | `200/DirectoryEntryResponse` | 请求体为 `targetParentFolderId/rowVersion`；根文件夹不可移动；文件夹不可移动到自身或后代；移动成功后递增空间安全纪元 |

### 9.3 请求和响应字段

分页请求统一使用 `page`、`pageSize`，默认值为 `1/50`，最大 `pageSize=200`。列表响应至少包含 `items/page/pageSize/total/requestId`，并在适用时返回 `spaceId/rootFolderId/parentFolderId`。

`DirectoryEntry` 字段严格映射数据库事实和 OpenAPI：

- 公共字段：`entryId/spaceId/parentFolderId/entryType/name/normalizedName/pathCache/depth/lifecycleStatus/isRoot/createdByUserId/createdAt/updatedAt/deletedAt/rowVersion`；
- Folder 字段：`inheritanceMode/aclVersion`；
- Document 字段：`ownerUserId/currentVersionId/availabilityStatus/extensionNormalized/inheritanceMode/aclVersion/classification/metadataSchemaVersion/metadata`。

目录名称允许中文和普通业务字符，但禁止空名、`.`、`..`、斜杠、反斜杠和控制字符。同一父目录下 `ACTIVE/ARCHIVED` 目录项名称归一化后唯一；根目录由系统创建，不作为普通用户可移动或重命名对象。

### 9.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN/AUTH_ORIGIN_REJECTED` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `DIRECTORY_ENTRY_NOT_FOUND` | 目录项、父目录或空间不存在、已删除或对调用者不可见 |
| 409 | `DIRECTORY_CONFLICT` | 名称唯一约束、引用约束或其他目录事实冲突 |
| 409 | `ROW_VERSION_CONFLICT` | `rowVersion` 已过期 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求体 |
| 409 | `DIRECTORY_TREE_CYCLE` | 文件夹移动会形成循环 |
| 409 | `DIRECTORY_ROOT_OPERATION_FORBIDDEN` | 对根文件夹执行移动或重命名 |

正式接口测试至少覆盖：未登录拒绝；普通用户越权；系统管理员通过统一权限入口操作公共空间；根目录懒创建；中文目录名；同目录重名冲突；`page/pageSize` 默认值、非法值和最大值；Document 占位状态；详情读取；乐观锁重命名；文件夹重命名后代路径刷新；移动到新父目录；文件夹防环；幂等重放和冲突；Outbox 与幂等记录同事务写入。

现有自动化证据：

- `backend/internal/modules/files/domain/validation_test.go`
- `backend/internal/modules/files/application/service_test.go`
- `backend/tests/integration/files_http_test.go`
- `backend/scripts/verify.ps1`

## 10. 模块 06：文件传输与存储

### 10.1 当前边界

本周期先完成上传控制面，不伪造完整上传闭环。对象存储仍通过项目内 Object Storage Interface 访问 S3 Compatible API，默认实现面向 SeaweedFS S3 Gateway；业务代码不依赖 SeaweedFS 私有路径、Volume/Filer 元数据或管理 API。

当前能力：

- 创建上传会话时先创建 S3 Multipart Upload，再在 PostgreSQL 事务内预留空间配额、写入 `quota_reservations/upload_sessions`、完成幂等记录和 Outbox 事件；
- 对象存储未启用或不可用时返回稳定错误，不创建数据库上传会话；
- 对象 Key 由系统生成，不使用用户文件名，也不在上传会话响应中暴露；
- 支持获取上传会话详情；
- 支持获取指定分片的短有效期 PUT 预签名 URL；
- 支持按 `rowVersion` 取消上传会话，并释放配额预留；
- `providerUploadId/temporaryObjectKey` 仅用于服务端内部和对象存储适配层，不进入 REST 响应。

尚未完成：完成上传提交、`upload_parts` 分片登记、最终 Hash 校验、`storage_objects/document_versions` 写入、下载、Range、真实 SeaweedFS 兼容性测试和孤儿对象清理。这些继续属于模块 06 后续周期。

### 10.2 接口清单

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `POST /api/v1/uploads` | `createUploadSession` | `201/UploadSessionResponse` | 要求 `Idempotency-Key`；创建 S3 Multipart、预留配额并写入上传会话；对象存储未启用时返回 `409/STORAGE_UNAVAILABLE` |
| `GET /api/v1/uploads/{uploadSessionId}` | `getUploadSession` | `200/UploadSessionResponse` | 仅会话创建者或 `SYSTEM_ADMIN` 可读；不返回对象 Key 和存储上传 ID |
| `POST /api/v1/uploads/{uploadSessionId}/parts/{partNumber}/presign` | `presignUploadPart` | `200/PresignedUploadPartResponse` | `partNumber` 范围 `1..expectedPartCount`；会话状态必须为 `INITIATED/UPLOADING` 且未过期 |
| `POST /api/v1/uploads/{uploadSessionId}/abort` | `abortUploadSession` | `200/UploadSessionResponse` | 请求体必须包含 `reason/rowVersion`；仅 `INITIATED/UPLOADING/FAILED` 可取消；取消后释放配额 |

### 10.3 创建上传会话

请求示例：

```http
POST /api/v1/uploads
Idempotency-Key: upload-20260810-0001
Content-Type: application/json
```

```json
{
  "spaceId": "0198a8e8-cb60-7c70-a4f6-3f011c89a021",
  "folderId": "0198a8e8-d0b1-7c80-b83d-27d790e69a61",
  "uploadIntent": "CREATE",
  "fileName": "工艺图纸.pdf",
  "declaredSizeBytes": 9437184,
  "partSizeBytes": 5242880,
  "declaredMimeType": "application/pdf"
}
```

响应示例：

```json
{
  "session": {
    "uploadSessionId": "0198a8e9-0c8a-7d21-9ea9-e9e70d477eed",
    "userId": "0198a8e7-9b6a-7f7f-b120-1d111ddca001",
    "spaceId": "0198a8e8-cb60-7c70-a4f6-3f011c89a021",
    "folderId": "0198a8e8-d0b1-7c80-b83d-27d790e69a61",
    "quotaReservationId": "0198a8e9-0c8b-7465-95a3-fb83ef6ce101",
    "uploadIntent": "CREATE",
    "fileName": "工艺图纸.pdf",
    "normalizedName": "工艺图纸.pdf",
    "declaredSizeBytes": 9437184,
    "declaredMimeType": "application/pdf",
    "partSizeBytes": 5242880,
    "expectedPartCount": 2,
    "status": "INITIATED",
    "expiresAt": "2026-08-11T02:00:00Z",
    "createdAt": "2026-08-10T02:00:00Z",
    "updatedAt": "2026-08-10T02:00:00Z",
    "rowVersion": 1
  },
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

`NEW_VERSION` 上传必须传入 `targetDocumentId`，可选传入 `expectedCurrentVersionId/expectedLockFencingToken/lockTokenHashHex` 作为后续完成提交时的并发条件。当前周期只保存这些条件，不执行完成提交。

### 10.4 分片预签名

请求：

```http
POST /api/v1/uploads/0198a8e9-0c8a-7d21-9ea9-e9e70d477eed/parts/1/presign
```

响应：

```json
{
  "part": {
    "uploadSessionId": "0198a8e9-0c8a-7d21-9ea9-e9e70d477eed",
    "partNumber": 1,
    "method": "PUT",
    "url": "https://s3.example.local/file-workshop?...",
    "headers": {},
    "expiresAt": "2026-08-10T02:15:00Z"
  },
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

客户端必须使用返回的 `method/url/headers` 上传该分片二进制内容。服务端不接收 Base64 文件内容，也不要求 API 进程整体读取大文件。

### 10.5 取消上传会话

请求：

```json
{
  "rowVersion": 2,
  "reason": "用户取消上传"
}
```

成功后会话状态变为 `ABORTED`，`failureCode=USER_ABORTED`，配额预留标记为 `RELEASED`。

### 10.6 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN/AUTH_ORIGIN_REJECTED` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `UPLOAD_SESSION_NOT_FOUND` | 上传会话、目标文件夹、目标文档或空间不存在、非活动或不可见 |
| 409 | `UPLOAD_CONFLICT` | 上传会话状态、分片范围、配额释放或数据库事实冲突 |
| 409 | `ROW_VERSION_CONFLICT` | `rowVersion` 已过期 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求体 |
| 409 | `STORAGE_UNAVAILABLE` | 对象存储未启用或暂不可用 |
| 409 | `SPACE_QUOTA_EXCEEDED` | 空间容量不足，无法预留本次上传声明大小 |

正式接口测试至少覆盖：未登录拒绝；错误 Origin 拒绝；对象存储禁用时不创建数据库会话；创建会话成功后不暴露对象 Key；幂等重放和幂等冲突；空间配额不足；普通用户越权；`NEW_VERSION` 缺少目标文档拒绝；预签名分片号越界；过期/已取消会话拒绝预签名；按陈旧 `rowVersion` 取消失败；取消后释放配额。

现有自动化证据：

- `backend/internal/modules/uploads/application/service_test.go`
- `backend/internal/platform/objectstorage/disabled_test.go`
- `backend/internal/platform/objectstorage/s3_test.go`
- `backend/scripts/verify.ps1`

## 11. 模块 07：版本与并发

### 11.1 当前边界

本周期完成不依赖真实对象存储集群的版本与锁基础能力。版本恢复通过复用历史版本的 `storage_object_id` 创建新的 `document_versions` 行并切换 `documents.current_version_id`；不会修改历史版本。文件锁使用数据库 `document_lock_counters/document_locks`，明文 `lockToken` 只在获取锁成功响应中返回一次，数据库仅保存 SHA-256 摘要。

当前能力：

- 分页查询文档版本；
- 恢复历史版本为新版本；
- 查询当前活动锁；
- 获取文件级租约锁；
- 续租锁；
- 释放本人持有的锁；
- 强制释放活动锁。

尚未完成：上传完成时创建真实版本、下载当前/历史版本、WebDAV LOCK/UNLOCK 兼容适配、Office/CAD 客户端兼容测试和大规模锁竞争压力测试。

### 11.2 接口清单

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/documents/{documentId}/versions?page=1&pageSize=50` | `listDocumentVersions` | `200/DocumentVersionListResponse` | 需要 `READ_METADATA`；分页统一使用 `page/pageSize` |
| `POST /api/v1/documents/{documentId}/versions/{documentVersionId}/restore` | `restoreDocumentVersion` | `201/DocumentVersionResponse` | 要求 `Idempotency-Key`、`rowVersion`；需要 `MANAGE_VERSION`；恢复创建新版本 |
| `GET /api/v1/documents/{documentId}/lock` | `getDocumentLock` | `200/DocumentLockResponse` | 需要 `READ_METADATA`；无活动锁时 `lock=null` |
| `POST /api/v1/documents/{documentId}/lock` | `acquireDocumentLock` | `201/AcquireDocumentLockResponse` | 需要 `LOCK`；返回一次性 `lockToken` 和单调 `fencingToken` |
| `POST /api/v1/documents/{documentId}/lock/heartbeat` | `heartbeatDocumentLock` | `200/DocumentLockResponse` | 请求体含 `lockToken/rowVersion`；仅锁持有者可续租 |
| `DELETE /api/v1/documents/{documentId}/lock` | `releaseDocumentLock` | `200/DocumentLockResponse` | 请求体含 `lockToken/rowVersion`；仅锁持有者可释放 |
| `POST /api/v1/documents/{documentId}/lock/force-release` | `forceReleaseDocumentLock` | `200/DocumentLockResponse` | 请求体含 `reason/rowVersion`；当前仅 `SYSTEM_ADMIN` 可强制释放 |

### 11.3 字段和示例

`DocumentVersion` 字段严格来自 `document_versions`：`documentVersionId/documentId/versionNumber/storageObjectId/sizeBytes/sha256Hex/mimeType/changeNote/sourceType/restoredFromVersionId/createdByUserId/createdAt`。

恢复请求示例：

```json
{
  "rowVersion": 3,
  "changeNote": "恢复到已确认的工艺图纸版本"
}
```

获取锁响应示例：

```json
{
  "lock": {
    "documentLockId": "0198b100-0000-7000-8000-000000000090",
    "documentId": "0198b100-0000-7000-8000-000000000001",
    "userId": "0198b100-0000-7000-8000-000000000010",
    "fencingToken": 1,
    "source": "WEB",
    "status": "ACTIVE",
    "acquiredAt": "2026-08-10T03:00:00Z",
    "heartbeatAt": "2026-08-10T03:00:00Z",
    "expiresAt": "2026-08-10T03:15:00Z",
    "createdAt": "2026-08-10T03:00:00Z",
    "updatedAt": "2026-08-10T03:00:00Z",
    "rowVersion": 1
  },
  "lockToken": "一次性明文锁令牌",
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

### 11.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN/AUTH_ORIGIN_REJECTED` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `DOCUMENT_VERSION_NOT_FOUND` | 文档、版本或锁不存在 |
| 409 | `VERSION_CONFLICT` | 版本或锁状态冲突 |
| 409 | `ROW_VERSION_CONFLICT` | `rowVersion` 已过期 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求 |
| 409 | `FILE_LOCKED` | 文档已有未过期活动锁 |

正式接口测试至少覆盖：未登录拒绝；普通用户越权；分页默认值、非法值和最大值；恢复历史版本创建新版本且不修改旧版本；恢复遇到活动锁时拒绝；幂等重放和冲突；活动锁查询；获取锁返回一次性令牌但不暴露摘要；已有锁再次获取失败；续租要求持有者和正确 `lockToken`；陈旧 `rowVersion` 失败；释放后可重新获取；强制释放必须有原因且仅系统管理员可执行。

现有自动化证据：

- `backend/internal/modules/versions/application/service_test.go`
- `backend/scripts/verify.ps1`

## 12. 模块 08：共享

### 12.1 当前边界

本周期完成不依赖对象存储集群的内部共享基础能力。共享事实写入 `shares/share_actions`，共享不复制源 Document/Folder，也不写入普通 ACL。权限模块已接入有效 `USER/ORGANIZATION` 共享作为最终授权允许来源；`LINK` 共享必须通过共享打开接口校验一次性明文 `shareToken`，不会在普通文件 API 中让所有登录用户自动获得权限。

当前能力：

- 创建 `USER`、`ORGANIZATION`、`LINK` 内部共享；
- 查询我创建的共享；
- 查询共享给我的用户/组织共享；
- 查询共享详情；
- 修改共享动作、到期时间和 `allowReshare`；
- 撤销共享；
- 打开共享并写入 Outbox 事件；
- 共享动作仅允许 `READ_METADATA/PREVIEW/DOWNLOAD/WRITE_CONTENT`，且 `READ_METADATA` 必选。

尚未完成：`SPACE` 目标共享和 `shared_entries` 命名空间引用、外部匿名链接、链接密码、共享下载/预览数据面、共享访问 HTTP 集成测试和大规模共享性能压测。

### 12.2 接口清单

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `POST /api/v1/shares` | `createShare` | `201/ShareResponse` | 要求 `Idempotency-Key`；创建者必须具备源资源 `SHARE` 和所授予动作权限；`LINK` 创建成功时返回一次性 `shareToken` |
| `GET /api/v1/shares/created?page=1&pageSize=50` | `listCreatedShares` | `200/ShareListResponse` | 查询当前用户创建的共享；分页统一使用 `page/pageSize` |
| `GET /api/v1/shares/received?page=1&pageSize=50` | `listReceivedShares` | `200/ShareListResponse` | 查询共享给当前用户或其所属组织的有效共享；不枚举 LINK |
| `GET /api/v1/shares/{shareId}` | `getShare` | `200/ShareResponse` | 创建者、接收者或系统管理员可见 |
| `PATCH /api/v1/shares/{shareId}` | `updateShare` | `200/ShareResponse` | 创建者、系统管理员或具备源资源 `MANAGE_PERMISSION` 的主体可修改；要求 `rowVersion` |
| `POST /api/v1/shares/{shareId}/revoke` | `revokeShare` | `200/ShareResponse` | 创建者、系统管理员或具备源资源 `MANAGE_PERMISSION` 的主体可撤销；要求 `reason/rowVersion` |
| `POST /api/v1/shares/{shareId}/open` | `openShare` | `200/OpenShareResponse` | 打开共享并审计；LINK 共享必须在请求体提交正确 `shareToken` |

### 12.3 字段和示例

`Share` 字段严格来自 `shares/share_actions`：`shareId/sourceType/sourceId/creatorUserId/targetKind/targetUserId/targetOrganizationId/targetSpaceId/allowReshare/actions/validFrom/validUntil/status/createdAt/updatedAt/revokedAt/revokedByUserId/revokeReason/rowVersion`。`token_hash/password_hash` 不对外返回。

创建用户共享示例：

```json
{
  "sourceType": "DOCUMENT",
  "sourceId": "0198b100-0000-7000-8000-000000000001",
  "targetKind": "USER",
  "targetUserId": "0198b100-0000-7000-8000-000000000010",
  "actions": ["READ_METADATA", "PREVIEW", "DOWNLOAD"],
  "allowReshare": false,
  "validUntil": "2026-12-31T15:59:59Z"
}
```

创建 LINK 共享成功响应会额外包含一次性 `shareToken`：

```json
{
  "share": {
    "shareId": "0198b100-0000-7000-8000-000000000080",
    "sourceType": "DOCUMENT",
    "sourceId": "0198b100-0000-7000-8000-000000000001",
    "creatorUserId": "0198b100-0000-7000-8000-000000000020",
    "targetKind": "LINK",
    "allowReshare": false,
    "actions": ["READ_METADATA"],
    "validFrom": "2026-08-10T04:00:00Z",
    "status": "ACTIVE",
    "createdAt": "2026-08-10T04:00:00Z",
    "updatedAt": "2026-08-10T04:00:00Z",
    "rowVersion": 1
  },
  "shareToken": "仅本次响应返回的明文内部链接令牌",
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

打开 LINK 共享请求示例：

```json
{
  "shareToken": "创建 LINK 共享时返回的一次性明文令牌"
}
```

### 12.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN/AUTH_ORIGIN_REJECTED` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `SHARE_NOT_FOUND` | 共享不存在或不可见 |
| 409 | `SHARE_CONFLICT` | 共享状态冲突 |
| 409 | `ROW_VERSION_CONFLICT` | `rowVersion` 已过期 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求 |
| 409 | `SHARE_TOKEN_INVALID` | LINK 共享令牌缺失或无效 |

正式接口测试至少覆盖：未登录拒绝；创建者没有 `SHARE` 拒绝；创建者授予自身没有的动作拒绝；共享动作缺少 `READ_METADATA` 拒绝；重复幂等键重放和冲突；用户共享接收者可见；组织共享成员可见；LINK 不进入共享给我的列表；LINK 打开必须提交正确 `shareToken`；撤销后不可打开；过期共享不可打开；非创建者修改/撤销拒绝；`page/pageSize` 非法值拒绝。

现有自动化证据：

- `backend/internal/modules/shares/application/service_test.go`
- `backend/internal/modules/permissions/application` 已覆盖权限判定编译与单元测试回归

## 13. 模块 09：回收与生命周期

### 13.1 当前边界

本周期完成不依赖对象存储集群的回收站元数据闭环。回收站事实写入 `recycle_items`，目录项状态写入 `namespace_entries.lifecycle_status`；进入回收站时将资源子树标记为 `TRASHED`，不会立即删除对象存储数据，也不会破坏版本、授权和审计引用。

当前能力：

- 文件或文件夹进入回收站；
- 分页查询回收站项目；
- 从回收站恢复到原父目录或指定父目录；
- 恢复时支持指定新名称，并校验同目录重名冲突；
- 发起永久清理，将目录子树标记为 `PURGING`，并写入 Outbox 事件等待后续 Worker；
- 删除时把源资源相关共享标记为 `SOURCE_UNAVAILABLE`；
- 删除、恢复和清理都会递增空间安全纪元，确保权限/搜索等后续缓存可失效；
- 法务保留存在时阻断永久清理。

尚未完成：自动到期扫描、批量清理 Job、法务保留创建/解除接口、归档/冷存储策略、真实对象存储删除、版本对象引用计数、HTTP 集成测试和大目录性能压测。

### 13.2 接口清单

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `POST /api/v1/entries/{entryId}/trash` | `trashDirectoryEntry` | `201/RecycleItemResponse` | 要求 `Idempotency-Key`；根目录不可删除；要求资源 `DELETE` 权限；要求 `rowVersion` |
| `GET /api/v1/recycle-bin?page=1&pageSize=50` | `listRecycleBinItems` | `200/RecycleItemListResponse` | 分页统一使用 `page/pageSize`；可选 `spaceId`；仅返回通过 `RESTORE` 权限复核的项目；当前 `total` 为本页可见数量，避免泄露无权项目总量 |
| `POST /api/v1/recycle-bin/{recycleItemId}/restore` | `restoreRecycleItem` | `200/RecycleItemResponse` | 要求回收项 `rowVersion`；要求源资源 `RESTORE` 权限和目标父目录 `UPLOAD/CREATE_FOLDER` 权限；同名冲突返回 409 |
| `POST /api/v1/recycle-bin/{recycleItemId}/purge` | `purgeRecycleItem` | `200/RecycleItemResponse` | 要求回收项 `rowVersion` 和 `reason`；要求源资源 `PURGE` 权限；存在有效法务保留时拒绝 |

### 13.3 字段和示例

`RecycleItem` 字段严格来自 `recycle_items` 与关联 `namespace_entries`：`recycleItemId/entryId/entryType/originalSpaceId/originalParentFolderId/originalName/currentName/lifecycleStatus/deletedByUserId/deletedAt/expiresAt/status/restoredToFolderId/restoredAt/createdAt/updatedAt/rowVersion`。

进入回收站请求示例：

```http
POST /api/v1/entries/0198b100-0000-7000-8000-000000000101/trash HTTP/1.1
Host: 127.0.0.1:8080
Authorization: Bearer <accessToken>
Idempotency-Key: trash-0198b100-0000-7000-8000-000000000101-1
Content-Type: application/json

{
  "rowVersion": 3,
  "reason": "用户主动删除"
}
```

成功响应示例：

```json
{
  "item": {
    "recycleItemId": "0198b100-0000-7000-8000-000000000201",
    "entryId": "0198b100-0000-7000-8000-000000000101",
    "entryType": "DOCUMENT",
    "originalSpaceId": "0198b100-0000-7000-8000-000000000001",
    "originalParentFolderId": "0198b100-0000-7000-8000-000000000010",
    "originalName": "设计说明.docx",
    "currentName": "设计说明.docx",
    "lifecycleStatus": "TRASHED",
    "deletedByUserId": "0198b100-0000-7000-8000-000000000020",
    "deletedAt": "2026-08-10T09:00:00Z",
    "expiresAt": "2026-11-08T09:00:00Z",
    "status": "ACTIVE",
    "createdAt": "2026-08-10T09:00:00Z",
    "updatedAt": "2026-08-10T09:00:00Z",
    "rowVersion": 1
  },
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

恢复请求示例：

```json
{
  "rowVersion": 1,
  "targetParentFolderId": "0198b100-0000-7000-8000-000000000010",
  "name": "设计说明-恢复.docx"
}
```

永久清理请求示例：

```json
{
  "rowVersion": 1,
  "reason": "超过保留期并经管理员确认"
}
```

注意：永久清理接口当前只把资源标记为 `PURGING` 并写入 `ENTRY_PURGE_REQUESTED` Outbox 事件；实际对象存储删除、版本对象引用计数和最终 `PURGED` 收敛由后续 Worker 周期实现。

### 13.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN/AUTH_ORIGIN_REJECTED` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `RECYCLE_ITEM_NOT_FOUND` | 回收站项目、目录项或目标父目录不存在/不可见 |
| 409 | `RECYCLE_CONFLICT` | 生命周期状态冲突 |
| 409 | `ROW_VERSION_CONFLICT` | `rowVersion` 已过期或资源不在可操作状态 |
| 409 | `IDEMPOTENCY_CONFLICT` | 同一幂等键对应不同请求 |
| 409 | `RECYCLE_ROOT_OPERATION_FORBIDDEN` | 根目录不允许进入回收站 |
| 409 | `RECYCLE_NAME_CONFLICT` | 恢复目标位置已存在同名活动文件或文件夹 |
| 409 | `LEGAL_HOLD_ACTIVE` | 存在有效法务保留，不能永久清理 |

正式接口测试至少覆盖：未登录拒绝；普通用户越权；根目录删除拒绝；进入回收站成功并生成 `recycle_items`；重复幂等键重放和冲突；删除后共享变为 `SOURCE_UNAVAILABLE`；回收站列表分页默认值、非法值和最大值；无 `RESTORE` 权限的项目不返回；恢复原位置；恢复到指定父目录；恢复重名冲突；陈旧 `rowVersion` 拒绝；永久清理要求原因；法务保留阻断清理；清理只进入 `PURGING` 并产生 Outbox，不伪造对象存储删除完成。

现有自动化证据：

- `backend/internal/modules/lifecycle/application/service_test.go`
- `go test ./...`

## 14. 模块 10：搜索

### 14.1 当前边界

本周期完成 Windows 本地可验证的 PostgreSQL 元数据搜索基础能力。搜索不依赖对象存储、预览、OCR、AI、OpenSearch/Elasticsearch 或 pgvector；只从数据库事实和投影表生成候选，并在返回前调用权限模块执行 `READ_METADATA` 最终复核。

当前能力：

- `GET /api/v1/search` 搜索活动目录项；
- 关键词匹配文件夹/文件名称、规范化名称、扩展名和分类；
- 支持 `spaceId/entryType/extension/classification/createdByUserId/updatedFrom/updatedTo/metadataKey/metadataValue` 筛选；
- 只返回 `namespace_entries.lifecycle_status=ACTIVE` 的资源；
- 返回结果复用 `DirectoryEntry` 结构，并附带 `matchedFields/indexStatus/source`；
- 响应 `degraded=true`，明确当前为 PostgreSQL 元数据搜索，全文/OCR/语义搜索尚未启用；
- 返回前逐项执行最终授权，权限拒绝的候选被过滤。

尚未完成：全文内容索引、OCR、外部搜索服务、语义检索、索引刷新后台任务、权限安全的跨页精确 `total`、HTTP 集成测试和大数据量性能压测。

### 14.2 接口清单

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/search?page=1&pageSize=50&query=工艺` | `searchDirectoryEntries` | `200/SearchResultListResponse` | 至少提供一个查询条件；统一使用 `page/pageSize`；仅搜索 ACTIVE 资源；返回前按资源 `READ_METADATA` 最终复核；当前 `total` 为本页可见数量 |

### 14.3 查询参数和响应字段

查询参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `query` | string | 可选，1～128 字符；匹配名称、规范化名称、扩展名和分类 |
| `spaceId` | uuid | 可选，限定空间 |
| `entryType` | `FOLDER/DOCUMENT` | 可选，限定目录项类型 |
| `extension` | string | 可选，文档扩展名，不包含点号 |
| `classification` | string | 可选，文档分级 |
| `createdByUserId` | uuid | 可选，创建者 |
| `updatedFrom/updatedTo` | date-time | 可选，更新时间范围；`updatedFrom` 不得晚于 `updatedTo` |
| `metadataKey/metadataValue` | string | 必须成对出现；匹配 `documents.metadata_json ->> metadataKey` |

`SearchResult` 字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `entry` | `DirectoryEntry` | 复用目录模块响应结构 |
| `matchedFields` | array | `NAME/EXTENSION/CLASSIFICATION/METADATA` |
| `indexStatus` | string | 可选，来自 `document_index_states.status`；文件夹或未建立索引状态时为空 |
| `source` | string | 当前固定为 `POSTGRES_METADATA` |

搜索响应示例：

```json
{
  "items": [
    {
      "entry": {
        "entryId": "0198b100-0000-7000-8000-000000000101",
        "spaceId": "0198b100-0000-7000-8000-000000000001",
        "entryType": "DOCUMENT",
        "name": "工艺卡.docx",
        "normalizedName": "工艺卡.docx",
        "depth": 2,
        "lifecycleStatus": "ACTIVE",
        "isRoot": false,
        "createdByUserId": "0198b100-0000-7000-8000-000000000020",
        "createdAt": "2026-08-10T09:00:00Z",
        "updatedAt": "2026-08-10T09:00:00Z",
        "rowVersion": 1,
        "availabilityStatus": "BLOCKED",
        "extensionNormalized": "docx"
      },
      "matchedFields": ["NAME"],
      "indexStatus": "PENDING",
      "source": "POSTGRES_METADATA"
    }
  ],
  "page": 1,
  "pageSize": 50,
  "total": 1,
  "degraded": true,
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

### 14.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN` 外，本模块当前不新增专属错误码。

正式接口测试至少覆盖：未登录拒绝；至少一个查询条件；非法 `page/pageSize`；非法 `entryType`；`metadataKey/metadataValue` 不成对拒绝；`updatedFrom > updatedTo` 拒绝；关键词搜索名称；扩展名、分类、空间和 metadata 筛选；TRASHED/PURGING/PURGED 不返回；权限拒绝候选被过滤；权限服务错误不能被吞掉；响应 `degraded=true`；`total` 为本页可见数量。

现有自动化证据：

- `backend/internal/modules/search/application/service_test.go`
- `go test ./internal/modules/search/...`

## 15. 模块 16：后台任务

### 15.1 当前边界

本周期完成后台任务模块的基础调度与管理员运维接口。模块仍不实现具体业务处理器，例如审计归档、文件 Hash、病毒扫描、预览、搜索索引、生命周期清理或 AI 任务；这些处理器由对应业务模块后续注册。当前 REST API 只面向 `SYSTEM_ADMIN`，用于查看 Outbox/Job 积压与失败项，并对 `FAILED/DEAD` 单项执行受控重试。

当前能力：

- `cmd/worker` 可作为独立 Go 进程启动；
- Worker 使用 PostgreSQL `outbox_events` 和 `background_jobs` 作为任务事实源；
- 只领取已注册处理器声明支持的 `eventType/jobType`，未注册事件或任务继续保留，不被空消费；
- 领取使用 `FOR UPDATE SKIP LOCKED`、状态条件、到期时间和租约；
- 成功处理 Outbox 标记为 `PUBLISHED`，成功处理 Job 标记为 `SUCCESS`；
- 可重试失败标记 `FAILED`，写入错误码、错误摘要和下一次可用时间；
- 永久失败或重试耗尽标记 `DEAD`；
- 管理员可分页查询 Outbox 事件和后台任务，并对 `FAILED/DEAD` 项按 `rowVersion` 受控重试；
- 使用 `context.Context`、处理器超时和系统信号完成优雅停止；
- Redis 不参与任务事实存储。

### 15.2 配置项

| 环境变量 | 默认值 | 含义 |
|---|---:|---|
| `FILE_WORKSHOP_WORKER_ID` | 自动生成 | Worker 实例标识；写入 `outbox_events.locked_by` |
| `FILE_WORKSHOP_WORKER_CONCURRENCY` | `2` | 并发轮询 goroutine 数 |
| `FILE_WORKSHOP_WORKER_BATCH_SIZE` | `10` | 单次每类事件最多领取数量 |
| `FILE_WORKSHOP_WORKER_POLL_INTERVAL` | `1s` | 无事件时轮询间隔 |
| `FILE_WORKSHOP_WORKER_LEASE_DURATION` | `30s` | 领取租约时长 |
| `FILE_WORKSHOP_WORKER_HANDLER_TIMEOUT` | `20s` | 单个事件处理超时，必须短于租约 |
| `FILE_WORKSHOP_WORKER_RETRY_INITIAL_BACKOFF` | `5s` | 首次失败退避 |
| `FILE_WORKSHOP_WORKER_RETRY_MAX_BACKOFF` | `5m` | 最大失败退避 |
| `FILE_WORKSHOP_WORKER_SHUTDOWN_TIMEOUT` | `15s` | 收到退出信号后的等待时长 |

GoLand 本地测试可直接运行：

```powershell
cd backend
go run ./cmd/worker
```

注意：模块 11 审计消费者已注册用户、组织、权限和文件目录模块当前产生的 Outbox 事件；未注册事件类型和未注册 Job 类型仍会继续保留，不会被空消费。

### 15.3 管理员运维接口

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/admin/background/outbox-events?page=1&pageSize=50` | `listBackgroundOutboxEvents` | `200/BackgroundOutboxEventListResponse` | 仅 `SYSTEM_ADMIN`；可按 `status/eventType` 筛选；分页统一使用 `page/pageSize` |
| `POST /api/v1/admin/background/outbox-events/{outboxEventId}/retry` | `retryBackgroundOutboxEvent` | `200/BackgroundOutboxEventResponse` | 仅允许 `FAILED/DEAD`；请求必须包含 `rowVersion` 和 `reason`；成功后回到 `PENDING` 并清理锁与错误重试状态 |
| `GET /api/v1/admin/background/jobs?page=1&pageSize=50` | `listBackgroundJobs` | `200/BackgroundJobListResponse` | 仅 `SYSTEM_ADMIN`；可按 `status/jobType` 筛选；分页统一使用 `page/pageSize` |
| `POST /api/v1/admin/background/jobs/{backgroundJobId}/retry` | `retryBackgroundJob` | `200/BackgroundJobResponse` | 仅允许 `FAILED/DEAD`；请求必须包含 `rowVersion` 和 `reason`；成功后回到 `PENDING` 并清理锁、心跳、开始/完成和错误状态 |

重试请求示例：

```json
{
  "rowVersion": 3,
  "reason": "人工确认依赖已恢复，允许重新执行"
}
```

列表响应均包含：

```json
{
  "items": [],
  "page": 1,
  "pageSize": 50,
  "total": 0,
  "requestId": "019fd14d-c956-7f0e-a061-e5ee440d77b1"
}
```

Outbox 事件响应字段严格来自 `outbox_events`：`outboxEventId/aggregateType/aggregateId/aggregateVersion/eventType/eventSchemaVersion/payload/deduplicationKey/correlationId/causationId/priority/status/attemptCount/maxAttempts/availableAt/lockedBy/lockedAt/leaseUntil/nextRetryAt/publishedAt/lastErrorCode/lastErrorSummary/createdAt/updatedAt/rowVersion`。

后台任务响应字段严格来自 `background_jobs`：`backgroundJobId/jobType/targetDocumentId/targetDocumentVersionId/targetStorageObjectId/payloadSchemaVersion/payload/deduplicationKey/priority/status/attemptCount/maxAttempts/availableAt/lockedBy/lockedAt/leaseUntil/heartbeatAt/startedAt/completedAt/lastErrorCode/lastErrorSummary/createdAt/updatedAt/rowVersion`。

### 15.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `BACKGROUND_ITEM_NOT_FOUND` | 指定 Outbox 事件或后台任务不存在 |
| 409 | `BACKGROUND_STATE_CONFLICT` | 当前状态不是 `FAILED/DEAD`，或 `rowVersion` 已过期 |

正式接口测试至少覆盖：按状态查询 Outbox/Job 积压；查询失败原因和最近错误；受控重放 `FAILED/DEAD`；普通用户访问运维接口被拒绝；分页统一使用 `page/pageSize`；非法状态、非法分页、缺少 `reason`、陈旧 `rowVersion` 和错误 Origin。

现有自动化证据：

- `backend/internal/modules/background/application/runner_test.go`
- `backend/internal/modules/background/application/job_runner_test.go`
- `backend/tests/integration/background_worker_test.go`
- `backend/tests/integration/background_admin_http_test.go`
- `backend/scripts/verify.ps1`

## 16. 模块 11：审计

### 16.1 当前边界

本周期完成审计模块的基础查询、详情、完整性状态和哈希链校验能力，并将用户、组织、权限、文件目录模块当前产生的 Outbox 事件注册为审计消费者。当前不实现审计导出、归档、WORM、批次锚定和安全告警；这些能力依赖后续对象存储与归档周期。

当前审计写入链路为：

1. 业务模块在自身事务内写入业务事实和 `outbox_events`；
2. `cmd/worker` 只领取已注册的 Outbox 事件类型；
3. 审计消费者按数据库设计写入 `audit_events`；
4. 高风险事件写入 `audit_chain_heads` 哈希链，使用 `partition_date + chain_id` 定位链；
5. 写入失败由后台任务框架按可重试失败处理，不静默吞掉。

所有 REST 查询接口当前仅允许 `SYSTEM_ADMIN` 访问。分页统一使用 `page/pageSize`，默认 `1/50`，最大 `pageSize=200`。

### 16.2 审计接口

| 接口 | Operation ID | 成功响应 | 关键约束 |
|---|---|---:|---|
| `GET /api/v1/audit/events?dateFrom=2026-08-10&dateTo=2026-08-10&page=1&pageSize=50` | `listAuditEvents` | `200/AuditEventListResponse` | 仅 `SYSTEM_ADMIN`；`dateFrom/dateTo` 必填；支持 `eventType/riskLevel/actorType/actorId/resourceType/resourceId/result/requestId` 筛选；排序为 `createdAt DESC, auditEventId DESC` |
| `GET /api/v1/audit/events/{auditEventId}?partitionDate=2026-08-10` | `getAuditEvent` | `200/AuditEventResponse` | 仅 `SYSTEM_ADMIN`；必须携带 `partitionDate`，因为 `audit_events` 按 `partition_date` 分区并以 `(partition_date, audit_event_id)` 定位 |
| `GET /api/v1/audit/integrity?dateFrom=2026-08-10&dateTo=2026-08-10&page=1&pageSize=50` | `getAuditIntegrity` | `200/AuditIntegrityResponse` | 仅 `SYSTEM_ADMIN`；返回 `audit_chain_heads`；可按 `status=ACTIVE/SEALED/INVALID` 筛选 |
| `POST /api/v1/audit/integrity/verify` | `verifyAuditIntegrity` | `200/AuditIntegrityVerificationResponse` | 仅 `SYSTEM_ADMIN`；请求体包含 `chainId/partitionDate`；按链内 `sequenceNumber` 重新计算 SHA-256 哈希并更新 `verifiedAt`，发现不一致时标记链为 `INVALID` |

完整性校验请求示例：

```json
{
  "chainId": "fw-audit:20260810:USER_ROLE_CHANGED",
  "partitionDate": "2026-08-10"
}
```

列表响应示例：

```json
{
  "items": [],
  "page": 1,
  "pageSize": 50,
  "total": 0,
  "requestId": "019fcc32-0bc6-7d82-aa70-62d5324b1fbb"
}
```

### 16.3 字段与映射

`AuditEvent` 字段严格来自 `audit_events` 和 OpenAPI：`auditEventId/eventType/riskLevel/actorType/actorId/actorDisplayName/actorEmployeeNo/effectiveRole/adminDelegationId/shareId/resourceType/resourceId/resourceName/spaceId/organizationId/documentId/documentVersionId/action/result/failureCode/sourceChannel/ipAddress/userAgent/requestId/traceId/correlationId/reason/metadataSchemaVersion/metadata/hashSchemaVersion/chainId/sequenceNumber/previousHash/eventHash/partitionDate/createdAt`。

Outbox 审计消费者的映射规则：

- `eventType/action` 使用 Outbox `event_type`；
- `resourceType/resourceId` 使用 Outbox `aggregate_type/aggregate_id`；
- `requestId` 优先使用 Outbox `correlation_id`；缺失时使用 `outbox_event_id` 兜底，并在 `metadata.requestIdFallback=true` 标记；
- payload 中存在 `actorUserId` 时映射为 `actorType=USER/actorId`，否则为 `SYSTEM`；
- payload 中存在 `reason/spaceId/organizationId/documentId/documentVersionId` 时按同名字段映射；
- `metadata` 保留 `outboxEventId/aggregateType/aggregateId/aggregateVersion/eventSchemaVersion/deduplicationKey/sourcePayload/mappedBy` 等追踪信息；
- `USER_ROLE_CHANGED`、`AUTH_PASSWORD_CHANGED`、权限与管理委派变更等高风险事件进入哈希链；`AUTH_ACCOUNT_LOCKED` 视为 `CRITICAL`。

### 16.4 错误码和接口测试依据

除公共 `INVALID_REQUEST/AUTH_REQUIRED/AUTH_FORBIDDEN` 外，本模块使用：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 404 | `AUDIT_NOT_FOUND` | 审计事件、链头或指定分区内链事件不存在 |
| 409 | `AUDIT_STATE_CONFLICT` | 审计链状态不允许继续写入或校验操作 |

正式接口测试至少覆盖：未登录拒绝；普通用户拒绝；系统管理员按日期分页查询；非法 `page/pageSize`；缺失或倒置 `dateFrom/dateTo`；按事件类型、风险级别、主体、资源和 Request ID 筛选；按 `partitionDate + auditEventId` 读取详情；高风险事件进入哈希链；完整链校验通过；篡改或缺失事件导致校验失败并标记 `INVALID`；Outbox 缺失 `correlationId` 时 requestId 兜底且 metadata 可追踪。

现有自动化证据：

- `backend/internal/modules/audit/application/outbox_handler_test.go`
- `backend/internal/modules/audit/domain/hash_test.go`
- `backend/scripts/verify.ps1`
- `go test ./internal/modules/audit/... ./...`

## 17. 文档维护与冻结规则

1. 新模块先更新 `backend/api/openapi.yaml`，再生成代码和实现。
2. 模块开发完成时，在本文档追加对应模块章节和接口测试要点，并更新第 2 章状态。
3. 接口发生变化时，同步修改 OpenAPI、实现、测试和本文档，不保留长期漂移的别名或旧结构。
4. 后端 16 个模块完成后，逐项核对路由、Operation ID、认证、字段、状态码、错误码和示例，形成 V1.0 冻结版本。
5. 正式接口测试以冻结版本为准；冻结前的模块测试以本文档当前模块章节和 OpenAPI 为准。
