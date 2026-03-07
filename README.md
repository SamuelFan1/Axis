# Axis

Axis 是一个独立的控制平面系统，负责区域节点管理、健康状态采集、容量视图、调度支撑与后续控制面编排能力。

## 项目定位

Axis 只做控制平面，不承载网盘业务接口本身。

核心职责：

- node 注册与节点元数据管理
- heartbeat 心跳与健康状态上报
- region 区域管理与拓扑视图
- 容量快照与资源水位统计
- 上传调度与后续调度策略支撑
- 运维控制接口与内部管理 API

非目标：

- 不直接承担文件上传、下载、分享等业务流量
- 不在 `handler` 层写业务编排逻辑
- 不把节点控制逻辑继续堆进现有业务后端

## 设计原则

- 单一职责：Axis 只做控制平面。
- 分层清晰：入口层、应用层、领域层、持久化层、后台任务层必须分离。
- 领域优先：先定义 `node`、`region`、`heartbeat`、`scheduling` 等领域，再写接口。
- 面向演进：目录结构要支持未来增加 gRPC、OpenAPI、运维命令、调度策略、历史快照。
- 大厂式目录规范：严禁把所有代码塞进一个目录或一个包里。

## 标准项目目录规范

后续实现必须严格遵守以下目录结构。

```text
Axis/
  README.md
  go.mod
  go.sum

  cmd/
    axisd/
      main.go
    axis/
      main.go

  internal/
    app/
    bootstrap/
    config/
    domain/
      node/
      region/
      heartbeat/
      capacity/
      scheduling/
    service/
    repository/
    transport/
      http/
      grpc/
    worker/
    platform/

  pkg/
    logger/
    xhttp/
    xerror/
    xclock/

  api/
    openapi/
    proto/

  configs/
    local/
    dev/
    test/
    prod/

  deployments/
    docker/
    kubernetes/
    systemd/

  scripts/

  test/
    integration/
    e2e/
    fixtures/

  docs/
    architecture/
    adr/
    operations/
```

## 目录职责说明

### `cmd/`

放所有可执行程序入口。

- `cmd/axisd/`：Axis 服务主进程入口
- `cmd/axis/`：Axis 运维命令行工具入口

要求：

- `main.go` 只做启动装配
- 不允许在这里写业务规则
- 不允许在这里直接访问数据库细节

### `internal/`

放项目私有核心代码，禁止外部项目直接依赖。

#### `internal/app/`

应用装配层。

职责：

- 组织模块初始化
- 连接各层依赖
- 组装 server、worker、repository、service

#### `internal/bootstrap/`

启动与基础设施初始化层。

职责：

- 配置加载
- 日志初始化
- 数据库初始化
- 中间件初始化
- 生命周期管理

#### `internal/config/`

配置定义与解析层。

职责：

- 配置结构体
- 环境变量绑定
- 配置文件读取
- 默认值与配置校验

要求：

- 所有配置从这里统一进入
- 禁止在业务代码中随处 `os.Getenv`

#### `internal/domain/`

领域层，是整个项目最核心的目录。

建议按聚合拆分子目录：

- `node/`：节点实体、状态、上下线、标签、角色
- `region/`：区域、可用区、区域映射
- `heartbeat/`：心跳、健康事件、状态变迁
- `capacity/`：队列、磁盘、资源水位、容量快照
- `scheduling/`：调度策略、候选集、排序规则、调度结果

要求：

- 领域对象定义必须放这里
- 核心规则优先沉淀在领域层
- 不在这里依赖 HTTP、gRPC、ORM 细节

#### `internal/service/`

应用服务层，也可以理解为用例编排层。

职责：

- 编排多个领域对象与仓储接口
- 对外提供清晰 use case
- 负责事务边界与流程组织

适合放的内容：

- 注册节点
- 上报心跳
- 更新容量快照
- 查询区域可用节点
- 执行上传调度

#### `internal/repository/`

仓储层。

职责：

- 定义仓储接口
- 提供数据库实现
- 屏蔽底层持久化细节

要求：

- 查询语句、ORM 细节放这里
- 不允许在 `handler` 或 `service` 中直接堆 SQL

#### `internal/transport/`

传输层。

- `http/`：HTTP API、路由、DTO、middleware
- `grpc/`：后续需要时放 gRPC 服务与 proto 适配

要求：

- 这里只做协议转换和参数校验
- 不在这里写控制平面核心规则

#### `internal/worker/`

后台任务层。

职责：

- 周期巡检
- 过期数据清理
- 异步状态聚合
- 节点失联判定

#### `internal/platform/`

平台适配层。

职责：

- 对接第三方系统
- 云平台/宿主机能力封装
- 节点探针适配

适合放：

- 对接 node agent
- 对接监控系统
- 对接消息队列

### `pkg/`

放通用公共库，但必须是“通用能力”，不能放项目核心业务规则。

可以放：

- 日志封装
- HTTP 客户端封装
- 通用错误包装
- 时间与重试工具

不能放：

- 调度策略
- 节点健康判断规则
- 区域映射核心逻辑

### `api/`

接口契约目录。

- `openapi/`：HTTP 接口规范
- `proto/`：gRPC / protobuf 协议文件

要求：

- 所有对外接口协议优先在这里沉淀
- 避免接口定义散落在各处

### `configs/`

配置样例与环境配置目录。

- `local/`
- `dev/`
- `test/`
- `prod/`

要求：

- 示例配置与环境配置分离
- 不把敏感信息直接提交到仓库

### `deployments/`

部署目录。

- `docker/`
- `kubernetes/`
- `systemd/`

要求：

- 所有部署资产集中管理
- 不把部署脚本随意散落到根目录

### `scripts/`

只放辅助脚本。

例如：

- 本地启动
- 代码生成
- 测试辅助
- 打包发布

要求：

- 脚本必须幂等
- 脚本命名要直观

### `test/`

测试目录。

- `integration/`
- `e2e/`
- `fixtures/`

要求：

- 单元测试优先跟代码包放在一起
- 集成测试、端到端测试和测试数据统一放这里

### `docs/`

文档目录。

- `architecture/`：系统设计与架构图
- `adr/`：架构决策记录
- `operations/`：运维手册、巡检手册、故障处理手册

## 新增代码落点规则

后续开发必须遵守以下规则：

1. 新增实体模型时，先在 `internal/domain/` 下确定所属聚合。
2. 新增用例时，优先写在 `internal/service/`，不要直接堆到 `transport`。
3. 新增数据库访问逻辑时，统一落到 `internal/repository/`。
4. 新增 HTTP 接口时，路由、请求结构、响应结构放 `internal/transport/http/`。
5. 新增后台巡检、失联判定、过期处理逻辑时，统一落到 `internal/worker/`。
6. 新增通用工具时，先判断是否真的通用，再决定是否放到 `pkg/`。
7. 新增部署和运行配置时，只能放到 `configs/` 或 `deployments/`。
8. 新增架构说明与操作手册时，统一放到 `docs/`。

## 当前阶段要求

当前阶段先按上述规范建立项目骨架，后续所有实现必须基于这套目录执行，不允许再随意新增顶层杂目录。

当前目录状态：

- 已创建项目根目录
- 已定义标准目录规范
- 后续代码实现将严格按此 README 执行

## Quick Start

启动服务：

```bash
AXIS_DB_HOST=127.0.0.1 \
AXIS_DB_PORT=4000 \
AXIS_DB_USER=root \
AXIS_DB_PASSWORD=your_password \
AXIS_DB_NAME=AXIS \
AXIS_HTTP_ADDRESS=:9090 \
go run ./cmd/axisd
```

纳管服务器：

```bash
curl -X POST http://127.0.0.1:9090/api/v1/nodes/register \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "sgp-edge-01",
    "management_address": "10.8.1.11:9090",
    "status": "up",
    "region": "sgp"
  }'
```

说明：

- `uuid` 可选
- 如果未提供，管理端会自动生成 `uuid4`
- 同一 `management_address` 再次纳管时会复用已有 UUID

查看已纳管服务器：

```bash
./axis service-list
```

## License

Axis 使用 MIT License 发布。

这意味着：

- 可以商用
- 可以修改
- 可以私有部署
- 可以再分发

前提是保留原始版权与许可证声明。
