# NexusDock

NexusDock 是面向个人和可信小型环境的 AgentDock 中心服务。它把多台 AgentDock 节点、长期记忆、工作流模板、运行状态和管理入口集中到一个自托管 Web 控制台中。

NexusDock 适合已经在一台或多台设备上使用 AgentDock，希望统一查看任务、Skill、MCP、记忆和运行状态的用户。它不是 AgentDock 的替代品：任务和工具仍在各个 AgentDock 节点上运行，NexusDock 负责集中管理、查询和协调。

## 主要能力

- **Recall 记忆库**：浏览、搜索、创建和编辑 Markdown / 文本记忆，查看 Git 变更与历史，并按需同步远端仓库。
- **经验卡片与向量召回**：把可复用结论整理为经验卡片；配置兼容的 Embeddings 服务后可进行语义搜索和索引重建。
- **多节点 Runtime**：配对多台 AgentDock，节点主动连接 Nexus，按节点查看任务、Skill、动态 MCP 和运行概况。
- **MCP 管理**：通过 NexusDock 管理选定 AgentDock 节点上的 HTTP 或 stdio MCP 服务、工具发现和隔离环境变量。
- **工作流模板**：集中浏览、匹配和维护可复用的任务工作流模板。
- **统一 MCP**：客户端只连接 NexusDock `/mcp`；Recall 等中心工具只出现一次，设备工具通过必填 `node_id` 路由。
- **安全与状态**：管理员登录、浏览器会话管理、短时单次配对码、设备身份 Token 和系统健康检查。
- **自托管 Web 控制台**：桌面端与移动端均可使用，后端 API 与前端由同一个服务提供。

## 快速开始

### 1. 准备环境

需要：

- Docker 与 Docker Compose
- Git
- 一个可写的数据目录

克隆仓库：

```bash
git clone https://github.com/uvwt/nexusdock.git
cd nexusdock
cp .env.example .env
```

生成一个仅用于程序化 API 的随机 Bearer Token：

```bash
openssl rand -hex 32
```

编辑 `.env`，至少确认以下配置：

```dotenv
NEXUS_DATA_DIR=./nexus-data
NEXUS_AUTH_TOKEN=<粘贴刚才生成的随机值>
NEXUS_REQUIRE_AUTH=true

RECALL_REPO_DIR=./recall
```

本机通过 `http://127.0.0.1` 试用时，还需要临时允许 HTTP 登录：

```dotenv
NEXUS_AUTH_ALLOW_INSECURE_HTTP=true
```

远程访问时不要保留这个设置，应使用 HTTPS，详见[安全部署](#安全部署)。

### 2. 创建数据目录

```bash
mkdir -p nexus-data recall
```

默认镜像使用 UID/GID `10001:10001` 运行。Linux 使用宿主机绑定目录时，首次启动前执行：

```bash
sudo chown -R 10001:10001 nexus-data recall
```


### 3. 初始化管理员

```bash
docker compose build nexusdock
docker compose run --rm nexusdock admin init owner
```

命令会在终端中要求输入并确认管理员密码。密码只写入 NexusDock 的 SQLite 数据库，不需要放进 `.env` 或 Compose 文件。

### 4. 启动服务

```bash
docker compose up -d nexusdock
```

检查服务：

```bash
curl http://127.0.0.1:18777/health
```

认证后还可检查 `/v1/system/status`、`/v1/runtime/nodes` 和 `/v1/workflow-templates`，并对 `nexus.db` 执行 SQLite `quick_check`。

然后在浏览器打开：

```text
http://127.0.0.1:18777
```

使用刚才创建的管理员账号登录。

## 首次使用

### 连接 AgentDock 节点

在 NexusDock 设置页点击“配对设备”，复制生成的命令并在目标设备运行：

```bash
agentdock nexus pair --endpoint https://nexus.example.com --code pair_xxx
```

重启 AgentDock 后，它会主动建立到 NexusDock 的 WSS 长连接。只需要 NexusDock 具备公网 HTTPS 地址；AgentDock 可以位于 NAT 或无入站公网的网络中。Nexus 不保存 AgentDock 地址和 AgentDock `/mcp` Token，Device Token 仅表达固定设备身份，不提供权限 scope 配置。

客户端可继续直连某台 AgentDock 的 `/mcp`，也可只连接 NexusDock 的 `/mcp` 使用汇总模式。直连模式的本地工具、认证与部署方式不变。

### 连接 NexusDock MCP

支持 OAuth 的 MCP 客户端可直接连接 NexusDock `/mcp` 并完成浏览器授权。不支持 OAuth、需要固定凭据的客户端，可在 Web 控制台的“设置 → MCP 接入”查看专用 Access Token，并使用：

```text
Authorization: Bearer <Access Token>
```

这个 Token 只允许访问 `/mcp`，不能访问 `/v1` 管理 API。点击“重置 Token”后旧值立即失效；新值会保存在 `NEXUS_DATA_DIR/secrets/mcp-access-token`，服务重启后保持不变。

### 使用 Recall

Recall 仓库默认位于 `RECALL_REPO_DIR`。你可以在 Web 控制台中：

- 新建、编辑、移动和删除 `.md`、`.markdown`、`.txt` 文件；
- 按关键词搜索记忆；
- 查看本地改动和版本历史；
- 把稳定经验整理为卡片；
- 配置 Embeddings 后进行向量搜索。

Recall 内容本身是普通 Git 仓库，可以继续使用现有的 Git 托管和备份方式。

### 本地版本与数据保护

当 `RECALL_REPO_DIR` 是 Git 仓库时，NexusDock 会在自身写入 Recall 后记录本地 Git 版本，并在 Web 控制台展示本地变更和版本历史。NexusDock 不配置、读取或访问 Git remote，也不提供远端同步或备份功能。

生产环境的数据保护由服务器管理员负责。至少应按部署策略保护 `NEXUS_DATA_DIR` 和 `RECALL_REPO_DIR`；可以使用宿主机快照、文件备份、Git 或其他运维工具，但这些都不属于 NexusDock 服务职责。

### 启用向量召回

NexusDock 支持 OpenAI 兼容的 `/v1/embeddings` 接口。示例：

```dotenv
RECALL_EMBEDDING_ENABLED=true
RECALL_EMBEDDING_ENDPOINT=http://embedding-service:8000/v1/embeddings
RECALL_EMBEDDING_MODEL=BAAI/bge-m3
RECALL_EMBEDDING_TIMEOUT_SECONDS=30
```

未配置 Embeddings 时，普通文件浏览、关键词搜索和本地版本历史仍可正常使用。

## 配置参考

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `NEXUS_HOST` | `127.0.0.1` | 监听地址；Docker 镜像内默认为 `0.0.0.0` |
| `NEXUS_PORT` | `18777` | HTTP 端口 |
| `NEXUS_DATA_DIR` | `./nexus-data` | SQLite 和系统密钥目录；容器内为 `/var/lib/nexus` |
| `NEXUS_AUTH_TOKEN` | 空 | 程序化 `/v1` API 的 Bearer Token |
| `NEXUS_REQUIRE_AUTH` | `false` | 为 `true` 时，没有配置 API Token 将拒绝启动 |
| `NEXUS_AUTH_ALLOW_INSECURE_HTTP` | `false` | 是否允许通过 HTTP 提交浏览器登录；仅限本机调试 |
| `NEXUS_TRUSTED_PROXIES` | `127.0.0.1,::1` | 允许提供 `X-Forwarded-*` 的反向代理地址 |
| `NEXUS_LOG_LEVEL` | `info` | `debug`、`info`、`warn` 或 `error` |
| `RECALL_REPO_DIR` | `./recall` | Recall Git 仓库目录；容器内为 `/recall` |
| `RECALL_EMBEDDING_ENABLED` | `false` | 是否启用经验卡片向量索引 |
| `RECALL_EMBEDDING_ENDPOINT` | 空 | OpenAI 兼容 Embeddings 地址 |
| `RECALL_EMBEDDING_MODEL` | `BAAI/bge-m3` | Embeddings 模型 |

完整示例见 [`.env.example`](./.env.example)。

## 数据

默认数据结构：

```text
NEXUS_DATA_DIR/
  nexus.db
  secrets/
    mcp-access-token

RECALL_REPO_DIR/
  .git/
  profile.md
  recall/
```

不要让两个 NexusDock 实例同时写同一个 SQLite 数据库。系统状态只写入 `NEXUS_DATA_DIR`，不得放到 `RECALL_REPO_DIR/.nexus`。恢复数据库后需要让仍持有有效 Device Token 的 AgentDock 重新连接；数据库异常时不要反复重启容器，回退应恢复上一个已验证镜像和部署前数据库快照。

## 安全部署

NexusDock 面向个人和可信环境，不应直接暴露在公网。

远程访问时建议：

- 让 Docker 端口继续只绑定 `127.0.0.1`；
- 使用 Caddy、Nginx、Traefik 或 Cloudflare Tunnel 提供 HTTPS；
- 保持 `NEXUS_AUTH_ALLOW_INSECURE_HTTP=false`；
- 只把实际反向代理地址加入 `NEXUS_TRUSTED_PROXIES`；
- 使用高强度管理员密码和随机 `NEXUS_AUTH_TOKEN`；
- 限制 `nexus-data`、Recall 仓库和凭据文件的宿主机权限。

Compose 默认以 `10001:10001` 运行，根文件系统只读，并丢掉全部 Linux capability。

浏览器使用管理员会话 Cookie，程序化 `/v1` 客户端使用：

```text
Authorization: Bearer <NEXUS_AUTH_TOKEN>
```

客户端自行设置的 `Host`、`X-Forwarded-For` 或 `X-Forwarded-Proto` 不会自动获得本地访问权限。

## 管理员恢复

忘记密码时，在服务所在主机的终端执行：

```bash
docker compose run --rm nexusdock admin recover owner
```

该操作需要直接访问 `NEXUS_DATA_DIR`，不会通过 Web 或远程 API 修改密码。

## 从源码运行

需要 Go `1.26.3`、Node.js/npm 和 Python 3：

```bash
make web-deps
make build
./bin/nexusdock
```

本地开发常用检查：

```bash
make check
make ci
```

`make check` 会执行 Go 格式、依赖、测试、`go vet`、公共契约和仓库边界检查；`make ci` 还会构建前端、执行 race 测试并生成生产二进制。

## 公共契约

HTTP API 与 DTO 定义在 `scripts/generate-contracts.py`。修改接口后执行 `make contracts`。

公开错误码必须列入 `scripts/check-contracts.py` 的 `ERROR_CODES`，并与源码中的公开错误码一致。Runtime 上游私有错误码只能通过 `upstream_code` 透传。
