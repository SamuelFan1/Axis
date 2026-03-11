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

先创建 `.env`：

```bash
cp .env.example .env
```

然后按你的 TiDB 环境修改 `.env` 中的数据库连接参数。

同时在 `.env` 中设置管理员鉴权与 node 注册共享密钥：

- `AXIS_ADMIN_USERNAME`
- `AXIS_ADMIN_PASSWORD`
- `AXIS_NODE_SHARED_TOKEN`
- `AXIS_NODE_TIMEOUT_SEC`
- `AXIS_NODE_MONITOR_INTERVAL_SEC`

如需启用可选 DNS 自动化，还需要配置：

- `AXIS_DNS_ENABLED=true`
- `AXIS_DNS_PROVIDER=cloudflare`
- `AXIS_DNS_ZONE=github.com`
- `AXIS_DNS_RECORD_PREFIX=dl-`
- `AXIS_DNS_RECORD_TYPE=A`
- `AXIS_DNS_TTL=1`
- `AXIS_DNS_PROXIED=false`
- `AXIS_DNS_CLOUDFLARE_API_TOKEN`

区域与可用区配置（参考 ISO-3166-1 alpha-2）：

- `AXIS_REGIONS`：大洲列表，默认 `asia,europe,australia,north_america,south_america`
- `AXIS_LOCAL_REGION`：当前这台 `axisd` 所属区域，只用于本地超时下线监控
- `AXIS_REGION_ASIA_ZONES`、`AXIS_REGION_EUROPE_ZONES` 等：各大洲允许的 zone（国家代码），逗号分隔
- 建议只在权威区域先创建标准 `regions` 与 `zones`，再通过 `AXIS` 同步扩散到其他区域
- 不在 `AXIS_REGIONS` 或对应 `AXIS_REGION_*_ZONES` 中的值不应被创建为主数据

启动服务：

```bash
go run ./cmd/axisd
```

推荐生产环境使用 systemd：

```bash
# 构建并部署
cd /apps/Axis
go build -o axisd ./cmd/axisd
go build -o axis ./cmd/axis
sudo cp axisd axis /usr/local/bin/

# 安装 systemd 单元并启动
sudo cp deployments/systemd/axisd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now axisd.service
```

然后创建管理员 shell 环境文件：

```bash
cp axis-rc.sh.example axis-rc.sh
```

按你的环境修改 `axis-rc.sh` 中的管理端地址、管理员账号和密码，然后执行：

```bash
source /apps/Axis/axis-rc.sh
```

纳管服务器：

```bash
curl -X POST http://127.0.0.1:9090/api/v1/nodes/register \
  -H "Content-Type: application/json" \
  -H "X-Axis-Node-Token: your_node_shared_token" \
  -d '{
    "hostname": "sgp-edge-01",
    "management_address": "10.8.1.11:9090",
    "status": "up",
    "region": "asia",
    "zone": "SG"
  }'
```

说明：

- `uuid` 可选
- `region` 为大洲：asia、europe、australia、north_america、south_america
- `zone` 为可用区，ISO-3166-1 alpha-2 国家代码（如 SG、CN、US）
- 只有已在配置中声明的 `region/zone` 组合才能纳管或上报
- 如果未提供，管理端会自动生成 `uuid4`
- 同一 `management_address` 再次纳管时会复用已有 UUID

查看已纳管服务器：

```bash
axis service-list
```

说明：

- `axisd` 会优先从项目根目录 `.env` 读取数据库连接配置
- `axis` CLI 不再直连 TiDB
- `axis` CLI 只能通过管理端 HTTP API 工作
- 使用 `axis` 前必须先 `source /apps/Axis/axis-rc.sh`
- 未加载管理环境时，`axis` 会直接拒绝执行
- `GET /api/v1/nodes` 为管理员接口，使用 HTTP Basic Auth
- `POST /api/v1/nodes/register` 为 node 接口，使用 `X-Axis-Node-Token`
- `POST /api/v1/admin/nodes/register` 为管理员显式纳管接口，使用 HTTP Basic Auth
- `POST /api/v1/nodes/report` 为 node 指标上报接口，使用 `X-Axis-Node-Token`
- `GET /api/v1/nodes/assign` 为管理员分配接口，使用 HTTP Basic Auth
- 如果超过 `AXIS_NODE_TIMEOUT_SEC` 秒未收到上报，控制端会自动把节点状态置为 `down`
- 控制端按 `AXIS_NODE_MONITOR_INTERVAL_SEC` 周期扫描超时节点；配置 `AXIS_LOCAL_REGION` 后只扫描本区域节点，防止多区域互相覆盖 `status`
- 建议 `AXIS_NODE_TIMEOUT_SEC` 明显大于 `AXIS_NODE_REPORT_INTERVAL_SEC`
- DNS 自动化是可选能力；未配置或未启用时，Axis 不会调用任何 DNS 服务商接口

分配接口示例：

```bash
curl -u admin:password \
  "http://127.0.0.1:9090/api/v1/nodes/assign?region=asia&zone=SG"
```

分配规则：

- 请求必须同时提供 `region` 和 `zone`
- 只会在 `status=up` 的节点中选择
- 优先在指定 `zone` 内选择；如果该 `zone` 没有任何 `up` 节点，则回退到同 `region` 内选择
- 加权分数公式为 `disk_usage_percent * 0.5 + cpu_usage_percent * 0.3 + memory_usage_percent * 0.2`
- 分数越低越优先；如果多台节点分数完全相同，则在并列最低分节点中随机返回 1 台
- 成功返回完整 `node` 信息
- 本期不包含“资源特殊需求阈值”过滤

## 多区域运维注意事项

AXIS 数据通过 TiCDC 跨区同步，每个区域都有完整数据副本；但 `axisd` 的节点状态监控（`MarkTimedOutNodesDown`）默认作用于**全局节点**。

**必须**为每个区域的 `axisd` 配置 `AXIS_LOCAL_REGION`，使超时下线只扫描本区节点，否则每个区域的超时监控会互相把别区节点打成 `down`：

```env
# /apps/Axis/.env（以亚洲为例）
AXIS_LOCAL_REGION=asia
```

**标准 regions 和 zones 必须只在权威区创建一次**，再通过 TiCDC 同步到其他区域；绝对不要在多个区域分别创建同名 region/zone，否则每个区域会分配不同 UUID，导致数据结构分叉。

**重新纳管时必须先清空所有区域的 `AXIS.managed_nodes`**，否则旧 UUID 通过唯一键约束会把新注册覆盖回旧身份。

## 可选 DNS 模块

Axis 可在 node 首次成功上报 `public_ip` 后，自动为该 node 分配稳定的子域名并写入 DNS 解析。

当前支持：

- 服务商：`cloudflare`
- 记录类型：`A`
- 记录值：node 当前上报的 `public_ip`

配置项：

- `AXIS_DNS_ENABLED`：是否启用，默认 `false`
- `AXIS_DNS_PROVIDER`：当前固定为 `cloudflare`
- `AXIS_DNS_ZONE`：目标根域，例如 `github.com`
- `AXIS_DNS_RECORD_PREFIX`：记录前缀，默认 `dl-`
- `AXIS_DNS_RECORD_TYPE`：默认且当前仅支持 `A`
- `AXIS_DNS_TTL`：Cloudflare TTL，默认 `1`（自动）
- `AXIS_DNS_PROXIED`：是否启用 Cloudflare 代理，默认 `false`
- `AXIS_DNS_CLOUDFLARE_API_TOKEN`：Cloudflare API Token，必须具备目标 Zone 的 DNS 编辑权限

分配规则：

- 当 `AXIS_DNS_RECORD_PREFIX=dl-` 时，首个成功分配的记录为 `dl-001.<zone>`
- 后续新增 node 依次递增为 `dl-002.<zone>`、`dl-003.<zone>` ...
- 同一个 node 一旦完成分配，后续重启或重复上报都会稳定复用原 `dns_name`
- 如果 node 后续上报的公网 IP 发生变化，Axis 会继续使用原域名并更新对应 `A` 记录
- 只有 `public_ip` 非空时才会触发 DNS 写入；公网 IP 探测失败不会阻塞基础纳管和心跳数据落库

## CLI 管理命令

### 注册服务器

```bash
axis service-register \
  --hostname sgp-edge-01 \
  --management-address 10.8.1.11:9090 \
  --region asia \
  --zone SG \
  --status up
```

可选参数：

- `--uuid`
- `--status`

必填参数：

- `--region`：大洲（asia、europe、australia、north_america、south_america）
- `--zone`：可用区，ISO-3166-1 alpha-2 国家代码（如 SG、CN、US）

说明：

- 如果不传 `--uuid`，系统会自动生成 `uuid4`
- 如果相同 `management_address` 已存在，会复用已有 UUID

### 列出全部服务器

```bash
axis service-list
```

输出摘要字段：`UUID`、`HOSTNAME`、`INTERNAL_IP`、`PUBLIC_IP`、`DNS_NAME`、`STATUS`、`REGION`、`ZONE`。

验证：执行 `axis service-list` 与 `axis service-show <uuid>`，确认资源指标（CPU cores、内存 GB、Swap GB、磁盘明细）带单位正常显示。

### 查看单台服务器

```bash
axis service-show <uuid>
```

输出完整资源详情（数值带单位：cores、GB、%）：

- CPU 核数、使用率
- 内存/Swap 总量、已用、使用率
- 全部磁盘挂载点明细（MOUNT_POINT、FILESYSTEM、TOTAL_GB、USED_GB、USAGE_PERCENT）
- `last_seen_at`、`last_reported_at`

### 删除服务器

```bash
axis service-delete <uuid>
```

### 设置服务器为 up

```bash
axis service-up <uuid>
```

### 设置服务器为 down

```bash
axis service-down <uuid>
```

### 查看区域聚合信息

```bash
axis region-list
```

输出字段：

- `REGION`：大洲
- `ZONE`：可用区（国家代码）
- `TOTAL`
- `UP`
- `DOWN`

## 管理端与 node 端职责边界

- `Axis`：控制平面，负责纳管、查询、状态维护与运维命令
- `axis-node`：节点代理，负责以 node 身份向管理端注册自己
- 管理端命令优先通过 `source /apps/Axis/axis-rc.sh` 后执行 `axis`
- 节点注册优先通过 `axis-node register` 或 `axis-node agent` 自动纳管
- 生产环境推荐通过 systemd 运行 `axisd` 与 `axis-node`
- `axis-node.service` 会默认把宿主机主机名注入为节点主机名

## License

Axis 使用 MIT License 发布。

这意味着：

- 可以商用
- 可以修改
- 可以私有部署
- 可以再分发

前提是保留原始版权与许可证声明。
