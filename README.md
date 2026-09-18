# Galaxy CLI 与 Galaxy Agent

Docker Swarm 的唯一接入方式是 [Docker Swarm 统一 Agent 架构](../galaxy-api/docs/swarm-agent-architecture.md)。Docker API 直连、SSH Agent、CLI 内常驻 Agent 和单 Manager Agent均为已废弃的旧版设计。

## 构建环境

- Go 1.26.4（`go.mod` 通过 `toolchain` 指令选择该版本）

构建用户 CLI：

```bash
make build
```

`make build` 强制使用 `CGO_ENABLED=0` 和 `-trimpath`，生成不依赖目标机器
glibc、musl 等用户态动态库的静态 Go 可执行文件。不要使用裸
`go build -o galaxy .` 生成正式分发包。

构建节点 Agent：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -o bin/galaxy-agent-linux-amd64 ./cmd/galaxy-agent
```

`galaxy` 和 `galaxy-agent` 使用同一个 Go Module。公共实现放在 `internal/agent`、`internal/dockerproxy`、`internal/terminal` 和 `internal/protocol`，不创建私有 Go Module。

Agent 以 Swarm Global Service 运行，因此所有 Manager 和 Worker 都必须能
拉取同一个镜像。生产发布必须先把 Agent 镜像推送到公共仓库，或推送到所有
Swarm 节点均可访问的私有仓库。默认构建当前 Docker 主机的平台，不要求
安装 Buildx：

```bash
make agent-image
```

镜像构建默认使用 `https://goproxy.cn,direct` 和
`sum.golang.google.cn` 下载、校验 Go Module。需要使用企业内部代理时可以
覆盖：

```bash
make agent-image \
  GoProxy=https://goproxy.example.com,direct \
  GoSumDB=sum.golang.google.cn
```

随后发布 CLI 时使用相同的 `AgentImage`，它会成为 `galaxy agent install`
的默认镜像；也可以安装时显式覆盖：

```bash
sudo galaxy agent install \
  --server https://galaxy.example.com \
  --image registry.example.com/galaxy/galaxy-agent:1.0.3
```

CLI 不内置任何镜像仓库地址。Agent 镜像按以下优先级确定：`--image` 参数、
环境变量 `GALAXY_AGENT_IMAGE`、构建时通过 `AgentImage` 注入的默认值；三者都
未提供时 `agent install` 会直接报错。发布指定版本可执行
`make build AgentImage=registry.example.com/galaxy/galaxy-agent:<version>`
与 `make agent-image AgentImage=registry.example.com/galaxy/galaxy-agent:<version>`。

从 `1.0.3` 开始，使用 IP 地址连接管理中心且 Galaxy API 与 Swarm Manager
部署在同一主机时，Manager IP 变化后 Agent 会先尝试原地址，再使用 Docker
`/info` 返回的当前 `NodeAddr` 连接相同端口。连接成功后会自动更新
`galaxy-agent` Global Service 的 API 与 Join 地址，并滚动恢复其他节点。
远程 API 或 HTTPS 部署仍应使用稳定域名，避免 DNS 与证书随 IP 变化失效。

私有 Agent 镜像仓库应先在 Galaxy 管理中心配置。`agent install` 会把镜像
地址上报给 API，由 API 按当前集群所属组织、Registry 地址和 namespace
匹配凭证，再一次性传给 Docker 创建 Global Service；效果等同于
`docker service create --with-registry-auth`，Worker 无需逐台执行
`docker login`，凭证也不会写入 CLI 临时配置。若 API 未匹配到仓库，CLI
才会回退读取当前执行用户的 Docker 登录配置。完全不使用仓库只适合离线
环境，此时必须把完全相同的镜像逐节点 `docker load`，不作为 Galaxy 的
默认安装流程。

## 组件职责

### `galaxy`

用户侧一次性命令：

```text
galaxy docker compose build ...
galaxy docker compose up ...
galaxy docker compose down ...
galaxy docker compose list
galaxy docker compose migrate --project ...
galaxy docker container list
galaxy docker container migrate --container ...
```

这些 Docker 命令仅访问目标主机上的 Docker Engine，不调用 Galaxy API，也不属于 Galaxy Web 产品能力。CLI 不作为常驻 Agent运行，不维护多个集群 WebSocket，也不保存 Manager SSH 私钥。

`compose list` 默认包含当前 Docker 节点上已停止的 Compose 项目，并支持 `--format json`。`migrate` 要求明确提供 `--project`，然后从 Docker Compose 项目清单及容器 Label 自动发现该项目的工作目录和配置文件。检查会覆盖 Compose 配置、现有容器、Swarm Manager 状态、Stack 兼容性、同名 Stack 冲突，以及镜像、端口、绑定挂载和本地卷等迁移风险；Compose v2 顶层 `name` 会被移除，字符串型 `published` / `target` 端口会转换为 Swarm 要求的整数。使用本机路径 bind mount、匿名卷或默认 `local` 命名卷的 Compose Service 会自动注入 `node.hostname == 当前节点` 放置约束；明确使用远程 Volume Driver 的 Service 不会被固定。已有约束如果指定或排除了其他节点，检查会失败，不会生成不可调度的 Stack。

`migrate` 已包含完整检查流程，不提供 `check`、`convert` 或 `run` 子层级：

```bash
galaxy docker compose migrate --project mysql
```

命令先展示检查表；存在失败项时立即退出。全部可迁移时显示确认框，只有输入完整的 `yes` 才执行迁移，其他输入均取消。自动化场景可使用 `--yes`；`--source-hash` 仅作为可选的额外配置一致性约束。执行期间会持续展示停止旧工作负载、提交 Stack 和 Service 副本就绪状态。迁移成功后会再次询问是否删除旧 Compose 容器和已发现的 Compose 配置文件；该清理确认独立于 `--yes`，项目目录、命名卷及其数据始终保留。

迁移报告使用 Lip Gloss v2 按 terminal cell/grapheme 宽度渲染中文、Emoji 和混合文本表格；交互终端中使用绿色、红色和黄色区分通过、失败和警告，输出被重定向或设置 `NO_COLOR` 时自动保持无 ANSI 控制码的纯文本。

新增子命令应遵循 [Galaxy CLI UI 规范](docs/cli-ui-guidelines.md)，复用 `pkg/cliui` 的标题、摘要、表格、检查状态、结论和操作提示组件。

### 文件同步忽略规则

`galaxy sync` 会读取项目根目录的 `.galaxyignore`，在向部署实例容器写入文件前
排除敏感配置和仅供本地使用的文件。语法与 `.gitignore` 类似，支持注释、通配符、
目录规则、`**` 和以 `!` 开头的反向包含规则；越靠后的规则优先级越高：

```gitignore
# 本地数据库账户和环境变量
.env
config/database.php
config/*-local.php
secrets/**

# 可以同步不含真实凭证的示例
!secrets/example.env
```

`.galaxyignore` 自身永远不会同步到容器。忽略规则只影响 `galaxy sync`，不会隐藏
`galaxy diff` 的差异；`--dry-run` 会将命中的文件显示为
`跳过：.galaxyignore`。建议将 `.galaxyignore` 提交到 Git，让项目成员共享相同的
同步安全边界。未配置该文件时保持原有同步行为。

默认跳过本地已删除的文件。使用 `galaxy sync --delete` 时，会删除 Git 变更中明确标记为
已删除的对应实例文件；`.galaxyignore` 仍会排除这些文件。可先执行
`galaxy sync --delete --dry-run` 查看待删除文件，不修改容器。删除只针对文件，不清理目录。

`galaxy sync` 会在写入前检查所有待同步文件的目标路径。如果实例中某个目标路径是
目录，命令会报告冲突并停止，不会用本地文件覆盖目录。缺失的父目录会在写入时创建。
请先手动处理路径冲突，再重新运行同步。

`container list` 只列出不属于 Compose 或 Swarm Task 的独立容器。独立容器复用相同的“检查 → 确认 → 执行 → 失败回滚”流程：

```bash
galaxy docker container migrate \
  --container my-app \
  --stack my-app \
  --service my-app
```

迁移会保留镜像、命令、入口、环境变量、端口、挂载、健康检查、重启策略、Capability 和普通 Label。独立容器会被停止，生成的单副本 Service 固定到原节点，以保留 host 发布端口、绑定挂载和本地 Docker Volume 的语义。默认等待最多一分钟，直到 Service 达到 `1/1`；部署失败或超时时会删除新 Stack 并自动启动原容器。迁移成功后会独立询问是否删除旧容器，只有再次输入完整的 `yes` 才删除，`--yes` 不会跳过这次清理确认；关联 Volume、绑定挂载路径和数据始终保留。等待时间可通过 `--wait` 调整，设为 `0` 可只提交部署，此时不会询问删除旧容器。`privileged`、`--rm`、host/container 网络、设备映射、GPU DeviceRequest 和旧式 Link 等无法安全等价转换的配置会阻止迁移。

### `galaxy-agent`

节点常驻服务：

- 以 Swarm Global Service部署到每个 Manager 和 Worker；
- 不监听任何端口；
- 主动连接 Galaxy API WebSocket；
- 通过本机 Docker Socket获得 Swarm ID、NodeID 和节点能力；
- 执行 API 授权的 Swarm、节点容器、终端、日志和指标领域命令；
- 不提供任意 Shell 或任意目标地址代理。

## 首次接入

1. 在 Galaxy“连接设置”生成有效期 15 分钟的 Bootstrap Token。
2. 在 Manager 使用分发给用户的 Galaxy CLI 执行：

```bash
sudo galaxy agent install --server https://galaxy.example.com
```

3. 复制 Web 生成的完整安装命令；命令通过 `--bootstrap-token` 携带一次性 Token。CLI 读取本机 Docker `/info`，向 API上报 Swarm ID、Manager NodeID 和 NodeAddr。
4. API 建立 Galaxy Cluster ID 与 Swarm ID 的映射并生成集群机器凭证。
5. API 按 Agent 镜像匹配当前组织的镜像仓库，并仅在本次 Bootstrap
   连接中下发拉取凭证。
6. Manager 创建版本化 Docker Secret，并携带 Registry Auth 部署
   `galaxy-agent` Global Service。
7. Bootstrap Token 立即失效。

Web 默认把 Token 写入一次性安装命令，避免再次输入；手工执行时仍可使用交互式密码提示或 `--bootstrap-token-file`。Bootstrap Token 有效期 15 分钟、只能使用一次，重新生成后旧 Token 立即失效。`galaxy` CLI 只负责一次性安装；常驻运行的仍是由 CLI 部署的 `galaxy-agent` Global Service。

如果 Global Service 已创建，但因 Galaxy API 地址误填为 `localhost`、
`127.0.0.1` 或旧地址而无法连接，不需要手工执行 Docker Service 更新命令。
在 Web“Galaxy Agent 连接”页面确认节点可访问的管理中心地址，然后复制执行：

```bash
sudo galaxy agent set \
  --server https://galaxy.example.com
```

升级 Agent 镜像时使用不可变的版本标签，并在 Manager 执行：

```bash
sudo galaxy agent set \
  --image registry.example.com/galaxy/galaxy-agent:1.0.3
```

`--server` 与 `--image` 可以在一次调用中同时设置。`agent set` 仅通过 Manager 本机
Docker Socket 查找受 Galaxy Label 管理的
`galaxy-agent` Service，保留 Secret、网络和其他部署配置，只更新指定参数并触发滚动更新。CLI 会拒绝回环地址和
不受 Galaxy 管理的同名 Service。

## Agent 网络

Agent 使用专用加密 Overlay 网络，只作为 WebSocket 客户端。Agent 到 API 的网络可达性可以由直连、SSH 反向端口转发、FRP、VPN或专线提供，所有方案使用相同的 WSS协议和 Agent Credential。

开发环境默认使用一条 `ssh -R` 命令将 Manager LAN 的 9501 反向映射到开发机 API 9501；需要常驻、多隧道或集中管理时再选择 FRP。“SSH 端口转发”只承载 API TCP流量；“SSH Agent 登录远程节点执行 Docker”属于已废弃的旧版管理架构。

## 安全边界

- Agent Credential 通过 Docker Secret挂载，API 只保存 Hash。
- Credential 与 Swarm ID、NodeID 一起在 WebSocket Upgrade 前校验。
- 每个 `cluster_id + node_id` 只允许一个有效 Session。
- Agent 容器不需要 Host 网络或 `--privileged`。
- Docker Socket 本身具有高权限，Agent 镜像必须最小化、只读并移除不必要 Capability。
- 凭证使用版本化 Docker Secret滚动轮转。
- 所有 Docker 和终端操作由 Galaxy API完成用户权限校验和审计。

## 目录规划

```text
cmd/galaxy/          用户 CLI 入口
cmd/galaxy-agent/    节点 Agent 入口
cmd/docker/agent.go  注册、节点身份、命令分发和 Docker Socket 通信
cmd/docker/          Galaxy CLI 的本地 Docker/Compose 工具
pkg/                 确需供外部使用的公共包
```
