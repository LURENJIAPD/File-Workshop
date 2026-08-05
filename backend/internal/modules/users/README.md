# 模块 02：用户管理

本模块负责用户资料、系统角色、账号状态、逻辑删除、管理员密码重置和本人会话管理。

## 已实现能力

- `GET/PATCH /api/v1/users/me`：读取和修改本人资料，使用 `rowVersion` 防止并发覆盖。
- `GET/DELETE /api/v1/users/me/sessions`：分页查看并撤销本人会话。
- `/api/v1/admin/users`：`SYSTEM_ADMIN` 分页查询、创建、读取和修改用户。
- 管理员启用、禁用、锁定和逻辑删除用户；至少保留一个活动 `SYSTEM_ADMIN`。
- 创建用户要求 `Idempotency-Key`；相同键和相同请求返回同一用户，不同请求返回冲突。
- 用户创建和管理员密码重置复用模块 01 的 Argon2id 与密码策略，不实施密码历史限制。
- 禁用、锁定、删除或重置密码时，同一事务撤销相关 Session 与活动 Refresh Token。
- 角色和状态变化递增 `principal_security_versions.global_authorization_version`。
- 用户变更与 Outbox 在同一 PostgreSQL 事务写入，为模块 11 审计和模块 16 Worker 保留可靠处理入口。

## 模块边界

- 本模块以 `users` 为用户事实，受控写入 `user_credentials`、`user_sessions`、`session_refresh_tokens`、`principal_security_versions`、`idempotency_records` 和 `outbox_events`。
- 用户名永久不可复用；逻辑删除只更新状态和时间，不物理删除文件、版本、权限、共享或审计引用。
- 用户组织关系和个人空间由模块 03 管理；收藏和最近访问由对应文件资源模块管理。用户创建事件是后续协作入口，本模块不绕过边界直接写入这些模块的内部表。

## 验证

- 单元测试：`go test ./internal/modules/users/...`
- 本地真实依赖测试：`backend/scripts/verify-integration.ps1`
- 全量验证：`backend/scripts/verify.ps1`
