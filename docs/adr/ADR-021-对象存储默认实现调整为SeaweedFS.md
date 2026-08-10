# ADR-021 对象存储默认实现调整为 SeaweedFS

日期：2026-08-09

状态：接受

关联变更：ACN-001

## 背景

File Workshop V1.0 原技术文档将对象存储描述为 MinIO，同时要求业务代码只依赖标准 S3 兼容语义。随着架构评审推进，项目需要进一步避免绑定具体对象存储产品，把协议、SDK、业务接口和默认实现分清楚。

本次变更不调整数据库模型、权限体系、审计体系、AI 架构和文件领域模型。

## 决策

1. 对象存储协议冻结为 S3 Compatible API。
2. Lite 默认对象存储实现调整为 SeaweedFS，并通过 SeaweedFS S3 Gateway 接入。
3. Go 端统一使用 AWS SDK for Go v2 访问标准 S3 API。
4. 业务代码只能依赖项目内 Object Storage Interface，不直接依赖 SeaweedFS、MinIO 或其他对象存储产品的内部实现。
5. 文件服务、后台任务、WebDAV、预览、搜索和 AI 等入口访问文件二进制时，都必须经过统一存储适配层和统一权限语义。

## 约束

- 禁止业务代码使用 SeaweedFS Filer、Volume、Master、Needle、Collection、TTL、Replication 等内部概念作为业务事实。
- 禁止使用存储产品专属数据结构污染数据库设计、API 契约或领域对象。
- 禁止拼接对象存储产品私有 URL 作为长期业务状态。
- 预签名 URL 只能作为短期数据面凭证，由应用服务在完成权限、配额、会话和审计判断后生成。
- Bucket、Object Key、Multipart Upload、Range 下载、HEAD、COPY、DELETE 和错误映射必须由适配层封装。

## 影响

- 技术选型说明和主设计文档中的 MinIO 唯一实现表述改为 S3 Compatible API + SeaweedFS 默认实现。
- 开发计划中模块 06 的前置条件改为 SeaweedFS S3 Gateway 与 AWS SDK for Go v2 适配器基线。
- 健康检查对外组件名由 `minio` 调整为 `objectStorage`，避免把旧默认实现固化到接口语义中。
- 数据库不新增、不删除、不重命名任何表或字段；现有 `storage_objects`、`upload_sessions`、`upload_parts` 等设计仍然有效。

## 兼容与替换路径

如企业已有对象存储，可替换为其他经兼容性验证的 S3 服务。替换前至少验证：

- 分片上传创建、签名、完成和终止；
- 预签名上传与下载；
- Range 下载；
- 对象元数据读取；
- 对象复制与删除；
- 常见 S3 错误码映射；
- 凭据最小权限；
- 对象访问日志或可观测性接入。

## 后续

模块 06 文件传输与存储开始前，需要完成 SeaweedFS S3 Gateway 本地或测试环境搭建，并冻结 `.env` 配置项、适配器接口、兼容性测试和故障降级语义。
