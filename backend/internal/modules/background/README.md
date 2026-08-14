# 模块 16：后台任务

本模块对应开发设计文档第 6.17 节，负责可靠异步处理、重试、死信和后台运维控制。当前已完成基础调度周期，不接真实对象存储任务，也不注册审计、索引、预览、病毒扫描、生命周期、AI 等具体业务处理器。

## 已实现能力

- `cmd/worker` 独立启动入口。
- Outbox Runner：按已注册 `eventType` 领取 `outbox_events`，未注册事件不会被空消费。
- Job Runner：按已注册 `jobType` 领取 `background_jobs`，未注册任务不会被空消费。
- PostgreSQL 领取使用状态、到期时间、租约和 `FOR UPDATE SKIP LOCKED`，支持并发安全认领。
- 成功事件标记为 `PUBLISHED`；成功任务标记为 `SUCCESS`。
- 可重试错误标记为 `FAILED`，永久失败或重试耗尽标记为 `DEAD`。
- 支持失败摘要、错误码、退避重试、租约续期、心跳和 `rowVersion` 乐观并发。
- 管理员 REST API 可分页查询 Outbox/Job 积压与失败原因，并对 `FAILED/DEAD` Outbox/Job 执行单项或批量受控重试。
- 管理员 REST API 可按 `rowVersion` 受控取消 `PENDING/FAILED/DEAD` 后台任务，取消后任务进入 `CANCELLED` 终态。
- 管理员 REST API 可查询 Outbox/Job 按状态聚合的积压统计。
- 管理员 REST API 可查询 Outbox/Job 已到期待处理、已到期失败重试和过期处理租约摘要。
- 管理员 REST API 可查询 Outbox/Job `FAILED/DEAD` 状态下 Top 20 失败原因聚合，辅助定位主要故障类型。
- 管理员 REST API 可主动恢复过期 `PROCESSING` 租约：未耗尽次数的项目收敛为 `FAILED` 并立即可重试，已耗尽次数的项目收敛为 `DEAD`。
- 管理员 REST API 可对最多 50 个后台任务执行批量重试、批量取消、批量死信或批量跳过，单项失败以明细返回。
- 使用 `context.Context`、处理器超时和系统信号完成优雅关闭。

## 数据库边界

字段严格来自数据库设计：`outbox_events`、`background_jobs`。Redis 不参与任务事实存储。

本模块不新增数据库字段，不绑定对象存储实现，不直接消费 SeaweedFS/S3。

## 启动方式

```powershell
cd backend
go run ./cmd/worker
```

如果暂未注册业务处理器，Worker 会启动并保持空闲，不会领取任何 Outbox 事件或后台任务。

## 管理员接口

- `GET /api/v1/admin/background/outbox-events?page=1&pageSize=50`
- `POST /api/v1/admin/background/outbox-events/{outboxEventId}/retry`
- `POST /api/v1/admin/background/outbox-events/batch-retry`
- `GET /api/v1/admin/background/summary`
- `GET /api/v1/admin/background/queue-lag`
- `GET /api/v1/admin/background/failure-summary`
- `POST /api/v1/admin/background/expired-leases/recover`
- `GET /api/v1/admin/background/jobs?page=1&pageSize=50`
- `POST /api/v1/admin/background/jobs/{backgroundJobId}/retry`
- `POST /api/v1/admin/background/jobs/{backgroundJobId}/cancel`
- `POST /api/v1/admin/background/jobs/batch-retry`
- `POST /api/v1/admin/background/jobs/batch-cancel`
- `POST /api/v1/admin/background/jobs/batch-dead-letter`
- `POST /api/v1/admin/background/jobs/batch-skip`

所有接口仅允许 `SYSTEM_ADMIN` 访问；分页统一使用 `page/pageSize`；重试、取消、死信和跳过必须携带 `rowVersion` 和 `reason`；过期租约恢复必须携带 `reason`。

## 延期边界

- 审计 Outbox 消费器；
- 文件 Hash、病毒扫描、预览、索引、生命周期和垃圾回收处理器；
- 指标、告警和死信人工处理流程；
- 崩溃恢复压力测试和长时间稳定性测试。

## 验证

```powershell
cd backend
go test ./internal/modules/background/...
$env:FILE_WORKSHOP_RUN_INTEGRATION='1'; go test ./tests/integration -run TestBackgroundWorkerOutboxLifecycle -count=1
$env:FILE_WORKSHOP_RUN_INTEGRATION='1'; go test ./tests/integration -run TestBackgroundAdministrationHTTPWorkflow -count=1 -v
```
