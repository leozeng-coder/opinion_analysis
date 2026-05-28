package workflow

import (
	"fmt"
	"sync"
)

// NodeFactory 节点工厂
type NodeFactory struct {
	mu        sync.RWMutex
	executors map[string]NodeExecutor
}

// NewNodeFactory 创建节点工厂
func NewNodeFactory() *NodeFactory {
	return &NodeFactory{
		executors: make(map[string]NodeExecutor),
	}
}

// Register 注册节点执行器
func (f *NodeFactory) Register(executor NodeExecutor) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	nodeType := executor.Type()
	if nodeType == "" {
		return fmt.Errorf("node type cannot be empty")
	}

	if _, exists := f.executors[nodeType]; exists {
		return fmt.Errorf("node type %s already registered", nodeType)
	}

	f.executors[nodeType] = executor
	return nil
}

// Get 获取节点执行器
func (f *NodeFactory) Get(nodeType string) (NodeExecutor, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	executor, exists := f.executors[nodeType]
	if !exists {
		return nil, fmt.Errorf("node type %s not registered", nodeType)
	}

	return executor, nil
}

// List 列出所有已注册的节点类型
func (f *NodeFactory) List() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.executors))
	for nodeType := range f.executors {
		types = append(types, nodeType)
	}
	return types
}

// MustRegister 注册节点执行器，失败则 panic
func (f *NodeFactory) MustRegister(executor NodeExecutor) {
	if err := f.Register(executor); err != nil {
		panic(err)
	}
}

// 全局节点工厂实例
var globalFactory = NewNodeFactory()

// RegisterNode 注册节点到全局工厂
func RegisterNode(executor NodeExecutor) error {
	return globalFactory.Register(executor)
}

// MustRegisterNode 注册节点到全局工厂，失败则 panic
func MustRegisterNode(executor NodeExecutor) {
	globalFactory.MustRegister(executor)
}

// GetNodeExecutor 从全局工厂获取节点执行器
func GetNodeExecutor(nodeType string) (NodeExecutor, error) {
	return globalFactory.Get(nodeType)
}

// ListNodeTypes 列出所有已注册的节点类型
func ListNodeTypes() []string {
	return globalFactory.List()
}
