# Skill Cloud 平台 — 需求文档 v0.2（已确认）

> 目标：搭建一个"Skill Cloud"平台，集中托管多个可被远程调用的 skills，本地 agent 通过 SDK / 标准协议发现并调用这些 skills。
>
> **状态：v0.2 — 关键决策已与产品 owner 确认；脚手架已落地，进入 P0 实现阶段。**
>
> 确认事项：
> - 后端：**Go**（gin）
> - SDK：**Python + TypeScript** 双发
> - **P0 包含 MCP 协议**（`/mcp` 暴露所有 skill 为 MCP tools）
> - Skill 形态：**docker 沙箱 + http_proxy 外部代理** 两种都支持
> - **MVP 直接做多租户**（org/user/api_key 三层 + 行级 `org_id` 隔离）
> - 仓库：`skill-cloud`，**public**
> - 部署：本地 `docker compose`

---

## 1. 一句话定义

**Skill Cloud** = 一个把"能力（skill）"作为云端服务发布、版本化、鉴权、调用的平台。本地 agent 不需要把工具代码塞到自己进程里，而是按需远程调用 Skill Cloud 上的 skill。

类比：
- npm / PyPI 之于代码包
- Hugging Face Hub 之于模型
- **Skill Cloud 之于 agent 可调用的能力**

---

## 2. 核心概念

| 概念 | 说明 |
|---|---|
| **Skill** | 一个可调用单元，有名字、版本、输入/输出 schema、实现代码或调用入口 |
| **Skill Manifest** | YAML/JSON 描述文件，定义 skill 的元数据、参数、权限、运行时 |
| **Skill Registry** | 平台的 skill 索引和元数据库，提供搜索、列表、版本管理 |
| **Skill Runtime** | 实际执行 skill 的沙箱环境（容器/函数/外部 HTTP） |
| **Agent Client / SDK** | 本地 agent 用来发现和调用远程 skill 的库（Python / TS / Go） |
| **Invocation** | 一次 skill 调用，含输入、输出、日志、计费、追踪 |
| **Namespace / Org** | 多租户隔离单位，skill 归属于某个组织或用户 |

---

## 3. 用户角色

1. **Skill Author（技能作者）** — 写 skill、推到平台、发布版本
2. **Agent Developer（agent 开发者）** — 在本地 agent 中接入 SDK，调用云端 skill
3. **Platform Admin（平台管理员）** — 审核、限流、计费、监控
4. **End User（最终用户，间接）** — 通过 agent 体验 skill 的能力

---

## 4. 核心功能需求（按优先级）

### P0 — MVP 必须有

- [ ] **Skill 注册中心**：CRUD skill 元数据 + 版本（semver）
- [ ] **Skill Manifest 规范**：一份 YAML 标准格式（见 §7），同时支持 `docker` 和 `http_proxy` 两种 `runtime.type`
- [ ] **Skill 发布流程**：`skill push` CLI / API，上传 manifest + （可选）代码打包
- [ ] **Skill 发现 API**：`GET /skills`, `GET /skills/{ns}/{name}`, 支持 tag / 关键词搜索
- [ ] **Skill 调用 API**：`POST /skills/{ns}/{name}/invoke`，同步返回结果
- [ ] **MCP Server 端点**：`POST /mcp` 实现 `initialize` / `tools/list` / `tools/call`，自动将所有 skill 映射为 MCP tools
- [ ] **Python SDK**：`client.call("ns/name", **kwargs)`
- [ ] **TypeScript SDK**：`client.call("ns/name", { ... })`
- [ ] **多租户 + 鉴权**：org / user / api_key 三层，行级 `org_id` 隔离
- [ ] **基础沙箱执行**：docker 一次性容器，超时 + CPU/内存限制；http_proxy 路径实现超时 + 重试
- [ ] **调用日志 / 审计**：每次 invocation 记录 org/user/skill/input/output/耗时/状态

### P1 — 应该有

- [ ] **异步调用 / 长任务**：返回 invocation_id，轮询或 webhook 回调
- [ ] **流式输出**：SSE / WebSocket，适合 LLM 类 skill
- [ ] **Web UI**：浏览 / 文档 / 试用 / 调用历史
- [ ] **CLI**：`skill init / push / list / call / logs`
- [ ] **配额 / 限流**：按 org 配置 QPS 和月调用次数
- [ ] **容器预热池**：减少 docker 冷启动延迟

### P2 — 后续迭代

- [ ] **Skill 市场 / 公开发布**：public skill discovery
- [ ] **计费**：按调用次数 / token / 计算时间
- [ ] **多区域部署**
- [ ] **OAuth / SSO**
- [ ] **Skill 链式编排**（workflow）
- [ ] **本地 + 云端 hybrid skill**：同一 manifest 既能本地跑也能云端跑

---

## 5. 非功能需求

| 维度 | 目标（MVP） |
|---|---|
| 可用性 | 单机部署 + Docker Compose，后续 k8s |
| 延迟 | 元数据 API p95 < 100ms；invocation 视 skill 而定 |
| 并发 | MVP 支持 ~100 QPS |
| 隔离 | 每次 invocation 在容器/沙箱内执行，禁止访问宿主 |
| 可观测 | 结构化日志 + Prometheus metrics + OpenTelemetry trace（可选） |
| 安全 | API key 加密存储、传输 HTTPS、输入大小限制、超时强制 |

---

## 6. 系统架构（建议）

```
┌─────────────────┐       ┌──────────────────────────────────────┐
│ Local Agent     │       │            Skill Cloud Platform       │
│ (Devin/Cursor/  │       │                                       │
│  Claude/自研)    │       │   ┌─────────────┐    ┌────────────┐  │
│                 │       │   │  API Gateway │───▶│  Registry  │  │
│  ┌───────────┐  │ HTTPS │   │  (FastAPI)   │    │  (Postgres)│  │
│  │ Agent SDK │──┼──────▶│   │              │    └────────────┘  │
│  └───────────┘  │       │   │  Auth / Rate │                    │
└─────────────────┘       │   │  Limit       │    ┌────────────┐  │
                          │   └──────┬───────┘───▶│  Object    │  │
                          │          │            │  Storage   │  │
                          │          ▼            │  (S3/MinIO)│  │
                          │   ┌─────────────┐    └────────────┘  │
                          │   │  Invocation │                    │
                          │   │  Dispatcher │                    │
                          │   └──────┬──────┘                    │
                          │          │                            │
                          │          ▼                            │
                          │   ┌─────────────┐                    │
                          │   │  Runtime    │ (Docker / firecracker│
                          │   │  Sandbox    │  / serverless)      │
                          │   └─────────────┘                    │
                          └──────────────────────────────────────┘
```

**关键组件：**

1. **API Gateway / Server** — FastAPI（Python）或 NestJS（TS），提供 REST + （可选）MCP
2. **Registry DB** — Postgres，存 skill 元数据、版本、用户、调用记录
3. **Object Storage** — S3 兼容（MinIO 本地），存 skill 代码包
4. **Runtime / Sandbox** — Docker 起一次性容器执行 skill，超时 kill
5. **Agent SDK** — 轻量 HTTP 客户端 + 类型生成

---

## 7. Skill Manifest 草案

```yaml
# skill.yaml
name: web_search                    # 全局唯一（namespace/name）
namespace: acme                     # 所属组织
version: 1.2.0                      # semver
description: "Search the web via Bing API."
author: john@acme.com
license: MIT
tags: [search, web, retrieval]

runtime:
  type: docker                      # docker | python | http_proxy
  image: python:3.12-slim
  entrypoint: "python -m web_search"
  timeout_seconds: 30
  memory_mb: 512

inputs:                              # JSON schema
  query:
    type: string
    description: "Search query"
    required: true
  top_k:
    type: integer
    default: 5

outputs:                             # JSON schema
  results:
    type: array
    items:
      type: object
      properties:
        title: { type: string }
        url:   { type: string }
        snippet: { type: string }

secrets:                             # 由平台注入
  - BING_API_KEY

permissions:                         # 沙箱白名单
  network:
    - api.bing.microsoft.com
  filesystem: read-only
```

---

## 8. API 设计草案（REST）

| Method | Path | 说明 |
|---|---|---|
| `POST` | `/v1/auth/tokens` | 创建 API key |
| `GET`  | `/v1/skills` | 列出 / 搜索 skills |
| `GET`  | `/v1/skills/{ns}/{name}` | 获取 skill 详情 |
| `GET`  | `/v1/skills/{ns}/{name}/versions` | 列出版本 |
| `POST` | `/v1/skills` | 创建 / 发布 skill（上传 manifest + 代码包） |
| `POST` | `/v1/skills/{ns}/{name}/invoke` | 同步调用 |
| `POST` | `/v1/skills/{ns}/{name}/invoke?async=true` | 异步调用 |
| `GET`  | `/v1/invocations/{id}` | 查询异步结果 |
| `GET`  | `/v1/invocations` | 调用历史 |

**MCP 兼容入口**（P1）：`/mcp` 暴露标准 MCP server，自动把所有 skills 映射为 MCP tools。

---

## 9. 已确认技术栈

| 层 | 选型 | 备注 |
|---|---|---|
| 后端 | **Go 1.23 + gin** | 性能 + 单二进制部署 |
| DB | Postgres 16 | MVP 用 sqlc 或 pgx |
| 对象存储 | MinIO（dev） / S3 兼容（prod） | 存 skill 代码包 |
| 沙箱 | Docker（MVP） → Firecracker / gVisor（后续） | 一次性容器，超时 kill |
| SDK | **Python + TypeScript**（双发，同步首发） | |
| CLI | Cobra（Go） | 与 server 同仓库共享类型 |
| 协议 | REST + **MCP**（P0 同时支持） | |
| 前端 UI | Next.js（P1，可选） | |
| 部署 | `docker compose`（MVP） → k8s/Helm（后续） | |

---

## 10. 仓库结构建议（monorepo）

```
skill-cloud/
├── README.md
├── docker-compose.yml
├── server/                 # FastAPI 平台后端
│   ├── app/
│   ├── alembic/            # DB 迁移
│   ├── tests/
│   └── pyproject.toml
├── sdk/
│   ├── python/             # pip install skill-cloud
│   └── typescript/         # npm i @skill-cloud/client
├── cli/                    # skill CLI
├── runtime/                # 沙箱执行器
│   └── docker-runner/
├── examples/
│   ├── hello-skill/        # 最小示例 skill
│   └── web-search-skill/
├── web/                    # （P1）Next.js 控制台
├── docs/
└── .github/workflows/      # CI
```

---

## 11. MVP 验收标准（Definition of Done）

1. `docker compose up` 一键起平台（server + DB + MinIO）
2. 能用 CLI 发布 `examples/hello-skill`：`skill push`
3. 能在 Python REPL 里：
   ```python
   from skill_cloud import Client
   c = Client(api_key="...")
   print(c.call("acme/hello", name="world"))
   # -> "hello, world"
   ```
4. 该次调用在 `/v1/invocations` 中能查到日志
5. 沙箱：恶意 skill 试图 `rm -rf /` 不会影响宿主
6. 单元测试 + 集成测试覆盖 server / sdk 主流程，CI 跑通

---

## 12. 风险与开放问题

1. **沙箱安全**：Docker 起容器有冷启动延迟（100ms~秒级），是否接受？要不要预热池？
2. **代码 vs HTTP proxy**：skill 是"上传代码 → 平台执行"还是"注册一个 HTTP endpoint → 平台转发"？前者更省心、后者更轻量。建议两种都支持。
3. **MCP 兼容**优先级 — 若你的 agent 已经在用 MCP，把 MCP server 放到 P0 会大幅降低接入成本。
4. **多租户隔离**：MVP 是单租户还是多租户？影响 DB schema 和鉴权设计。
5. **开源 / 闭源** — 影响协议、文档详细程度、CI 配置。

---

## 13. 下一步（待你确认后我会做）

1. 在 GitHub 上新建 `skill-cloud` 仓库（public / private？）
2. 按 §10 结构生成脚手架：FastAPI server + 最小 Python SDK + 一个 hello skill 示例 + docker-compose
3. 配好 CI（lint + test）
4. 提交首个 PR，附预览部署链接（如果需要）
5. 后续按 P0 → P1 → P2 迭代

---

## 14. 关键决策记录

| # | 问题 | 决策 |
|---|---|---|
| 1 | 后端技术栈 | **Go + gin**，Python/TS SDK 双发 |
| 2 | MVP 是否包含 MCP | **是**，P0 就实现 `/mcp` |
| 3 | Skill 形态 | **docker 沙箱 + http_proxy 两种都支持** |
| 4 | 多租户 | **MVP 直接做** org/user 隔离 |
| 5 | 仓库可见性 | **Public** |
| 6 | 仓库名 | `skill-cloud` |
| 7 | 部署目标 | 本地 `docker compose`，后续再 k8s |
