# 审计模块

当前周期实现基础审计闭环：

- 从已注册 Outbox 事件生成 `audit_events`；
- 高风险事件写入 SHA-256 Hash Chain，并维护 `audit_chain_heads`；
- 提供管理员审计事件分页查询、详情查询、完整性状态查询和链校验接口；
- 仅依赖数据库设计中已存在的 `audit_events`、`audit_chain_heads` 和 `outbox_events` 字段。

暂未实现：

- 审计导出、归档、WORM、批次锚定和安全告警；
- 面向组织管理员的资源级审计视图；
- 对未来模块事件类型的完整映射。

这些能力将在对象存储和后续业务模块准备好后继续补充。
