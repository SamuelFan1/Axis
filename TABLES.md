# Axis 表说明

## 总览

`Axis` 是控制平面，不是网盘业务库。  
它的表不能一刀切地都按“全局强一致业务真相”处理，而应按真实数据性质分层：

- 静态主数据：`regions`、`zones`
- 全局单权威分配/生成表：`dns_bindings`、`dns_binding_counters`、`routing_snapshot_*`
- 区域运行态/热写表：`managed_nodes`、`managed_nodes_history`、`routing_observations`

## 表分层结论

| 表名 | 数据性质 | 最终建议策略 |
| --- | --- | --- |
| `managed_nodes` | 区域运行态 | 区域本地化 |
| `managed_nodes_history` | 区域运行态历史 | 区域本地化 |
| `dns_bindings` | 全局命名权威表 | 单权威生成+复制 |
| `dns_binding_counters` | 全局序号分配器 | 单权威生成+复制 |
| `regions` | 静态主数据 | 全局静态同步 |
| `zones` | 静态主数据 | 全局静态同步 |
| `routing_observations` | 区域观测累加表 | 区域本地化 |
| `routing_snapshot_manifests` | 单权威派生产物 | 单权威生成+复制 |
| `routing_snapshot_bundles` | 单权威派生产物 | 单权威生成+复制 |

---

## `managed_nodes`

- 表功能：活动节点主表，保存节点身份、区域、状态、资源指标、监控快照、DNS 镜像。
- 主键 / 关键唯一约束：
  - 主键：`uuid`
  - 唯一约束：`management_address`、`dns_label`、`dns_name`
- 主要写入来源：
  - `POST /api/v1/nodes/register`
  - `POST /api/v1/nodes/report`
  - 管理员 `service-up` / `service-down`
  - 后台 `NodeMonitor` 超时将节点标记为 `down`
  - DNS 绑定成功后的镜像回写
- 容易遗漏的自动维护链路：
  - `axis-node` 周期心跳会持续覆盖 `cpu_usage_percent`、`memory_used_gb`、`monitoring_snapshot`
  - `NodeMonitor` 会自动更新 `status`
  - 启动期只有在显式开启 `AXIS_AUTO_SCHEMA_UPGRADE` 时，才允许执行一次性 `region_uuid/zone_uuid` 迁移补写
- 数据性质：区域运行态热表
- 最终建议策略：`区域本地化`
- 理由：这张表的差异天然表现为同一 `uuid` 的实时状态不同，不适合要求多区长期字节级一致。更合理的是本区主写、全局只读汇总。

## `managed_nodes_history`

- 表功能：活动节点被新 UUID 替换时的归档历史表。
- 主键 / 关键唯一约束：
  - 主键：`history_id`
- 主要写入来源：
  - `Register()` 命中新旧 UUID 冲突时，归档旧 active 节点
- 容易遗漏的自动维护链路：
  - 只有“同 `management_address` 被新 UUID 替换”才写，不是全量审计表
- 数据性质：区域运行态历史表
- 最终建议策略：`区域本地化`
- 理由：它依附 `managed_nodes` 生命周期，属于节点归属区的历史，不需要做全局强一致主写。

## `dns_bindings`

- 表功能：节点 UUID 到稳定 `dl-*` 域名的中心权威绑定表。
- 主键 / 关键唯一约束：
  - 主键：`node_uuid`
  - 关键唯一性：`dns_label`、`dns_name`、序号分配
- 主要写入来源：
  - 节点首次带公网 IP 上报时 `AllocateForNode()`
  - 后续心跳里的 `UpdateLastPublicIP()`
  - 一次性迁移 / 修复场景下的 `SeedFromManagedNodes()`
- 容易遗漏的自动维护链路：
  - `managed_nodes.dns_label/dns_name` 只是镜像，真正权威在 `dns_bindings`
  - 首次分配会同时推进序号分配器
- 数据性质：全局命名权威表
- 最终建议策略：`单权威生成+复制`
- 理由：它本质是全局命名空间，不能由多区并发生成。若后续确定控制面权威区为 `asia`，则应由 `asia` 单点写入。

## `dns_binding_counters`

- 表功能：按 `(zone, prefix)` 维护下一个可分配 DNS 序号。
- 主键 / 关键唯一约束：
  - 主键：`zone, record_prefix`
- 主要写入来源：
  - `AllocateForNode()` 事务内推进序号
  - 一次性迁移 / 修复场景下的 `EnsureCounterFloor()`
- 容易遗漏的自动维护链路：
  - 它和 `dns_bindings` 是成对工作的，全局唯一语义比普通表更强
- 数据性质：全局序号分配器
- 最终建议策略：`单权威生成+复制`
- 理由：这是典型单调计数器，多区同时写会天然冲突。

## `regions`

- 表功能：区域主数据表。
- 主键 / 关键唯一约束：
  - 主键：`uuid`
  - 关键字段：`name`
- 主要写入来源：
  - `POST /api/v1/regions`
  - CLI `region-create`
- 容易遗漏的自动维护链路：
  - `AXIS_AUTO_SCHEMA_UPGRADE` 开启时，会优先按配置补齐 `regions`
- 数据性质：静态主数据
- 最终建议策略：`全局静态同步`
- 理由：应由单点创建后扩散到全区，不应各区分别创建。

## `zones`

- 表功能：可用区主数据表，当前已显式从属于 `regions`。
- 主键 / 关键唯一约束：
  - 主键：`uuid`
  - 关键字段：`region_uuid`、`name`
  - 唯一约束：`region_uuid + name`
- 主要写入来源：
  - `POST /api/v1/zones`
  - CLI `zone-create --region-uuid`
- 容易遗漏的自动维护链路：
  - `AXIS_AUTO_SCHEMA_UPGRADE` 开启时，会优先按配置补齐 `regions/zones`
  - 旧的“只有 `name` 没有 `region_uuid`”结构会被兼容迁移到新的从属关系
- 数据性质：静态主数据
- 最终建议策略：`全局静态同步`
- 理由：和 `regions` 一样是低频静态字典，应一次创建、全局同步；但 zone 不再是全局唯一名，而是区域内唯一名。

## `routing_observations`

- 表功能：按 `(source_colo, target_node_uuid)` 聚合路由观测累计值。
- 主键 / 关键唯一约束：
  - 主键：`source_colo, target_node_uuid`
- 主要写入来源：
  - `POST /api/v1/routing/observations`
  - `UpsertMany()` 累加成功延迟、成功数、错误数、样本数
- 容易遗漏的自动维护链路：
  - 这是累加表，不是滑窗表
  - 当前没有 TTL 清理或衰减逻辑
- 数据性质：区域观测运行态
- 最终建议策略：`区域本地化`
- 理由：观测值天然带地域性，多区同时对同一主键累加会放大冲突；更适合按采集区写、本地消费或后续聚合。

## `routing_snapshot_manifests`

- 表功能：每次路由快照的总清单，保存版本、TTL、全局/区域/zone 候选引用。
- 主键 / 关键唯一约束：
  - 主键：`version`
- 主要写入来源：
  - 后台 `RoutingSnapshotPublisher`
  - 管理接口 `POST /api/v1/routing/snapshots/generate`
- 容易遗漏的自动维护链路：
  - worker 启动后立即跑一次，然后按周期持续生成新版本
  - 代码里没有自动清理过期快照
- 数据性质：单权威派生产物
- 最终建议策略：`单权威生成+复制`
- 理由：它不是同一行热更新表，而是追加型版本表。多 publisher 会直接制造额外 `version`。

## `routing_snapshot_bundles`

- 表功能：与 manifest 同步生成的分区域快照明细。
- 主键 / 关键唯一约束：
  - 主键：`version, region_name`
- 主要写入来源：
  - `GenerateAndStore()` 与 `routing_snapshot_manifests` 同批生成
- 容易遗漏的自动维护链路：
  - 它与 manifest 必须成对看，不能单独作为运行态表理解
- 数据性质：单权威派生产物
- 最终建议策略：`单权威生成+复制`
- 理由：和 manifest 一样，应由单点生成，再向各区只读扩散。

---

## 总结

`Axis` 不适合继续按 `AXIS.*` 整库一刀切地做全区多主热写。更合理的最终形态是：

- `regions`、`zones`：静态主数据，单点创建后全局静态同步
- `dns_bindings`、`dns_binding_counters`、`routing_snapshot_*`：单权威生成/写入，再向其他区复制
- `managed_nodes`、`managed_nodes_history`、`routing_observations`：区域运行态，本地化写入更自然
- `managed_nodes` 上的 `dns_*`、`region_uuid`、`zone_uuid` 只是镜像/补全字段，不再作为持续性反向回灌主真相的来源
