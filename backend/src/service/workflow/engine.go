package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/tagger"
	controlNodes "opinion-analysis/src/service/workflow/nodes/control"
	crawlerNodes "opinion-analysis/src/service/workflow/nodes/crawler"
	processorNodes "opinion-analysis/src/service/workflow/nodes/processor"
)

// Engine 工作流执行引擎
type Engine struct {
	db     *gorm.DB
	store  *repository.Store
	logger *zap.Logger

	// 依赖的服务
	taggerSvc   *tagger.Service
	ragProc     *ragprocess.Manager
	alertEngine *alertengine.Engine
}

// NewEngine 创建工作流引擎
func NewEngine(
	db *gorm.DB,
	store *repository.Store,
	logger *zap.Logger,
	taggerSvc *tagger.Service,
	ragProc *ragprocess.Manager,
	alertEngine *alertengine.Engine,
) *Engine {
	engine := &Engine{
		db:          db,
		store:       store,
		logger:      logger,
		taggerSvc:   taggerSvc,
		ragProc:     ragProc,
		alertEngine: alertEngine,
	}

	// 注册所有节点
	engine.registerNodes()

	return engine
}

// registerNodes 注册所有节点执行器
func (e *Engine) registerNodes() {
	// 注册爬虫类节点
	MustRegisterNode(crawlerNodes.NewRunNode(e.store.Crawler))

	// 注册处理类节点
	MustRegisterNode(processorNodes.NewPlatformSyncNode(e.db))
	MustRegisterNode(processorNodes.NewAITaggerNode(e.taggerSvc))
	MustRegisterNode(processorNodes.NewRAGVectorizeNode(e.ragProc))
	MustRegisterNode(processorNodes.NewAlertEvaluateNode(e.alertEngine))

	// 注册控制流节点
	MustRegisterNode(controlNodes.NewDelayNode())
	MustRegisterNode(controlNodes.NewConditionNode())
}

// Execute 执行工作流
func (e *Engine) Execute(ctx context.Context, workflowID int64, manualInput map[string]interface{}) (*model.WorkflowExecution, error) {
	// 1. 加载工作流定义
	workflow, err := e.store.Workflow.FindByID(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow: %w", err)
	}

	e.logger.Info("starting workflow execution",
		zap.Int64("workflowId", workflowID),
		zap.String("workflowName", workflow.Name))

	// 2. 查找最近创建的 running 状态的执行记录（由 Handler 创建）
	// 如果找不到，则创建新的执行记录
	execution, err := e.store.WorkflowExecution.FindLatestRunning(workflowID)
	if err != nil || execution == nil {
		execution = &model.WorkflowExecution{
			WorkflowID: workflowID,
			Status:     "running",
			StartedAt:  time.Now(),
		}
		if err := e.store.WorkflowExecution.Create(execution); err != nil {
			return nil, fmt.Errorf("failed to create execution record: %w", err)
		}
	}

	// 3. 解析节点和边
	var nodeList []map[string]interface{}
	var edgeList []map[string]interface{}

	if err := json.Unmarshal(workflow.Nodes, &nodeList); err != nil {
		e.updateExecutionStatus(execution.ID, "failed", "failed to parse nodes: "+err.Error())
		return nil, fmt.Errorf("failed to parse nodes: %w", err)
	}

	if err := json.Unmarshal(workflow.Edges, &edgeList); err != nil {
		e.updateExecutionStatus(execution.ID, "failed", "failed to parse edges: "+err.Error())
		return nil, fmt.Errorf("failed to parse edges: %w", err)
	}

	// 4. 拓扑排序（确定执行顺序）
	sortedNodes, err := e.topologicalSort(nodeList, edgeList)
	if err != nil {
		e.updateExecutionStatus(execution.ID, "failed", err.Error())
		return nil, err
	}

	// 5. 按顺序执行节点
	nodeOutputs := make(map[string]map[string]interface{})
	e.logger.Info("starting node execution",
		zap.Int("totalNodes", len(sortedNodes)))

	for _, node := range sortedNodes {
		nodeID := node["id"].(string)
		nodeType := e.resolveNodeType(node)

		e.logger.Info("processing node",
			zap.String("nodeId", nodeID),
			zap.String("nodeType", nodeType))

		// 跳过 trigger 节点（trigger 只是起点标记）
		if nodeType == "trigger" {
			e.logger.Info("skipping trigger node", zap.String("nodeId", nodeID))
			nodeOutputs[nodeID] = manualInput
			continue
		}

		// 获取上游节点的输出作为输入
		input := e.collectInputs(nodeID, edgeList, nodeOutputs)

		e.logger.Info("executing node",
			zap.String("nodeId", nodeID),
			zap.String("nodeType", nodeType),
			zap.Any("input", input),
			zap.Int("inputKeys", len(input)))

		// 执行节点
		output, err := e.executeNode(ctx, execution.ID, node, input)
		if err != nil {
			e.logger.Error("node execution failed",
				zap.String("nodeId", nodeID),
				zap.Error(err))
			e.updateExecutionStatus(execution.ID, "failed", err.Error())
			return execution, err
		}

		nodeOutputs[nodeID] = output
		e.logger.Info("node execution succeeded",
			zap.String("nodeId", nodeID),
			zap.Any("output", output),
			zap.Int("outputKeys", len(output)))
	}

	// 6. 更新执行状态
	e.updateExecutionStatus(execution.ID, "success", "")

	e.logger.Info("workflow execution completed",
		zap.Int64("workflowId", workflowID),
		zap.Int64("executionId", execution.ID))

	return execution, nil
}

// executeNode 执行单个节点
func (e *Engine) executeNode(ctx context.Context, executionID int64, node map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	nodeID := node["id"].(string)
	nodeType := e.resolveNodeType(node)
	config, _ := node["config"].(map[string]interface{})

	e.logger.Info("executing node",
		zap.String("nodeId", nodeID),
		zap.String("nodeType", nodeType))

	// 创建节点执行记录
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	nodeExec := &model.WorkflowNodeExecution{
		ExecutionID: executionID,
		NodeID:      nodeID,
		Status:      "running",
		Input:       model.JSON(inputJSON),
		StartedAt:   time.Now(),
	}
	if err := e.store.WorkflowNodeExecution.Create(nodeExec); err != nil {
		return nil, fmt.Errorf("failed to create node execution record: %w", err)
	}

	// 获取节点执行器
	executor, err := GetNodeExecutor(nodeType)
	if err != nil {
		errorMsg := fmt.Sprintf("Node type '%s' not registered", nodeType)
		e.updateNodeExecutionStatus(nodeExec.ID, "failed", errorMsg, nil)
		return nil, fmt.Errorf("[Node: %s, Type: %s] %s", nodeID, nodeType, errorMsg)
	}

	// 验证配置
	if err := executor.Validate(config); err != nil {
		errorMsg := fmt.Sprintf("Invalid config: %s", err.Error())
		e.updateNodeExecutionStatus(nodeExec.ID, "failed", errorMsg, nil)
		return nil, fmt.Errorf("[Node: %s, Type: %s] %s", nodeID, nodeType, errorMsg)
	}

	// 执行节点
	output, err := executor.Execute(ctx, config, input)
	if err != nil {
		errorMsg := err.Error()
		e.updateNodeExecutionStatus(nodeExec.ID, "failed", errorMsg, nil)
		return nil, fmt.Errorf("[Node: %s, Type: %s] Execution failed: %w", nodeID, nodeType, err)
	}

	// 持久化时只记录相对 input 的增量，避免重复字段把日志撑大；
	// 下游节点拿到的仍然是完整 output。
	outputDelta := diffPayload(input, output)
	e.updateNodeExecutionStatus(nodeExec.ID, "success", "", outputDelta)

	e.logger.Info("node execution completed",
		zap.String("nodeId", nodeID),
		zap.String("nodeType", nodeType))

	return output, nil
}

// topologicalSort 拓扑排序
func (e *Engine) topologicalSort(nodeList []map[string]interface{}, edgeList []map[string]interface{}) ([]map[string]interface{}, error) {
	// 构建邻接表和入度表
	adjList := make(map[string][]string)
	inDegree := make(map[string]int)
	nodeMap := make(map[string]map[string]interface{})

	for _, node := range nodeList {
		nodeID := node["id"].(string)
		nodeMap[nodeID] = node
		inDegree[nodeID] = 0
	}

	for _, edge := range edgeList {
		source := edge["source"].(string)
		target := edge["target"].(string)
		adjList[source] = append(adjList[source], target)
		inDegree[target]++
	}

	// Kahn算法进行拓扑排序
	queue := []string{}
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	sorted := []map[string]interface{}{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nodeMap[current])

		for _, neighbor := range adjList[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// 检测循环依赖
	if len(sorted) != len(nodeList) {
		return nil, fmt.Errorf("workflow contains circular dependency")
	}

	return sorted, nil
}

// collectInputs 收集上游节点的输出
func (e *Engine) collectInputs(nodeID string, edgeList []map[string]interface{}, outputs map[string]map[string]interface{}) map[string]interface{} {
	input := make(map[string]interface{})

	for _, edge := range edgeList {
		if edge["target"].(string) == nodeID {
			sourceID := edge["source"].(string)
			if output, ok := outputs[sourceID]; ok {
				// 合并上游输出
				for k, v := range output {
					input[k] = v
				}
			}
		}
	}

	return input
}

// diffPayload 返回 output 相对 input 的增量字段（新增或值发生变化）。
// 用于日志持久化，避免把上游字段重复存一遍。
func diffPayload(input, output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return map[string]interface{}{}
	}
	if input == nil {
		return output
	}
	delta := make(map[string]interface{}, len(output))
	for k, v := range output {
		if iv, ok := input[k]; !ok || !reflect.DeepEqual(iv, v) {
			delta[k] = v
		}
	}
	return delta
}

func (e *Engine) resolveNodeType(node map[string]interface{}) string {
	if t, ok := node["type"].(string); ok && t != "" && t != "custom" && t != "trigger" {
		return t
	}
	if st, ok := node["subType"].(string); ok && st != "" {
		return st
	}
	if t, ok := node["type"].(string); ok {
		return t
	}
	return ""
}

// updateExecutionStatus 更新执行状态
func (e *Engine) updateExecutionStatus(executionID int64, status, errorMsg string) {
	now := time.Now()
	execution := &model.WorkflowExecution{
		ID:         executionID,
		Status:     status,
		FinishedAt: &now,
		ErrorMsg:   errorMsg,
	}
	if err := e.store.WorkflowExecution.Update(execution); err != nil {
		e.logger.Error("failed to update execution status",
			zap.Int64("executionId", executionID),
			zap.String("status", status),
			zap.Error(err))
	} else {
		e.logger.Info("execution status updated",
			zap.Int64("executionId", executionID),
			zap.String("status", status))
	}
}

// updateNodeExecutionStatus 更新节点执行状态
func (e *Engine) updateNodeExecutionStatus(nodeExecID int64, status, errorMsg string, output map[string]interface{}) {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		e.logger.Error("failed to marshal output",
			zap.Int64("nodeExecId", nodeExecID),
			zap.Error(err))
		// 使用空对象作为fallback
		outputJSON = []byte("{}")
	}
	now := time.Now()
	nodeExec := &model.WorkflowNodeExecution{
		ID:         nodeExecID,
		Status:     status,
		Output:     model.JSON(outputJSON),
		ErrorMsg:   errorMsg,
		FinishedAt: &now,
	}
	if err := e.store.WorkflowNodeExecution.Update(nodeExec); err != nil {
		e.logger.Error("failed to update node execution status",
			zap.Int64("nodeExecId", nodeExecID),
			zap.String("status", status),
			zap.Error(err))
	}
}
