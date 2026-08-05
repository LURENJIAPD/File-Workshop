# File Workshop V1.0 API 接口文档

> 文档编号：FW-API-V1.0  
> 文档版本：V0.1  
> 文档状态：按模块持续编制  
> 最近更新：2026-08-05  
> 当前已收录：公共健康检查、模块 01 身份认证  
> 机器契约：`backend/api/openapi.yaml`

## 1. 文档定位

本文档是后端开发、前后端联调和接口测试共同使用的可读接口说明，按照设计文档的系统模块 01～16 持续追加。每完成一个模块，必须在同一开发周期补齐该模块章节；全部后端模块完成后只进行全量复核和版本冻结。

OpenAPI 是机器可读的唯一权威契约。本文档必须与 OpenAPI、生成代码和实际实现保持一致；如有冲突，应停止测试和联调，先修复契约或文档偏差。

## 2. 模块收录状态

| 顺序 | 模块 | 文档状态 | 接口数量 | 最近验证 |
|---:|---|---|---:|---|
| 公共 | 健康检查 | 已完成 | 2 | 2026-08-05 |
| 01 | 身份认证 | 已完成 | 4 | 2026-08-05 |
| 02～16 | 后续系统模块 | 未收录 | 0 | — |

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

## 6. 文档维护与冻结规则

1. 新模块先更新 `backend/api/openapi.yaml`，再生成代码和实现。
2. 模块开发完成时，在本文档追加对应模块章节和接口测试要点，并更新第 2 章状态。
3. 接口发生变化时，同步修改 OpenAPI、实现、测试和本文档，不保留长期漂移的别名或旧结构。
4. 后端 16 个模块完成后，逐项核对路由、Operation ID、认证、字段、状态码、错误码和示例，形成 V1.0 冻结版本。
5. 正式接口测试以冻结版本为准；冻结前的模块测试以本文档当前模块章节和 OpenAPI 为准。
