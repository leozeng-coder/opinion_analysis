# 工作流系统架构设计文档

## 一、设计原则

### 1. 责任链模式（Chain of Responsibility）
- **数据流向**：爬虫节点 → AI打标节点 → 告警评估节点 → ...
- **核心数据**：`articleIds` - 文章ID列表在节点间传递
- **数据继承**：每个节点的输出会合并上游节点的输入

### 2. 工厂模式（Factory Pattern）
- **节点工厂**：统一管理所有节点类型的注册和创建
- **动态扩展**：新增节点只需实现接口并注册，无需修改核心代码
- **类型安全**：编译时检查节点类型是否存在

## 二、目录结构

```
backend/src/service/workflow/
├── engine.go              # 工作流执行引擎
├── executor.go            # 节点执行器接口定义
├── factory.go             # 节点工厂和注册中心
└── nodes/
    ├── base.go            # 节点基类（通用功能）
    ├── crawler/           # 爬虫类节点
    │   ├── run.go         # 执行爬虫任务
    │   ├── schedule.go    # 定时爬虫
    │   └── status.go      # 查询爬虫状态
    ├── processor/         # 数据处理类节点
    │   ├── ai_tagger.go   # AI情感分析
    │   ├── rag_vectorize.go  # RAG向量化
    │   └── alert_evaluate.go # 告警评估
    └── control/           # 控制流节点
        ├── condition.go   # 条件判断
        └── delay.go       # 延迟执行
```

## 三、核心接口

### NodeExecutor 接口

```go
type NodeExecutor interface {
    // 返回节点类型标识（如 "crawler_run"）
    Type() string
    
    // 验证节点配置是否正确
    Validate(config map[string]interface{}) error
    
    // 执行节点逻辑
    // input: 上游节点的输出
    // output: 本节点的输出（传递给下游）
    Execute(ctx context.Context, config, input map[string]interface{}) (output map[string]interface{}, err error)
}
```

## 四、数据传递机制

### 责任链数据流

```
爬虫节点输出:
{
  "articleIds": [1, 2, 3, 4, 5],
  "platforms": ["xiaohongshu"],
  "articlesCount": 5
}
    ↓
AI打标节点接收:
{
  "articleIds": [1, 2, 3, 4, 5],  // 从上游继承
  "platforms": ["xiaohongshu"],    // 从上游继承
  "articlesCount": 5               // 从上游继承
}
    ↓
AI打标节点输出:
{
  "articleIds": [1, 2, 3, 4, 5],  // 继承
  "platforms": ["xiaohongshu"],    // 继承
  "articlesCount": 5,              // 继承
  "taggedCount": 5,                // 新增
  "success": true                  // 新增
}
    ↓
告警评估节点接收:
{
  "articleIds": [1, 2, 3, 4, 5],  // 从上游继承
  "taggedCount": 5,                // 从上游继承
  ...
}
```

### 核心字段说明

- **articleIds** ([]int64): 本次工作流处理的文章ID列表
  - 由爬虫节点生成
  - 在责任链中传递
  - 后续节点只处理这些文章

- **articlesCount** (int): 文章数量
- **taggedCount** (int): 打标数量
- **alertCount** (int): 告警数量

## 五、节点实现规范

### 1. 继承 BaseNode

```go
type MyNode struct {
    *nodes.BaseNode
    // 依赖的服务
}

func NewMyNode(deps...) *MyNode {
    return &MyNode{
        BaseNode: nodes.NewBaseNode("my_node"),
        // 初始化依赖
    }
}
```

### 2. 实现接口方法

```go
// Validate 验证配置
func (n *MyNode) Validate(config map[string]interface{}) error {
    // 使用 BaseNode 提供的辅助方法
    return n.ValidateRequired(config, "requiredField")
}

// Execute 执行逻辑
func (n *MyNode) Execute(ctx context.Context, config, input map[string]interface{}) (map[string]interface{}, error) {
    // 1. 获取配置参数
    param := n.GetString(config, "param", "default")
    
    // 2. 获取上游数据
    articleIds := n.GetArticleIDs(input)
    
    // 3. 执行业务逻辑
    result, err := n.doSomething(ctx, articleIds)
    if err != nil {
        return nil, n.WrapError("operation failed", err)
    }
    
    // 4. 构造输出（继承输入 + 新增字段）
    output := n.MergeOutput(input, map[string]interface{}{
        "myResult": result,
    })
    
    return output, nil
}
```

### 3. 注册节点

```go
// 在 engine.go 的 registerNodes 方法中
func (e *Engine) registerNodes() {
    MustRegisterNode(NewMyNode(deps...))
}
```

## 六、错误处理规范

### 1. 配置验证错误

```go
if field == "" {
    return &ValidationError{
        Field: "fieldName",
        Message: "field is required",
    }
}
```

### 2. 执行错误

```go
if err != nil {
    return nil, &ExecutionError{
        NodeType: n.Type(),
        NodeID: "node_xxx",
        Message: "operation failed",
        Cause: err,
    }
}
```

### 3. 使用 BaseNode 辅助方法

```go
return nil, n.WrapError("database query failed", err)
```

## 七、扩展新节点步骤

### 1. 创建节点文件

在对应的分类目录下创建文件：
- 爬虫类 → `nodes/crawler/`
- 处理类 → `nodes/processor/`
- 控制类 → `nodes/control/`

### 2. 实现节点

```go
package processor

import (
    "context"
    "opinion-analysis/src/service/workflow/nodes"
)

type MyProcessorNode struct {
    *nodes.BaseNode
    // 依赖
}

func NewMyProcessorNode(deps...) *MyProcessorNode {
    return &MyProcessorNode{
        BaseNode: nodes.NewBaseNode("my_processor"),
    }
}

func (n *MyProcessorNode) Validate(config map[string]interface{}) error {
    // 验证逻辑
    return nil
}

func (n *MyProcessorNode) Execute(ctx context.Context, config, input map[string]interface{}) (map[string]interface{}, error) {
    // 执行逻辑
    articleIds := n.GetArticleIDs(input)
    
    // 处理文章...
    
    return n.MergeOutput(input, map[string]interface{
        "processedCount": len(articleIds),
    }), nil
}
```

### 3. 注册节点

在 `engine.go` 中注册：

```go
func (e *Engine) registerNodes() {
    // ...
    MustRegisterNode(processor.NewMyProcessorNode(deps...))
}
```

### 4. 前端配置

在 `WorkflowEditorPage.tsx` 中添加节点定义：

```typescript
my_processor: {
  label: '我的处理器',
  description: '处理文章数据',
  color: '#52c41a',
  icon: '⚙️',
  configSchema: [
    { name: 'param1', label: '参数1', type: 'text', required: true },
  ],
}
```

## 八、优势总结

### 1. 可维护性
- 清晰的目录结构
- 统一的接口规范
- 完善的错误处理

### 2. 可扩展性
- 工厂模式支持动态注册
- 新增节点无需修改核心代码
- 责任链模式支持灵活组合

### 3. 健壮性
- 类型安全的节点注册
- 完整的错误信息传递
- 详细的日志记录

### 4. 数据一致性
- 文章ID在责任链中传递
- 确保所有节点处理同一批数据
- 避免数据不一致问题
