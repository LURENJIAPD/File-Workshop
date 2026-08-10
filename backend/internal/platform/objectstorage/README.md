# Object Storage Platform Adapter

本包是 File Workshop 后端访问对象存储的唯一平台入口。

规则：

- 业务模块只能依赖 `Client` 接口；
- 只有本包可以直接引用 AWS SDK for Go v2；
- 不得在业务模块中使用 SeaweedFS Filer、Volume、Needle、Collection 等私有概念；
- 当前本地未搭建对象存储时使用 `DisabledClient`，不能伪造上传成功；
- 启用真实对象存储后，必须通过 S3 兼容性测试验证 Multipart、预签名、Range、对象元数据和错误映射。

默认实现：

- 协议：S3 Compatible API；
- SDK：AWS SDK for Go v2；
- 默认产品：SeaweedFS S3 Gateway。
