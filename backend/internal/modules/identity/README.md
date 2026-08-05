# 身份认证模块工程说明

本目录对应开发设计文档第 6.2 节，只负责登录身份与会话上下文，不负责文件资源权限判断。

## 当前实现

- `POST /api/v1/auth/login`：用户名和密码登录；创建 `user_sessions` 与首个 `session_refresh_tokens`。
- `POST /api/v1/auth/refresh`：单事务锁定并消费旧 Refresh Token，签发下一代 Token；检测旧 Token 重放后撤销整个会话和活动 Token。
- `POST /api/v1/auth/logout`：按访问令牌或 Refresh Token 定位并幂等撤销会话。
- `GET /api/v1/auth/session`：同时校验 JWT、用户状态、数据库会话状态与过期时间。
- Argon2id PHC 密码哈希；已冻结 12～128 字符、拒绝用户名、拒绝常见弱密码、最近 5 个密码不可复用的策略。
- 同一规范化用户名 15 分钟内连续失败 5 次后锁定 15 分钟；成功登录会重置失败窗口。
- 单实例 IP 短时突发限制；登录结果写入 `login_attempts`。
- 浏览器令牌使用 HttpOnly Cookie，生产环境要求 Secure；带 Origin 的认证写请求执行允许列表校验，并为允许的前端源提供携带凭据的 CORS 预检响应。

所有持久化字段均来自数据库设计中的 `users`、`user_credentials`、`user_password_history`、`user_sessions`、`session_refresh_tokens` 与 `login_attempts`，没有另建身份数据模型。

## 令牌约定

- Access Token：HS256 JWT，默认 15 分钟；验证签名算法、Issuer、Audience、时间和会话 ID。
- Refresh Token：32 字节密码学安全随机值，默认会话绝对有效期 7 天；数据库只保存 SHA-256 摘要。
- 浏览器响应正文只返回 Access Token，不返回 Refresh Token。
- Access Cookie 路径为 `/`；Refresh Cookie 路径为 `/api/v1/auth`。

## 尚未开放

- MFA：数据库结构和 `application.MFAAdapter`/`TOTPSecretVerifier` 边界已准备，但 TOTP 的 `secret_ref` 需要 Vault/KMS 类密钥提供器；WebAuthn 还依赖前端流程。本阶段不开放伪实现路由。
- LDAP、OIDC、AD：保留数据库凭据类型和 `application.ExternalIdentityAdapter` 边界，待外部身份源确定后实现。
- 高风险认证事件的不可篡改哈希链审计由“模块 11：审计与追踪”统一实现；本模块当前保存登录尝试事实并输出不含密码和 Token 的结构化 HTTP 日志。
- 应用账号不等于 PostgreSQL 登录用户。当前没有自动创建业务 `root` 用户，避免在未确定初始业务密码时写入弱口令；由模块 02 的受控账号创建流程处理。

## 验证

```powershell
cd backend
./scripts/verify.ps1
./scripts/verify-integration.ps1
```

集成测试覆盖登录、当前会话、Refresh Token 轮换、旧 Token 重放、并发刷新、会话撤销和退出，并使用唯一测试用户后清理数据。
