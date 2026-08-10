# 模块 16：后台任务

本模块对应开发设计文档第 6.17 节。当前周期完成 Outbox Worker 最小可用框架，用于后续审计、索引、预览、生命周期、AI 等异步能力接入。

## 已实现能力

- `cmd/worker` 独立启动入口。
- 处理器注册模型：Worker 只领取已注册处理器支持的 `event_type`，未注册事件不会被空消费。
- PostgreSQL Outbox 领取：按状态、到期时间、租约和 `FOR UPDATE SKIP LOCKED` 并发安全领取。
- 成功事件标记为 `PUBLISHED`。
- 可重试错误标记为 `FAILED`，写入 `next_retry_at/last_error_code/last_error_summary`。
- 永久失败或重试耗尽标记为 `DEAD`。
- 使用 `context.Context`、处理器超时和系统信号完成优雅关闭。
- 提供按状态统计的 Repository 查询，为后续运维 API 预留。

## 数据库边界

字段严格来自数据库设计：`outbox_events`、`background_jobs`。当前只消费 `outbox_events`，不创建或执行具体 `background_jobs`。Redis 不参与任务事实存储。

## 启动方式

```powershell
cd backend
go run ./cmd/worker
```

如果暂未注册业务处理器，Worker 会启动并保持空闲，不会领取任何 Outbox 事件。

## 延期边界

- `background_jobs` 的统一调度、查询、取消和重放接口；
- 审计 Outbox 消费器；
- 文件 Hash、病毒扫描、预览、索引、生命周期和垃圾回收处理器；
- 运维 API、指标、告警和死信人工处理流程。

## 验证

```powershell
cd backend
go test ./internal/modules/background/...
$env:FILE_WORKSHOP_RUN_INTEGRATION='1'; go test ./tests/integration -run TestBackgroundWorkerOutboxLifecycle -count=1
```
