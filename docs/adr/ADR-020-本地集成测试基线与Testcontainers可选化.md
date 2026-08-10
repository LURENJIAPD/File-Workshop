# ADR-020：本地集成测试基线与 Testcontainers 可选化

> **状态：** 已接受  
> **日期：** 2026-08-05  
> **决策范围：** 后端集成测试依赖、数据隔离、Docker/Testcontainers 使用边界

## 背景

原技术选型将 Go Test + Testcontainers 冻结为后端测试方案，但当前开发环境已经部署并验证 PostgreSQL 18.4 和 Redis 6.2.6，Docker Desktop 引擎并非项目现阶段的准备条件。强制引入 Testcontainers 会增加 Docker、镜像下载和容器运行依赖，并不能为当前基础模块提供与成本相称的额外验证价值。

Go Test 是 Go 官方测试入口，必须保留；Testcontainers 是借助 Docker 启动临时真实依赖的可选隔离工具，不是 Go Test、后端应用或真实数据库测试的必要组成部分。

## 决策

1. 后端冻结测试基线调整为 Go Test、`httptest` 和真实本地依赖集成测试。
2. PostgreSQL 集成测试只使用随机命名的临时数据库，从空库执行全部 Goose Migration，测试结束后仅按严格名称规则精确删除该临时数据库。
3. Redis 集成测试使用带随机 UUID 的唯一键和短 TTL，测试结束后精确删除该键；禁止执行 `FLUSHALL`、`FLUSHDB` 或清理未知键。
4. Mock/Fake 只用于纯业务规则、错误分支和外部边界单元测试，不代替 PostgreSQL 约束、事务、SQL 和 Redis 协议的真实验证。
5. 禁止使用 SQLite 代替 PostgreSQL 集成测试，避免类型、约束、事务、JSONB、索引和 SQL 方言差异产生错误结论。
6. Testcontainers 改为具备 Docker/CI 环境后的可选方案。未来启用时必须固定版本、验证许可证和供应链风险，并保持与本 ADR 相同的数据隔离和清理边界。
7. SeaweedFS/S3、OpenSearch、ClamAV 等后续依赖启用时，另行选择专用测试实例或容器隔离方案，不由本 ADR 提前强制 Docker。

## 结果

正向影响：

- 当前开发不依赖 Docker，可直接复用已经搭建并通过连通验证的 PostgreSQL 和 Redis；
- 测试仍执行真实数据库 Migration、连接池、Redis 客户端和 HTTP 服务路径；
- 随机数据库名、唯一键、短 TTL 和精确清理降低污染现有开发数据的风险；
- CI 环境仍保留采用 Testcontainers 的升级路径。

代价与风险：

- 本地 PostgreSQL 和 Redis 必须处于可用状态才能运行集成测试；
- 不同开发者的本地服务配置可能存在差异，需要通过 `.env` 和统一版本基线控制；
- 当前 Redis 共用服务只能提供键级隔离，未来涉及事务、过期或全局配置的测试应使用专用测试实例。

## 被否决方案

- 当前立即强制 Testcontainers：Docker 引擎未作为开发前提，增加环境复杂度。
- 只使用 Mock/Fake：无法验证 Migration、数据库约束、SQL、连接池和真实 Redis 协议。
- 使用 SQLite 替代 PostgreSQL：与权威 PostgreSQL Schema 的行为差异过大。
- 直接复用当前业务 Schema 写入测试数据：可能污染开发数据且难以保证清理完整性。

## 实施与验证

- `backend/scripts/verify.ps1` 执行生成、格式、依赖、静态检查、单元测试、构建和漏洞扫描；
- `backend/scripts/verify-integration.ps1` 使用本地真实 PostgreSQL/Redis；
- Migration 测试执行临时数据库 `up`、Schema 校验、`down` 和精确删除；
- Redis 测试执行唯一键写入、读取和精确删除；
- 后续增加业务模块时，继续按数据库设计说明建立隔离测试数据，不得降低清理安全边界。
