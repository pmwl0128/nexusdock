# AgentDock 原生 Artifact Relay

## 背景

AgentDock 需要一个不依赖 OpenList 的原生临时文件中转能力，使 ChatGPT Connector 或已注册设备可以把文件本体上传到 Nexus，再由一个或多个目标设备主动拉取。该能力用于合法文件、代码包、日志和构建产物的持久化与跨设备传输，不用于绕过平台安全限制。

## 目标

1. Nexus 使用本地磁盘临时保存密文，SQLite 保存 Artifact 与 Delivery 元数据。
2. 上传前创建一次性上传任务；文件通过流式 multipart 直传 Nexus，单文件上限 500 MiB。
3. 默认保留 24 小时，最长 7 天；支持全部 Delivery 终态后立即删除。
4. 支持单个 Artifact 分发到多个设备，每台设备独立 Delivery 状态。
5. 目标设备通过现有命令队列领取 `artifact.pull`，使用设备凭据与短时 Delivery Token 双重鉴权下载。
6. 文件端到端加密：发送端使用随机 AES-256-GCM 文件密钥流式加密；每个目标设备使用 X25519 封装文件密钥；Nexus 只保存密文。
7. 设备先写入受控 inbox；可使用设备本地预配置的逻辑目标映射进行原子移动，禁止任意绝对路径。
8. 支持 reject/rename/overwrite 冲突策略，默认 reject；支持可选安全解包，默认不解包；绝不自动执行文件。
9. AgentDock 暴露 Artifact 工具入口，并为 Connector 文件参数声明 binary/file 语义；普通设备也可从本地文件或目录发送，目录由发送端打包。

## 非目标

- 首版不实现分片上传、断点续传、S3/MinIO/OpenList。
- 不开放任意 Shell、任意目标路径或上传后自动执行。
- 不在 Nexus 中解密或扫描文件明文。
- 首版不要求 Nexus Web 前端页面。

## 关键协议

### 创建上传任务

受 OAuth/Web Session 或 Bearer API Key 保护的 `POST /v1/artifacts/uploads`，以及设备凭据保护的设备上传入口。请求包含文件名、目标设备、分发方式、保留策略、逻辑目标、冲突策略和解包选项。响应只显示一次上传 Token，并返回每个目标设备的 X25519 公钥与 Delivery ID。

### 上传密文

`POST /v1/artifacts/{artifactID}/content` 使用 `multipart/form-data`，字段 `file` 为密文容器，字段 `manifest` 为加密元数据。请求使用短时一次性 Upload Token。Nexus 流式写 `.part`、计算密文 SHA-256，并原子改名。

### 下载与结果

目标设备用自身 Bearer 设备凭据及 `X-Artifact-Delivery-Token` 下载密文。设备解密后校验明文大小和 SHA-256，写入受控 inbox 或逻辑目标，随后回报 Delivery 成功或失败。

## 加密格式

- 设备单独维护 X25519 密钥对，不复用 Ed25519 身份密钥。
- Artifact 生成一个随机 32 字节文件密钥和一对临时 X25519 密钥。
- 文件使用分块 AES-256-GCM 加密，Nonce 为随机 8 字节前缀加 32 位块序号。
- 每个目标设备使用 X25519 ECDH + HKDF-SHA256 派生封装密钥，再用 AES-GCM 封装文件密钥。
- Nexus 保存临时公钥、封装后的文件密钥、Nonce 和密文，但无法恢复明文。

## 安全约束

- 文件名必须是 basename，拒绝路径分隔符、NUL 和空值。
- 逻辑目标只允许受控标识符；真实路径由目标设备本地映射。
- 下载必须同时通过设备身份与 Delivery Token，且 Delivery 必须属于该设备。
- Token 只存 SHA-256 摘要，不记录明文。
- 上传和下载均流式处理，不把 500 MiB 文件读入内存。
- 解包拒绝绝对路径、`..`、符号链接，并限制文件数和展开后总大小。
- 日志和错误不得包含 Token、密钥、文件内容或 Authorization Header。

## 验收标准

1. Nexus 全量 `go test ./...`、`go vet ./...`、契约检查通过。
2. AgentDock 全量 `go test ./...`、`go vet ./...`、构建通过。
3. HTTP 测试覆盖未鉴权、Token 错误、目标设备不匹配、超限上传、一次性 Token、下载双鉴权和过期清理。
4. 加密测试覆盖多块往返、篡改失败、错误设备私钥失败。
5. 生产部署后，DockMini 上报 `artifact-relay` 能力且含 X25519 公钥字段。
6. 实际发送一个测试文件：Nexus 磁盘只出现密文；DockMini 成功解密写入 inbox；明文 SHA-256 与源文件一致；Delivery 状态为 completed。
7. 服务重启后 Artifact/Delivery 状态仍可查询，过期清理可执行。
8. 两个仓库均提交并推送；恢复开发前已有的 Web 鉴权未提交改动且内容不丢失；长期记忆更新为已验证事实。
