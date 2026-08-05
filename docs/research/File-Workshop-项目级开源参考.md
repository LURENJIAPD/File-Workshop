# File Workshop 项目级开源参考

> 首次整理：2026-08-05
>
> 适用范围：后端、前端、文件协议、空间、共享、搜索、预览、审计和部署等后续模块
>
> 维护方式：仅在候选项目、许可证、维护状态或本项目选型发生实质变化时更新，不要求每个常规模块重复调研

## 1. 目标与使用规则

本清单用于持续参考成熟文件管理与文档管理系统，减少重复检索和重复造轮子。开发模块时优先从本清单定位可借鉴的架构、交互、协议和测试思路；只有高复杂度、高风险或本清单无法覆盖的能力才开展专项调研。

本项目数据库设计、权限边界、审计语义、分页契约和冻结技术栈仍以仓库文档为准。候选项目的源码不能因“可参考”而直接复制；任何直接引入必须再次核对具体目录和文件的许可证、NOTICE、依赖及安全风险。

## 2. 长期参考项目

| 项目 | 仓库与许可证 | 维护状态（整理时） | 主要参考价值 | 不直接采用的原因 |
|---|---|---|---|---|
| OpenCloud | <https://github.com/opencloud-eu/opencloud>，服务端 Apache-2.0；Web 前端需单独核对 AGPL-3.0 | 持续发布和维护 | Go 服务端、Vue/TypeScript 前端、Spaces、WebDAV、Graph/OCS API、同步与共享 | 微服务和文件系统事实源与本项目“模块化单体 + PostgreSQL + S3”不同 |
| ownCloud Infinite Scale（oCIS） | <https://github.com/owncloud/ocis>，源码 Apache-2.0；官方稳定构建可能附加 EULA | 持续维护 | Spaces、统一网关、WebDAV/CS3、OIDC、可伸缩文件协作和端到端测试组织 | 多服务架构和 Reva 体系引入成本过高，不作为直接依赖 |
| Nextcloud Server | <https://github.com/nextcloud/server>，AGPL-3.0-or-later，部分第三方目录许可证另列 | 长期活跃并持续发布 | 成熟文件管理交互、共享、版本、回收站、WebDAV 和扩展生态 | PHP 技术栈与 AGPL 约束不适合复制到当前 Go 后端，仅参考产品行为和兼容测试 |
| Pydio Cells | <https://github.com/pydio/cells>，AGPL-3.0 | 持续维护，主分支可能包含下一代开发内容 | Go 文件协作系统、任务调度、网关、身份与数据服务拆分 | 微服务复杂度较高，且 AGPL 不适合直接复制到当前仓库 |
| Paperless-ngx | <https://github.com/paperless-ngx/paperless-ngx>，GPL-3.0 | 社区持续发布 | 文档采集、OCR、元数据、标签、全文检索、异步工作流和管理界面 | 产品重点是扫描归档而非企业文件协作，Python/Django 技术栈不同 |
| Alfresco Community Repository | <https://github.com/Alfresco/alfresco-community-repo>，LGPL-3.0 | 成熟项目，按发布分支维护 | 企业内容模型、版本、保留、审计、搜索和 REST/CMIS 边界 | Java 平台体量和扩展模型远超 V1.0，需要提炼设计而非引入平台 |

## 3. 当前采用结论

1. 以 OpenCloud/oCIS 作为最接近 File Workshop 技术方向的主要架构和产品参考，重点学习 Space、网关、协议适配、同步和端到端测试，不复制其微服务拆分。
2. 以 Nextcloud 作为成熟文件操作、共享、版本、回收站和 WebDAV 兼容行为参考。
3. 以 Paperless-ngx 和 Alfresco 补充文档提取、检索、保留、企业元数据和审计参考。
4. Pydio Cells 主要用于评估 Go 文件平台的模块边界、后台任务和服务治理成本。
5. 当前不直接引入上述完整系统或源码，不新增许可证传播义务；继续复用项目已冻结的 Gin、pgx、sqlc、Goose、OpenAPI 生成链和前端模板。

## 4. 对模块 03 的具体启发

- Space 必须拥有稳定 ID，组织移动不能改变 Space 或后续 Document 的身份。
- 组织成员关系与文件访问授权分离；模块 03 只维护组织、成员和空间事实，模块 04 才是授权判定入口。
- 组织树和成员变化必须有可失效的安全版本，避免缓存和搜索投影长期返回过期结果。
- 个人、组织和公共空间使用同一资源模型，通过所有者条件约束区分，不为每类空间建立重复实现。
- 配额必须在数据库事务中原子预留、消费或释放；上传和对象存储流程只能调用该能力，不能自行修改计数。

## 5. 对模块 04 的专项轻量评估

权限是全系统高风险安全边界，因此在项目级参考之外补充比较了四类成熟方案：

| 方案 | 许可证与能力 | 可借鉴内容 | 本期结论 |
|---|---|---|---|
| [Apache Casbin](https://github.com/apache/casbin) | Apache-2.0；支持 ACL、RBAC、ABAC、ReBAC 等模型 | `subject-object-action` 判定模型、默认拒绝、角色/资源层级思想 | 不引入运行时依赖；本项目已存在更具体的委派、ACL、继承和版本表，再建 Casbin Policy 存储会形成双事实源 |
| [OpenFGA](https://github.com/openfga/openfga) | Apache-2.0；面向关系型细粒度授权 | 从资源出发建模关系、层级继承和批量 Check 思路 | 不新增独立授权服务；当前 PostgreSQL 事务必须同步写授权、版本和 Outbox，远程关系库会增加双写一致性成本 |
| [Open Policy Agent](https://github.com/open-policy-agent/opa) | Apache-2.0；通用声明式策略引擎和 Rego | 策略与协议适配分离、结构化输入、显式决策结果 | V1.0 规则由数据库结构和设计文档冻结，暂不引入第二种策略语言与发布链 |
| [SpiceDB](https://github.com/authzed/spicedb) | Apache-2.0；Zanzibar 风格关系授权数据库 | 关系图、递归权限、版本一致性和最终 Check 入口 | 能力成熟但部署、运维和数据迁移成本超出模块化单体 V1.0；保留为未来超大规模授权演进候选 |

本期吸收共同优势：使用“主体—资源—动作”统一判定；关系和资源层级只由服务端解析；直接与继承权限取并集；默认拒绝；批量过滤后仍执行最终 Check；变更通过安全版本使缓存失效。实现继续以 `admin_delegations`、`permission_grants`、组织闭包和 Security Version 为 PostgreSQL 唯一事实源，没有复制上述项目源码，也没有新增许可证传播义务。

## 6. 后续维护

- 开发复杂模块时先记录“参考本清单中的哪些项目和能力”，不再为普通 CRUD 重复建立候选表。
- 准备直接引入依赖、复制代码、采用协议实现或改变架构前，新增专项记录并固定上游版本或提交。
- 维护状态和许可证结论不是永久事实；正式引入前必须重新核验上游仓库。
