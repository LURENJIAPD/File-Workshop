# Goose Migration 使用说明

## 基线

`00001_initial_schema.sql` 是从已确认的 `database/File-Workshop-V1.0-PostgreSQL.sql` 机械转换而来的首版 Goose Migration。数据库表名、字段名、类型和约束仍以 `docs/File-Workshop-V1.0-数据库设计说明.md` 为唯一权威来源。

固定工具版本保存在 `backend/go.mod` 的 `tool` 块中：

```powershell
go tool goose -version
go tool goose -dir migrations validate
```

## 新空库

在 `backend/` 目录为 Goose 设置 PostgreSQL 驱动和目标连接串后执行：

```powershell
$env:GOOSE_DRIVER = 'postgres'
$env:GOOSE_DBSTRING = '<由部署环境安全注入的 PostgreSQL 连接串>'
go tool goose -dir migrations up
go tool goose -dir migrations status
```

不要把包含密码的连接串写入脚本、README、Git 或终端共享记录。

## 当前本地开发库

当前 `postgres/file_workshop` Schema 是此前通过 Navicat 直接导入的，不包含 Goose 版本历史。不要直接对该 Schema 执行首版 `up`，否则会因对象已经存在而失败。

现阶段应用可继续连接该 Schema。后续纳入 Goose 时应在以下方案中明确选择并验证：

1. 确认没有业务数据后，备份并重建本地开发数据库，再由 Goose 从空库执行；
2. 对现有 Schema 做完整结构比对后，受控登记首版基线版本。

未经用户明确同意，不自动删除或重建当前 Schema，也不直接修改 Goose 版本表。

## 集成验证

以下命令会创建名称受限制的临时数据库，执行 `up`、连接检查和 `down`，最后删除该精确临时数据库：

```powershell
.\scripts\verify-integration.ps1
```

首版 `down` 会删除整个 `file_workshop` Schema，只允许在临时数据库或明确可销毁的测试数据库执行。
