# File Workshop V1.0 Linux / SeaweedFS 环境启动清单

> 文档版本：V1.0  
> 文档状态：待环境执行  
> 最近更新：2026-08-14  
> 适用范围：Windows 本地阶段收敛后，搭建 Linux 测试服务器和 SeaweedFS S3 Gateway，并解锁模块 06 真实上传、下载、Range 与后续文件内容链路。

## 1. 目标

本清单的目标不是把 File Workshop 绑定到 SeaweedFS 内部实现，而是让项目拥有一个可验证的 S3 Compatible API 环境。

验收目标：

1. SeaweedFS S3 Gateway 可通过 S3 API 访问；
2. File Workshop 使用项目内 Object Storage Interface 和 AWS SDK for Go v2 连接对象存储；
3. 业务代码、数据库和 API 不出现 SeaweedFS Filer、Volume、Needle 等内部结构；
4. Bucket、凭据、预签名、Multipart、HEAD、GET、PUT、DELETE、Range 和错误映射可验证；
5. 通过后，再回到模块 06 上传完成、下载和 Range 开发。

## 2. 权威约束

本项目约束优先级：

1. 数据库结构仍以 `docs/File-Workshop-V1.0-数据库设计说明.md` 为唯一权威；
2. 对象存储只使用 S3 Compatible API；
3. 默认实现是 SeaweedFS S3 Gateway；
4. Go 端统一使用 AWS SDK for Go v2；
5. 业务层只依赖 `backend/internal/platform/objectstorage`，不得绕过适配器；
6. 对象 Key 由系统生成，不得直接使用用户文件名、业务路径或 SeaweedFS 内部路径；
7. 真实密钥不得写入仓库、文档、提交记录或日志。

## 3. 环境准备

建议至少准备一台 Linux 测试服务器。

| 项目 | 建议 |
|---|---|
| 操作系统 | Ubuntu Server / Debian / Rocky Linux 等稳定发行版 |
| 网络 | 后端服务器能访问 SeaweedFS S3 Gateway 地址 |
| 时间 | 开启 NTP，避免预签名 URL 因时间漂移失效 |
| 磁盘 | 为 SeaweedFS 数据目录单独规划挂载点 |
| 防火墙 | 仅开放必要端口；管理端口不要暴露到公网 |
| 凭据 | 使用测试专用 S3 Access Key / Secret Key |

端口规划建议：

| 服务 | 常见端口 | 暴露建议 |
|---|---:|---|
| SeaweedFS Master | 9333 | 仅内网/管理网络 |
| SeaweedFS Volume | 8080 | 仅内网/管理网络 |
| SeaweedFS Filer | 8888 | 仅内网/管理网络 |
| SeaweedFS S3 Gateway | 8333 | 仅后端和验证工具可访问 |
| File Workshop API | 8080 | 按部署架构暴露 |

端口可以按实际部署调整，但 `.env` 中的 `FILE_WORKSHOP_OBJECT_STORAGE_ENDPOINT` 必须指向 S3 Gateway，而不是 Filer、Master 或 Volume。

## 4. SeaweedFS 部署方式选择

可选路径：

| 方式 | 用途 | 说明 |
|---|---|---|
| 官方二进制 + systemd | 推荐用于长期测试环境 | 便于固定版本、配置数据目录、重启策略和日志 |
| 官方 Docker Compose | 适合快速验证 | Docker 只是 SeaweedFS 环境候选，不是 File Workshop 本地开发依赖 |
| 企业已有 S3 兼容服务 | 适合企业基础设施复用 | 必须通过本文第 7 章兼容性验证 |

不论采用哪种方式，最终交付给 File Workshop 的都只有：

- S3 Endpoint；
- Bucket；
- Access Key ID；
- Secret Access Key；
- Region；
- 是否使用 Path-style。

## 5. File Workshop 环境变量

后端当前读取以下对象存储配置：

```dotenv
FILE_WORKSHOP_OBJECT_STORAGE_ENABLED=true
FILE_WORKSHOP_OBJECT_STORAGE_PROVIDER=seaweedfs-s3
FILE_WORKSHOP_OBJECT_STORAGE_ENDPOINT=http://<seaweedfs-s3-host>:8333
FILE_WORKSHOP_OBJECT_STORAGE_REGION=us-east-1
FILE_WORKSHOP_OBJECT_STORAGE_BUCKET=file-workshop-dev
FILE_WORKSHOP_OBJECT_STORAGE_ACCESS_KEY_ID=<replace-with-test-access-key>
FILE_WORKSHOP_OBJECT_STORAGE_SECRET_ACCESS_KEY=<replace-with-test-secret-key>
FILE_WORKSHOP_OBJECT_STORAGE_FORCE_PATH_STYLE=true
FILE_WORKSHOP_OBJECT_STORAGE_PRESIGN_TTL=15m
FILE_WORKSHOP_OBJECT_STORAGE_HEALTH_TIMEOUT=3s
```

注意：

- 测试环境可以继续使用 `.env`，生产或准生产必须使用进程环境变量、Secret 文件、Kubernetes Secret、Vault 或企业密钥平台；
- `FILE_WORKSHOP_OBJECT_STORAGE_PROVIDER` 只用于业务元数据标识，不代表可以使用 SeaweedFS 私有字段；
- 若改用其他 S3 兼容服务，`PROVIDER` 应换成稳定标识，例如 `enterprise-s3`，并重新跑兼容性测试；
- `FORCE_PATH_STYLE=true` 是 S3 兼容服务常见默认值，只有目标服务明确支持虚拟主机风格并完成验证后才可改为 `false`。

## 6. Bucket 与凭据要求

建议创建独立测试 Bucket：

```text
file-workshop-dev
```

凭据权限至少需要覆盖：

- `HeadBucket`
- `CreateMultipartUpload`
- `UploadPart`
- `CompleteMultipartUpload`
- `AbortMultipartUpload`
- `PutObject`
- `GetObject`
- `HeadObject`
- `DeleteObject`

不建议授予：

- 全局管理员权限；
- 非必要 Bucket 管理权限；
- 跨 Bucket 访问权限；
- 长期暴露给浏览器的凭据。

浏览器只能拿到后端生成的短期预签名 URL，不得拿到 Access Key / Secret Key。

## 7. S3 兼容性验证清单

环境搭好后，先使用 AWS CLI、s3cmd、rclone 或等价 S3 客户端验证。

以下示例以 AWS CLI 形式表达，变量名仅用于本地 shell，不要提交到仓库：

```bash
export FW_S3_ENDPOINT="http://<seaweedfs-s3-host>:8333"
export AWS_ACCESS_KEY_ID="<replace-with-test-access-key>"
export AWS_SECRET_ACCESS_KEY="<replace-with-test-secret-key>"
export AWS_DEFAULT_REGION="us-east-1"
```

基础验证：

```bash
aws --endpoint-url "$FW_S3_ENDPOINT" s3api head-bucket --bucket file-workshop-dev
```

普通对象验证：

```bash
printf 'file-workshop-s3-smoke-test' > /tmp/fw-s3-smoke.txt
aws --endpoint-url "$FW_S3_ENDPOINT" s3 cp /tmp/fw-s3-smoke.txt s3://file-workshop-dev/smoke/fw-s3-smoke.txt
aws --endpoint-url "$FW_S3_ENDPOINT" s3api head-object --bucket file-workshop-dev --key smoke/fw-s3-smoke.txt
aws --endpoint-url "$FW_S3_ENDPOINT" s3 cp s3://file-workshop-dev/smoke/fw-s3-smoke.txt /tmp/fw-s3-smoke.out
cmp /tmp/fw-s3-smoke.txt /tmp/fw-s3-smoke.out
aws --endpoint-url "$FW_S3_ENDPOINT" s3 rm s3://file-workshop-dev/smoke/fw-s3-smoke.txt
```

Range 验证：

```bash
printf '0123456789abcdefghijklmnopqrstuvwxyz' > /tmp/fw-s3-range.txt
aws --endpoint-url "$FW_S3_ENDPOINT" s3 cp /tmp/fw-s3-range.txt s3://file-workshop-dev/smoke/fw-s3-range.txt
aws --endpoint-url "$FW_S3_ENDPOINT" s3api get-object \
  --bucket file-workshop-dev \
  --key smoke/fw-s3-range.txt \
  --range bytes=0-9 \
  /tmp/fw-s3-range.part
cat /tmp/fw-s3-range.part
aws --endpoint-url "$FW_S3_ENDPOINT" s3 rm s3://file-workshop-dev/smoke/fw-s3-range.txt
```

Multipart 验证至少要覆盖：

1. 创建 Multipart Upload；
2. 上传至少 2 个 part；
3. 完成 Multipart Upload；
4. `HeadObject` 验证大小和 ETag 行为；
5. 主动 Abort 未完成的 Multipart Upload；
6. 对不存在 Upload ID、错误 Part Number、重复完成进行错误码记录。

可以先用 AWS CLI 或一个临时验证工具完成；不要把临时脚本中的密钥提交到仓库。

## 8. 后端健康检查验证

更新后端 `.env` 后，在 `backend` 目录启动服务：

```powershell
go run ./cmd/server
```

另开终端检查健康接口：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health/live
Invoke-RestMethod http://127.0.0.1:8080/health/ready
```

期望：

- PostgreSQL 健康；
- Redis 健康；
- `objectStorage` 已启用；
- `objectStorage` 检查成功；
- 如果 Bucket 或凭据错误，健康检查应失败或显示对象存储不可用，不得在业务上传接口中伪造成功。

`cmd/dbcheck` 目前只检查 PostgreSQL，不代表对象存储已可用：

```powershell
go run ./cmd/dbcheck
```

## 9. 解锁模块 06 的最小通过条件

只有满足以下条件，才开始实现真实上传完成、下载和 Range：

- [ ] S3 Gateway Endpoint 可从后端机器访问；
- [ ] Bucket 已创建；
- [ ] 测试凭据可 `HeadBucket`；
- [ ] 普通 Put/Get/Head/Delete 通过；
- [ ] Range GET 通过；
- [ ] Multipart 创建、上传分片、完成、取消通过；
- [ ] 预签名 PUT/GET 在浏览器或等价 HTTP 客户端中通过；
- [ ] 错误码行为已记录，包括 Bucket 不存在、凭据错误、对象不存在、Upload ID 无效；
- [ ] 后端 `/health/ready` 能报告 `objectStorage` 成功；
- [ ] `.env` 或部署 Secret 已配置，真实密钥未进入 Git。

## 10. 回到开发的第一批任务

环境通过后，按以下顺序进入开发：

1. 模块 06：实现 `upload_parts` 登记；
2. 模块 06：实现上传完成提交，校验 Part Number、ETag、大小、Hash 和幂等；
3. 模块 06：写入 `storage_objects` 和 `document_versions`，更新 Document 当前版本；
4. 模块 06：实现下载凭证和 Range 下载契约；
5. 模块 07：把文件锁 `fencingToken` 接入上传完成写入；
6. 模块 09：实现 `LIFECYCLE_PURGE` 处理器的真实对象删除和最终 `PURGED` 收敛；
7. 模块 10/12：接入 `INDEX` 和预览处理器；
8. 最后再进入 WebDAV、SMB 迁移和 AI/Agent。

## 11. 参考来源

- SeaweedFS 官方仓库：<https://github.com/seaweedfs/seaweedfs>
- SeaweedFS S3 API 文档：<https://github.com/seaweedfs/seaweedfs/wiki/Amazon-S3-API>
- AWS SDK for Go v2 S3 示例：<https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/go_s3_code_examples.html>
- 项目 ADR：`docs/adr/ADR-021-对象存储默认实现调整为SeaweedFS.md`
- 项目调研：`docs/research/M06-文件传输与存储适配基线调研.md`
