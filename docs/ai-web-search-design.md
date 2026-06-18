# 智能助手工具调用（Function Calling）与联网搜索 设计文档

> 更新日期：2026-06-17
> 范围：`backend/src/service/tagger/` 深度思考 pipeline + `backend/src/service/tagger/pipeline/` 节点 + 新增 `pipeline/tools/` 工具层 + `system_settings` 动态配置 + `frontend-admin/` 配置前端
> 状态：设计阶段（未开始实现）

---

## 一、目标与背景

把「深度思考」模式的检索环节，从**固定调用本地检索**升级为**工具调用（function calling）架构**：由 LLM 自主决定调用哪个工具（本地知识库检索 / 联网搜索 / 未来扩展），从而既获得联网能力，又得到一个**可持续扩展工具的优雅框架**。

### 现状的本质问题

深度思考走 ReAct pipeline（`service/tagger/deep_think.go`）：

```
IntentNode      意图分析：是否需检索、拆子查询、评估 topK
ReActNode       推理↔检索循环：本地 Milvus 检索 → 打分 → 评估充分性 → 补充检索
SynthesizeNode  综合全部证据，提炼关键洞察
GenerateNode    流式生成最终回答
```

现有 ReActNode 看起来像 agent，但**它不是真正的工具调用**：

- LLM 从不做「选哪个工具」的决策，行动空间硬编码为单一动作——本地 Milvus 检索（`RAGSearchFn`）；
- 所谓"推理"是 prompt 让 LLM 吐 JSON（`next_queries` / `sufficient`），Go 代码解析后**固定调本地检索**；
- 加一个新数据源（如联网）需要侵入式改 `react.go` 的去重（`extractArticleID`）和打分映射（`scoreChunks` 按序号）逻辑。

要"优雅且可扩展"，就得引入**真正的工具抽象 + 让 LLM 自主选工具的 agent 循环**。

---

## 二、核心设计决策

### 决策 1：骨架 = Agent 循环替换 ReActNode，保留 Intent / Synthesize / Generate

新 pipeline：

```
IntentNode      意图分析（保留，轻量；不再决定"调什么"，只做问题理解 + 提示）
AgentNode       工具调用循环（替换 ReActNode）★核心新增
                  LLM 看到工具清单 → 决定调用哪些工具（可多轮、可并行）
                  → 执行工具 → 把结果回灌 → LLM 再决策 → 直到不再调用工具
SynthesizeNode  综合全部工具产出，提炼关键洞察（保留）
GenerateNode    流式生成最终回答（保留）
```

**原因**：

- ReActNode 的"伪 agent"编排被真正的 function calling 取代，但 Intent（轻量意图理解）、Synthesize（综合洞察）、Generate（流式作答）是与"用什么工具"正交的能力，且已打磨出良好产品体验（思考卡片、用户需求洞察、流式输出），予以保留。
- AgentNode 输出的工具结果（本地证据 + web 结果）统一沉淀进 `state.Observations` / `state.WebResults`，Synthesize 和 Generate 几乎不用改。

### 决策 2：原生 function calling 优先 + prompt/JSON 兑底

LLM 调用层抽象出"工具选择"能力：

- **默认**走 OpenAI 兼容的原生 `tools` / `tool_calls` 协议（DeepSeek 支持）；
- 若所配模型不支持或返回异常，**自动降级**为"把工具清单写进 prompt、让 LLM 输出 JSON 指明调用哪个工具"——即复用现有项目"prompt + JSON 解析"的成熟风格。

**原因**：原生 function calling 最优雅，但完全依赖模型支持质量；保留 JSON 兑底，换模型（如换到不支持 tools 的本地模型）时不至于功能崩溃，与项目现有容错风格一致。

### 决策 3：工具抽象为可注册接口（Tool Registry）

定义统一 `Tool` 接口 + 注册表。新增工具 = 实现接口 + 注册一行，**不动 AgentNode**。首期注册两个工具：


| 工具 name                  | 说明                   | 底层                  |
| ------------------------ | -------------------- | ------------------- |
| `search_local_knowledge` | 检索本地舆情知识库（文章正文 + 评论） | `rag.Client.Search` |
| `web_search`             | 联网搜索实时/外部信息          | 博查 Bocha API        |


未来扩展（计算、查指定平台数据、查时间范围统计等）只需新增 Tool 实现。

### 决策 4：联网搜索服务商 = 博查 Bocha

国内 LLM 搜索 API，中文结果质量好、合规、可返回正文摘要、无需翻墙，贴合中文舆情场景。

### 决策 5：第三方 Key 存 DB；工具失败降级不阻断

- 博查 API Key 存 `system_settings` 表、管理后台配置、脱敏回显（沿用项目惯例，见 [crawler-upgrade-status.md](crawler-upgrade-status.md)）。
- 任一工具执行失败 → 把错误作为工具结果回灌给 LLM（让它换工具或基于已有信息作答），不中断 pipeline。
- `web_search` 工具仅在配置了 key 且开关开启时才注册进工具清单；否则 LLM 看不到该工具，行为自动退回"仅本地检索"，向后兼容。

---

## 三、工具层设计（新增 `pipeline/tools/`）

### Tool 接口

```go
// pipeline/tools/tool.go
package tools

// Tool 是 agent 可调用的一个工具。新增工具只需实现此接口并注册。
type Tool interface {
    Name() string                 // 唯一标识，如 "web_search"
    Description() string          // 给 LLM 看的用途描述（决定 LLM 何时调用）
    Parameters() map[string]any   // JSON Schema，描述入参
    Invoke(ctx context.Context, args json.RawMessage) (Result, error)
}

// Result 是工具执行结果，统一回灌给 LLM 并可沉淀进 PipelineState。
type Result struct {
    // 给 LLM 看的文本结果（回灌进 tool 消息）
    Content string
    // 结构化沉淀：本地检索产出的 observations / web 产出的结果，供 Synthesize/Generate 复用
    Observations []Observation // 复用 pipeline.Observation 语义
    WebResults   []WebResult
}

// Registry 持有所有可用工具，按 name 索引。
type Registry struct{ tools map[string]Tool }
func (r *Registry) Register(t Tool)
func (r *Registry) Specs() []ToolSpec        // 转成 LLM tools 数组 / 或 prompt 清单
func (r *Registry) Invoke(ctx, name, args) (Result, error)
```

### 两个首期工具

```go
// pipeline/tools/local_search.go — 包装 RAGSearchFn
//   入参: {"query": string, "top_k": int, "topics": []string}
//   内部复用现有打分/去重逻辑，产出 []Observation

// pipeline/tools/web_search.go — 包装 WebSearchFn（博查）
//   入参: {"query": string, "count": int, "freshness": string}
//   产出 []WebResult（标题/URL/摘要/站点/时间）
```

> 本地检索工具内部仍可复用现有 `scoreChunks` 打分与 `extractArticleID` 去重——它们被收进工具内部，不再散落在循环里。

---

## 四、Agent 循环节点（替换 ReActNode）

```go
// pipeline/agent.go
type AgentNode struct {
    registry *tools.Registry
    llm      ToolCallFn   // 支持 tools 的 LLM 调用（原生优先 + JSON 兑底）
    maxSteps int          // 防止无限循环，默认 5
}
```

执行流程：

```
1. 组装 messages：system(助手设定 + 工具使用指引) + 历史 + 用户问题
2. 循环（最多 maxSteps 步）：
   a. 调 LLM，传入 registry.Specs() 工具清单
   b. LLM 返回 tool_calls？
        是 → 逐个/并行 Invoke 工具
             → 结果 emit 成 ThinkStep（"调用 web_search：找到 N 条…"）
             → 结果沉淀进 state.Observations / state.WebResults
             → 把 tool 结果作为 role=tool 消息追加，回到 a
        否 → 退出循环（LLM 认为信息已足够）
3. 工具产出已在 state 中，交给 SynthesizeNode
```

每一步的工具调用都通过现有 `emit(ThinkStep{...})` 推到前端思考链——**前端 SSE 协议无需改动**，用户能看到"决定联网搜索 → 调用 web_search → 找到 5 条"的过程。

### LLM 调用层抽象

```go
// ToolCallFn 在现有 LLMCallFn 基础上扩展工具能力。
type ToolCallFn func(ctx context.Context, msgs []Message, specs []ToolSpec) (ToolCallResp, error)

type ToolCallResp struct {
    Content   string      // LLM 的文本（无工具调用时即最终意图）
    ToolCalls []ToolCall  // 原生 tool_calls；JSON 兑底模式下由解析得到
}
```

实现位于 `service/tagger/`，复用现有 HTTP client；请求体在现有 `model/messages/...` 基础上加 `tools` / `tool_choice` 字段。兑底模式不传 `tools`，改在 system prompt 注入工具清单并要求输出指定 JSON，解析复用现有 `ParseIntent` 风格的容错解析。

---

## 五、改动清单


| 文件                               | 改动                                                                                | 类型     |
| -------------------------------- | --------------------------------------------------------------------------------- | ------ |
| `pipeline/tools/tool.go`         | `Tool` 接口、`Result`、`Registry`、`ToolSpec`                                          | **新增** |
| `pipeline/tools/local_search.go` | 本地知识库检索工具（包装 RAGSearchFn，内含打分/去重）                                                 | **新增** |
| `pipeline/tools/web_search.go`   | 联网搜索工具（包装博查 WebSearchFn）                                                          | **新增** |
| `pipeline/agent.go`              | `AgentNode`：工具调用循环，替换 ReActNode                                                   | **新增** |
| `pipeline/node.go`               | `PipelineState` 加 `WebResults`；定义 `WebResult`、`Message`、`ToolSpec`、`ToolCall` 等类型 | 改      |
| `pipeline/intent.go`             | 意图节点保留，去掉"决定检索策略"职责，专注问题理解（可选精简 prompt）                                           | 改      |
| `pipeline/reasoning.go`          | `buildDeepContext` 增加【联网搜索结果】块，标注 URL/站点/时间来源                                     | 改      |
| `service/tagger/toolcall.go`     | `ToolCallFn` 实现：原生 tools 优先 + JSON 兑底                                             | **新增** |
| `service/tagger/bocha.go`        | 博查 HTTP client，构造 `WebSearchFn`                                                   | **新增** |
| `service/tagger/deep_think.go`   | 构建 Registry、注册工具、用 AgentNode 替换 ReActNode                                         | 改      |
| `config/config.go`               | `TaggerConfig` 加 `WebSearchEnabled` / `WebSearchApiKey` / `WebSearchCount`        | 改      |
| `service/tagger/settings.go`     | `SettingKeys` / `applySetting` / `SaveConfig` 同步新字段                               | 改      |
| `api/handler/admin/system.go`    | system settings 接口暴露联网搜索配置（key 脱敏）                                                | 改      |
| `frontend-admin/` LLM 配置页        | 增加"联网搜索"开关 + API Key 输入                                                           | 改      |
| `react.go` / `react_test.go`     | ReActNode 被 AgentNode 取代后退役（删除或保留备用，见风险节）                                         | 删/留    |


---

## 六、博查 API 对接（待实测确认）


| 项        | 值                                                                                                 |
| -------- | ------------------------------------------------------------------------------------------------- |
| Endpoint | `POST https://api.bochaai.com/v1/web-search`                                                      |
| 认证       | `Authorization: Bearer <API_KEY>`                                                                 |
| 请求体      | `query`、`count`、`summary`、`freshness`                                                             |
| 响应       | `data.webPages.value[]`，每项含 `name` / `url` / `snippet` / `summary` / `siteName` / `datePublished` |


> ⚠️ 字段依据博查通用接口写出，**实现前需用真实 API Key 实跑核对**，再定 client 解析。

### 动态配置项（system_settings，key 前缀 `tagger.`）


| 配置项                  | 说明                   | 默认    |
| -------------------- | -------------------- | ----- |
| `web_search_enabled` | 联网搜索工具总开关（关闭则不注册该工具） | false |
| `web_search_api_key` | 博查 API Key（敏感，脱敏回显）  | 空     |
| `web_search_count`   | 单次返回结果数              | 5     |


---

## 七、实施步骤

1. **工具层骨架**：`tools/tool.go` 定义接口 + Registry；`local_search.go` 把现有 RAG 检索（含打分/去重）收进工具，先让 AgentNode 只挂这一个工具跑通，对齐现有效果。
2. **LLM 工具调用层**：`toolcall.go` 实现原生 tools 调用；先验证 DeepSeek 原生 function calling，再补 JSON 兑底分支。
3. **Agent 循环**：`agent.go` 写循环 + ThinkStep 事件；`deep_think.go` 用 AgentNode 替换 ReActNode，跑回归确认体验不退化。
4. **联网工具**：`bocha.go`（用真实 key 实测字段）+ `web_search.go` 注册进 Registry；`reasoning.go` 注入 web 结果。
5. **配置后台**：`config.go` / `settings.go` / system 接口 / admin 前端配置项。
6. **自查**：`go build ./...` + `go vet`；分别在 web 开关开/关两种状态回归，确认关闭时行为与现状一致。

---

## 八、风险与注意

- **DeepSeek function calling 质量**：原生 tools 的稳定性需实测（多工具选择、并行调用、流式下的 tool_calls 解析）。这是决策 2 保留 JSON 兑底的根本原因。
- **Agent 循环成本与失控**：每步都是一次 LLM 调用 + 可能的外网请求。用 `maxSteps` 上限 + 工具失败降级兜底，避免循环失控和费用飙升。
- **ReActNode 退役**：现有 `react.go` 的渐进式扩容、打分映射是已验证逻辑。建议先让 AgentNode + local_search 工具对齐其检索效果后再删除 `react.go` / `react_test.go`，过渡期可保留备用。
- **外网依赖 + 第三方 Key**：超时、限流、降级；新增敏感 key 存 DB 脱敏（决策 5）。
- **范围限定**：本期只改深度思考模式；普通单轮 `/ai/chat`、`/ai/sessions/chat` 不动。
- **流式与工具调用的协同**：AgentNode 内部的工具决策轮用非流式调用，最终作答仍由 GenerateNode 流式输出，保持现有打字机体验。

