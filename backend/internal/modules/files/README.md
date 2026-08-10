# 模块 05：文件目录

本模块对应开发设计文档第 6.6 节，负责文件夹树、Document 稳定身份和统一目录命名空间。当前实现的是不依赖对象存储集群的目录事实层。

## 已实现能力

- Space 根目录懒创建：空间创建后不强制创建 root，首次创建文件夹或 Document 占位时在同一事务中初始化。
- `namespace_entries` 统一承载 Folder/Document 公共身份、名称、父目录、路径缓存、深度、生命周期和乐观锁版本。
- `folders`、`documents` 通过共享主键保存类型专属字段，严格映射数据库设计，不新增表或字段。
- 文件夹创建、Document 占位创建、目录列表、详情、重命名和移动。
- Document 占位默认 `availabilityStatus=BLOCKED`，`currentVersionId` 为空；真实二进制、版本和对象 Key 等待模块 06/07。
- 同目录名称归一化唯一、根文件夹不可移动或重命名、文件夹移动防环、移动后刷新后代路径缓存。
- 所有读写入口强制调用模块 04 权限服务，移动成功后递增 `spaces.security_epoch`，确保路径继承变化后权限缓存失效。
- 幂等记录和 Outbox 与目录事实在同一 PostgreSQL 事务提交。

## 数据库边界

字段严格来自数据库设计：`spaces.root_folder_id/security_epoch`、`namespace_entries`、`folders`、`documents`、`idempotency_records`、`outbox_events`。本模块不写 `document_versions`、`storage_objects`、扫描、预览、搜索或回收表。

## 延期边界

- 模块 06 接入 S3 Compatible API + SeaweedFS 后，负责上传、下载、对象 Key、预签名 URL 和对象存储适配。
- 模块 07 接入版本与并发后，负责 `document_versions`、当前版本指针、锁和内容级乐观并发。
- 模块 09/10/12/16 分别接入生命周期、搜索、预览和后台任务，不在本模块伪造异步处理结果。

## 验证

```powershell
cd backend
go test ./internal/modules/files/...
go test ./...
$env:FILE_WORKSHOP_RUN_INTEGRATION='1'; go test ./tests/integration -run TestFilesHTTPDirectoryLifecycle -count=1
```

真实依赖测试覆盖公共空间、中文目录、根目录懒创建、Document 占位、分页列表、重命名、移动和循环移动拒绝。
