# File Workshop 后端

后端使用 Go 1.26.5、Gin、PostgreSQL、原生 `pgxpool`、sqlc、Goose 和 go-redis，所有环境变量统一使用 `FILE_WORKSHOP_` 前缀。SeaweedFS/S3 尚未搭建，当前明确暂缓。

## 本地配置

本地实际配置保存在 `.env`，该文件已被根目录 `.gitignore` 排除；可提交模板为 `.env.example`。进程环境变量优先于 `.env`，也可通过 `FILE_WORKSHOP_ENV_FILE` 指定其他配置文件。

从 `backend/` 目录验证数据库连接：

```powershell
go run ./cmd/dbcheck
```

启动 HTTP 服务：

```powershell
go run ./cmd/server
```

GoLand 运行配置可选择 `backend/cmd/server/main.go`，Working directory 必须设置为 `backend/`。当前基础接口：

- `GET /health/live`：只检查进程存活；
- `GET /health/ready`：PostgreSQL 必需、Redis 可降级、对象存储显示 `disabled`。

连接池默认最大 10 个连接、最小 1 个连接，设置连接寿命、空闲回收、健康检查、连接与 Ping 超时，并为每条连接固定 `search_path=file_workshop,public`、UTC 时区和查询/锁/空闲事务超时。

Redis 使用官方 `go-redis` 客户端和内置连接池。为兼容当前 Redis 6.2.6，客户端固定使用 RESP2 并关闭旧服务端不支持的客户端身份上报；真实本地集成测试必须保持通过。

## 代码生成与验证

OpenAPI、sqlc 和 Goose 工具版本固定在 `go.mod` 的 `tool` 块中。生成和全量基础验证：

```powershell
.\scripts\generate.ps1
.\scripts\verify.ps1
.\scripts\verify-integration.ps1
.\scripts\vulnerability-check.ps1
```

`verify-integration.ps1` 直接使用 `.env` 指向的本地 PostgreSQL 和 Redis，不依赖 Docker。PostgreSQL Migration 测试只创建并精确删除名称带随机 UUID 的临时数据库，不修改当前 `postgres` 数据库中的 `file_workshop` Schema；Redis 测试只写入带随机 UUID、有效期一分钟的单个测试键，并在结束时精确删除，禁止使用 `FLUSHALL` 或 `FLUSHDB`。

- `api/openapi.yaml` 是当前 REST 工作契约；`api/api.gen.go` 由 oapi-codegen 生成，禁止手工修改；
- `sqlc.yaml` 和 `internal/platform/database/queries/` 生成 `internal/platform/database/dbgen/`，生成物禁止手工修改；
- `migrations/` 由 Goose 管理，应用启动不会自动执行 Migration；
- 当前 Navicat 手工导入的开发 Schema 尚未登记 Goose 版本，处理方式见 `migrations/README.md`。

## 依赖取舍

- 采用 `github.com/jackc/pgx/v5/pgxpool`：MIT、持续维护，与项目冻结的 pgx/sqlc 技术路线一致。
- 采用官方 `github.com/redis/go-redis/v9`：BSD-2-Clause，复用连接池和协议实现，不自行实现 Redis 客户端。
- 采用 Gin、Goose、sqlc 和 oapi-codegen 的固定版本，拒绝引入与 PostgreSQL/pgx/OpenAPI-first 冲突的整套 GORM 脚手架。
- 不采用 GORM：本项目不需要 ORM 自动建模或隐式迁移。
- 不叠加 `database/sql` 连接池：避免同时维护两层池语义。
- `.env` 仅使用轻量的 `github.com/joho/godotenv` 在开发环境加载；生产环境继续使用进程环境变量或 Secret，不依赖 `.env`。
