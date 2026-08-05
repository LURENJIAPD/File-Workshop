# 模块 04：权限与管理委派

本模块对应开发设计文档第 6.5 节，是 Web、REST、搜索、共享、WebDAV、AI 与后台任务统一复用的唯一资源授权判定入口。

## 已实现能力

- 系统角色、组织管理委派和普通文件 ACL 分离；组织成员关系只用于展开授权主体，不自动授予文件权限。
- `SELF/SUBTREE` 管理委派、七类管理能力、向下递归委派、父能力/范围/有效期上限、父链有效性和撤销后代立即失效。
- 用户和组织主体对 Space、Folder、Document 的显式 `ALLOW`，支持直接授权、向后代继承、有效期、修改、撤销和乐观锁。
- 个人空间所有者隐式完整权限；组织管理委派不跨入个人空间；系统管理员敏感访问必须提供原因和二次确认标志。
- Folder/Document 的 `INHERIT/BREAK`；断开继承只阻断祖先 ACL，不阻断直接 ACL、所有者、系统管理员或管理委派。
- 单项和最多 100 项批量最终判定；无匹配默认拒绝，不存在资源也返回拒绝结果，避免通过判定接口枚举资源。
- 委派、ACL、资源 ACL 版本、空间安全纪元、用户/组织安全版本、幂等记录和 Outbox 在同一 PostgreSQL 事务更新。
- Redis 30 秒版本化判定缓存；版本 Key 覆盖成员、委派、直接授权、共享、全局授权、空间和组织安全版本，Redis 故障回源 PostgreSQL。

## 数据库边界

字段严格来自数据库设计：`admin_delegations`、`admin_delegation_capabilities`、`permission_grants`、`permission_grant_actions`、`principal_security_versions`、`organization_security_versions`、`spaces.acl_version/security_epoch`、`folders/documents.inheritance_mode/acl_version`。模块只写授权事实及这些授权安全投影，不修改组织名称、成员关系、空间所有者或文件业务事实。

`00003_fix_namespace_subtype_trigger.sql` 修复了首版 Schema 共用触发器跨表引用不存在字段的问题。该缺陷在插入首个 Folder/Document 子类型时触发，模块 04 继承测试和模块 05 文件目录都依赖修复。

## 延期边界

- 模块 08 接入 Share 后复用同一最终判定入口，并通过已有 `share_version` 参与缓存版本；当前没有伪造共享允许结果。
- 模块 11 消费本模块 Outbox，形成不可篡改审计链、管理员特权访问和拒绝事件查询；本模块已经持久化授权变更意图。
- 模块 05 建立正式目录 API 后，文件操作 Handler 必须调用本模块应用服务，不得自行解释 ACL。

## 验证

```powershell
cd backend
go test ./internal/modules/permissions/...
./scripts/verify.ps1
./scripts/verify-integration.ps1
```

真实依赖测试覆盖默认拒绝、个人空间所有者、系统管理员特权确认、组织子树委派、递归委派、父撤销立即失效、直接/继承 ACL、断开继承、授权撤销以及 Redis 缓存版本变化立即失效。
