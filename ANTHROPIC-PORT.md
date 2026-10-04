# Anthropic Messages / Codex Responses 入站兼容层 · 维护说明

本文件是本仓库**唯一需要读的维护文档**。上游 README（`README.md`）保持原样，只在顶部加了一段指向本文件的说明。

- 本仓库 fork 自 [`linguo2625469/workbuddy2api-panel`](https://github.com/linguo2625469/workbuddy2api-panel)
- 基线：上游 **tag `v1.11.11`**（commit `1b6abaa0cce4a960749494cad56d67fe87a6ee1f`）
- 默认分支：`feat/anthropic-messages-responses`
- 许可证：MIT（署名链：Sliverkiss → linguo2625469 → 本仓库），见 `LICENSE`

---

## 1. 这个分支做了什么

上游作者曾收到 PR **#63**「feat(server): 适配 Codex Responses 与 Anthropic Messages 入站」，实现完整（含 21 个测试），但以「**入站协议保持单一 + 维护成本**」为由**关闭**了它。也就是说：**上游 1.11.12+ 永远不会带这个功能**，只能自己维护一个衍生分支。

本仓库做的是把这个已关闭的实现**移植到 v1.11.11** 上，并补了两处上游没有的小修：

| # | 改动 | 来源 |
|---|---|---|
| 1 | Anthropic Messages 入站：`/v1/messages`、`/messages` | 移植自 `misakano7545/workbuddy2api-panel` 分支 `feat/anthropic-messages-responses` = `9a11e2b2dce470c8bc9620950e4a1b3c49af3ae1`（该分支基线是 **v1.11.7**，本仓库移植到 **v1.11.11**） |
| 2 | Codex Responses 入站：`/v1/responses` | 同上 |
| 3 | 面板运行日志 / `data/request-logs/*.jsonl` 归档盲区修复 | 本仓库自加（上游把入站路径写死成 `/v1/chat/completions`，导致新端点的流量在面板「运行日志」整片消失） |
| 4 | `Dockerfile.local`（含中国镜像 GOPROXY + HEALTHCHECK） | 本仓库自加（上游 `Dockerfile` 未改，仍可用） |

**没有改的（很重要）**：聊天主链路（选号 / 换号 / 冷却 / 熔断 / 会话粘性 / 提示词改写 / 脱敏 / 用量记账 / WAF）**一行未动**；没有新增依赖；没有新增配置项；没有改面板、前端与存储格式。新端点只是把请求转换成 `chat_completions` 形态后，**走原来那条完整管线**。

---

## 2. 相对上游 v1.11.11 的完整差异（9 个文件）

补丁文件：`anthropic-port.patch`（可用 `git apply -3` 或 `patch -p1` 打在任何 v1.11.11 树上）。

| 文件 | 增/删 | 说明 |
|---|---|---|
| `internal/server/messages.go` | +952 / −0 | **新**：Anthropic Messages ⇄ chat_completions 双向转换 + SSE 事件合成 |
| `internal/server/responses.go` | +873 / −0 | **新**：Codex Responses ⇄ chat_completions + 事件流回写 |
| `internal/server/messages_test.go` | +418 / −0 | **新**：9 个用例 |
| `internal/server/responses_test.go` | +418 / −0 | **新**：8 个用例 |
| `internal/server/reqlog_path_test.go` | +50 / −0 | **新**：归档路径回归测试（本仓库自加） |
| `internal/server/handler.go` | +16 / −2 | 3 条路由；请求归档门槛 `loggedChatPath()`；`trace.path` |
| `internal/server/logging.go` | +7 / −1 | `requestTrace` 增加 `path` 字段并写入事件 |
| `go.mod` | +2 / −1 | redis 从 `indirect` 提为直接 `require`（构建时 `-mod=mod` 顺带产生，**无新依赖**；`go.sum` 未变） |
| `Dockerfile.local` | +34 / −0 | **新**：本仓库构建用（`GOPROXY=https://goproxy.cn,direct` + `HEALTHCHECK`） |

另有两个非代码改动：`README.md` 顶部说明、`.gitignore` 放行 `ANTHROPIC-PORT.md`（上游 `.gitignore` 忽略所有 `*.md`）。

> 移植时的一个坑：`handler.go` / `panel.go` 在上游是 **LF**，plus 类分支常被改成 CRLF。用 `diff` 对比时务必加 `--strip-trailing-cr`，否则会虚报上千行差异。

---

## 3. 新增端点与客户端接入

| 端点 | 协议 | 鉴权 |
|---|---|---|
| `POST /v1/messages`、`POST /messages` | Anthropic Messages | `Authorization: Bearer <api_key>` **或** `x-api-key: <api_key>` |
| `POST /v1/responses` | Codex Responses | `Authorization: Bearer <api_key>` |

模型名直接用面板「模型与档位」页里的 id（如 `cn:hy3`、`cn:deepseek-v4.1-flash`）。

### 3.1 Claude Code CLI（实测 v2.1.289 通过）

```bash
export ANTHROPIC_BASE_URL=http://<网关地址>:7863     # ⚠️ 不要带 /v1，客户端自己拼 /v1/messages
export ANTHROPIC_AUTH_TOKEN=<面板 api_key>           # Bearer；用 ANTHROPIC_API_KEY 则发 x-api-key，两者都认
export ANTHROPIC_MODEL=cn:hy3                        # 面板里的模型 id
claude -p "只回答两个字：正常" --output-format json
```

实测结果：`result:"正常"`、`is_error:false`、`stop_reason:end_turn`；带工具往返（Read 工具）时 `num_turns=2`，说明 `tool_use → tool_result` 闭环真的走通。

### 3.2 Codex CLI

Responses 端点：`base_url = http://<网关地址>:7863/v1`。

### 3.3 curl

```bash
# Anthropic Messages（工具调用）
curl -s http://127.0.0.1:7863/v1/messages \
  -H 'content-type: application/json' \
  -H 'x-api-key: <api_key>' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{"model":"cn:hy3","max_tokens":256,
       "tools":[{"name":"get_weather","description":"查天气","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}],
       "tool_choice":{"type":"tool","name":"get_weather"},
       "messages":[{"role":"user","content":"北京天气"}]}'
# → content:[{"type":"tool_use","name":"get_weather","input":{"city":"北京"}}], stop_reason:"tool_use"

# OpenAI 兼容（老用法不变）
curl -s http://127.0.0.1:7863/v1/chat/completions \
  -H 'authorization: Bearer <api_key>' -H 'content-type: application/json' \
  -d '{"model":"cn:hy3","messages":[{"role":"user","content":"hi"}],"max_tokens":64}'
```

---

## 4. 已知限制 / 与官方 API 的差异（都是实测结论，不是猜测）

| 项 | 现状 | 影响 |
|---|---|---|
| `POST /v1/messages/count_tokens` | **404**（未实现） | Claude Code 2.1.289 实测未调用；但若将来调用会失败 |
| `message_start.message.usage` | 恒为 `0/0`；真实 usage 只在 `message_delta.usage` | 依赖首帧 usage 的统计脚本会读到 0 |
| `GET /v1/models` | 只认 `Authorization: Bearer`（`x-api-key` → 401） | Claude Code 不走这个端点，无影响 |
| thinking / `thinking_delta` | 不支持（上游响应无 signature） | 不支持 extended thinking 块 |
| Codex `previous_response_id` / `store` | 不保存状态 | 多轮需客户端自己带全量上下文 |
| Claude Code 的 `[claude-code:unrecognized_model]` 警告 | 客户端不认识自定义模型名 | 仅日志噪音 |
| Claude Code JSON 里的 `total_cost_usd` | 客户端按官方价估的**假账** | 真实成本看面板「用量」页 |
| `ANTHROPIC_BASE_URL` 带 `/v1` | `POST /v1/v1/messages` → 404 | 按 3.1 写即可 |
| 工具能力 | `tools` / `tool_choice`（含 `disable_parallel_tool_use` → `parallel_tool_calls:false`）、`tool_use`/`server_tool_use` → `tool_calls`、`tool_result` → `role:tool`、出站 `tool_calls` → `tool_use` block、流式 `input_json_delta` **全部可用** | — |

---

## 5. 构建

### Docker（推荐）

```bash
docker build -f Dockerfile.local -t wb2api-anthropic:1.11.11 .
```

`Dockerfile.local` = 上游 `Dockerfile` + 中国区 `GOPROXY=https://goproxy.cn,direct` + `GOFLAGS=-mod=mod` + 补回的 `HEALTHCHECK`。**注意**：官方 `Dockerfile` 也能构建本分支（功能完全一致），只是在国内拉 Go 模块会超时。

### 裸机

需要 Go ≥ 1.22.5：

```bash
go test ./internal/server/     # 移植的 17 个用例应全绿
go build -o wb2api ./cmd/server
```

> ⚠️ 全量 `go test ./...` 里有 **1 个既有失败**：`internal/reqlog` 的 `TestArchiveRotationReadAndFilter`。它在**纯净上游 v1.11.11 上同样失败**，与本分支改动无关（本仓库未修）。开启 CI 前请知悉。

---

## 6. 部署（compose 参考）

仓库里的 `docker-compose.yml` 是上游的 `build: .` 版本；本分支线上实际用的是 image 版本：

```yaml
services:
  wb2api:
    image: wb2api-anthropic:1.11.11      # 或 build 出来的自建 tag
    container_name: workbuddy2api
    restart: unless-stopped
    user: "10001:10001"
    environment: [TZ=Asia/Shanghai]
    ports: ["7863:7863"]
    volumes:
      - ./auths:/app/auths
      - ./data:/app/data
      - ./config.json:/app/config.json   # 宿主必须先存在该文件，否则 Docker 会建成目录
```

要点：

1. 首次部署 `cp config.example.json config.json`，并把 `api_key` 从默认 `test_key` 改成强随机值；
2. 容器以 uid 10001 运行，挂载目录属主不符会报 `permission denied`（见 compose 内注释的三种解法）；
3. 若宿主**没有** `config.json`，程序会自己生成一份（含 `crypto/rand` 生成的 `sk-` 密钥），但前提是该路径是文件而不是目录；
4. 回滚：镜像打 tag 保留旧版，改 `image:` 后 `docker compose up -d` 即可；`./auths`、`./data`、`config.json` 都在挂载卷里，重建不丢账号与用量。

---

## 7. 升级上游（最重要的维护动作）

因为上游不会合并这个功能，**每次上游发版都要重放一次补丁**：

```bash
# 1) 拿上游新版本
git remote add upstream https://github.com/linguo2625469/workbuddy2api-panel.git
git fetch upstream --tags

# 2) 以新 tag 为基线开分支
git checkout -b sync/v1.11.12 v1.11.12

# 3) 重放补丁（冲突几乎只出现在 handler.go 的路由表）
git apply --3way anthropic-port.patch

# 4) 跑测试 + 构建
go test ./internal/server/
docker build -f Dockerfile.local -t wb2api-anthropic:<新版本> .

# 5) 提交、推送，然后 PR 到本仓库默认分支
git commit -am "port: Anthropic Messages / Responses to v1.11.12"
git push origin sync/v1.11.12
```

**冲突预判**：上游凡是动 `internal/server/handler.go`（路由注册表 / 鉴权中间件）或 `internal/server/logging.go`（`requestTrace`）的提交，都可能冲突。冲突解法通常是「保留上游新代码 + 重新加上 3 条路由和 `loggedChatPath()`」。

**重生成补丁**（升级完成后必做，让补丁始终对应当前基线）：

```bash
# 在两个目录上做，务必带 --strip-trailing-cr，否则 CRLF 会虚报上千行
diff -ruN --strip-trailing-cr /path/上游-vX.Y.Z /path/本仓库树 > anthropic-port.patch
```

**如果本机 git 报 TLS 错误**（Windows schannel：`SEC_E_NO_CREDENTIALS` / 连接超时）：

```bash
git -c http.sslBackend=openssl clone https://github.com/...
# 永久生效（会写 ~/.gitconfig）：git config --global http.sslBackend openssl
```

---

## 8. 上游 CI 与自动构建

仓库里保留了上游两个工作流：

| 工作流 | 触发 | 行为 | 注意 |
|---|---|---|---|
| `.github/workflows/docker-ghcr.yml` | push `main` / `v*` tag / 手动 | 多架构（amd64+arm64）构建并推送到 `ghcr.io/${{ github.repository }}` | 镜像名走 `github.repository`，在本仓库会推到**你自己的** GHCR 命名空间 |
| `.github/workflows/go-binaries.yml` | push `main` / `v*` tag / 手动 | 跑 `go test ./...` + 五平台编译，打 tag 时发 Release | **`go test ./...` 会因 `internal/reqlog` 的既有失败而红**（见 §5），发版前需先处理 |

> 本仓库默认分支是 `feat/anthropic-messages-responses`，因此上面「push `main`」的触发**不会**自动发生；GitHub Actions 也已关闭，避免误触发与误推镜像。要启用：仓库 Settings → Actions → Allow all actions，然后手动 `workflow_dispatch`（或先把默认分支切成 `main`）。启用前建议先修/跳过那个 reqlog 测试。

---

## 9. 安全与合规须知

1. **永远不要提交**：`config.json`（含 `api_key`）、`auths/`（**明文** accessToken / refreshToken）、`data/`（用量、日志、状态）。上游 `.gitignore` 已挡，别绕过。
2. `config.example.json` 的 `api_key` 是 `test_key`，**照抄即等于不设防**；线上必须换成强随机值。
3. 网关默认是**明文 HTTP**，公网暴露请自行加 TLS 终止（nginx / Caddy）或只走内网 / WireGuard。
4. 功能本身只是把请求转换成上游已有的 chat 管线，不引入新的出站目的地；但新端点同样消耗账号额度。
5. 沿用上游的使用声明：**仅限本人账号自用**，禁止批量小号、对外售卖 API、加壳收费分发。

---

## 10. 本仓库的验证记录（2026-10-04）

- `go test ./internal/server/` 全包 **ok**（移植的 17 个用例全 PASS：`TestMessages*` 9 个 + `TestResponses*` 8 个），另加本仓库的 `TestRequestMetricsRecordsAllInboundPaths`。
- **Claude Code v2.1.289 真机实测**（Docker `node:22-bookworm-slim`，`--network host`，经公网 `http://39.105.213.13:7863`）：纯文本 → `result:"正常"`、`is_error:false`；工具往返 → `num_turns=2`，`tool_use`/`tool_result` 闭环通过。
- **官方 `@anthropic-ai/sdk@0.131.0` 五项全通过**：工具强制调用（`stop_reason=tool_use`、`usage.input_tokens=197`，公网请求还命中 `cache_read_input_tokens=128`）、Bearer 文本、流式、`tool_result` 回灌、`count_tokens`（预期 404）。
- **工具 schema 转发保真**：塞 2628 token 的 tools schema → Anthropic 路径 `input_tokens=2628`，与 OpenAI 路径 `prompt_tokens=2628` 完全一致。
- **流式事件序**：`message_start` → `content_block_start(tool_use)` → 4 个 `input_json_delta` → `content_block_stop` → `message_delta(usage)` → `message_stop`。
- **回归检查**：`/v1/chat/completions`、`/panel/`、`/healthz`、账号池调度、排程、用量桶全部无变化。
- 面板运行日志：`/v1/messages`、`/v1/responses`、`/v1/chat/completions` 三条路径均正常归档（修复前前两条整片消失）。

---

## 11. 文件速查

```
ANTHROPIC-PORT.md          ← 本文件（维护说明）
anthropic-port.patch       ← 相对上游 v1.11.11 的代码补丁（9 文件，升级时重放）
Dockerfile.local           ← 本仓库构建用（中国 GOPROXY + HEALTHCHECK）
internal/server/messages.go        ← Anthropic Messages 入站实现
internal/server/responses.go       ← Codex Responses 入站实现
internal/server/messages_test.go   ← 9 个用例
internal/server/responses_test.go  ← 8 个用例
internal/server/reqlog_path_test.go← 归档路径回归测试（本仓库自加）
internal/server/handler.go         ← 路由注册（3 条新路由）+ 归档门槛
internal/server/logging.go         ← requestTrace.path
```
