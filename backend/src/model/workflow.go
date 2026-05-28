package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// JSON 自定义类型，用于存储 JSON 数据
type JSON json.RawMessage

// Scan 实现 sql.Scanner 接口
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal JSON value: %v", value)
	}
	*j = JSON(bytes)
	return nil
}

// Value 实现 driver.Valuer 接口
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return []byte(j), nil
}

// MarshalJSON 实现 json.Marshaler 接口
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return fmt.Errorf("JSON: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// Workflow 工作流定义
type Workflow struct {
	ID            int64          `gorm:"primarykey" json:"id"`
	Name          string         `gorm:"size:255;not null" json:"name"`
	Description   string         `gorm:"type:text" json:"description"`
	Status        int            `gorm:"default:1;comment:1=启用,0=禁用" json:"status"`
	TriggerType   string         `gorm:"size:50;comment:触发类型:schedule,manual,webhook" json:"triggerType"`
	TriggerConfig JSON           `gorm:"type:json;comment:触发配置" json:"triggerConfig"`
	Nodes         JSON           `gorm:"type:json;comment:节点配置" json:"nodes"`
	Edges         JSON           `gorm:"type:json;comment:连线配置" json:"edges"`
	CreatedBy     int64          `gorm:"index" json:"createdBy"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// WorkflowExecution 工作流执行记录
type WorkflowExecution struct {
	ID         int64      `gorm:"primarykey" json:"id"`
	WorkflowID int64      `gorm:"index;not null" json:"workflowId"`
	Status     string     `gorm:"size:20;comment:running,success,failed" json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	ErrorMsg   string     `gorm:"type:text" json:"errorMsg"`
}

// WorkflowNodeExecution 节点执行记录
type WorkflowNodeExecution struct {
	ID          int64      `gorm:"primarykey" json:"id"`
	ExecutionID int64      `gorm:"index;not null" json:"executionId"`
	NodeID      string     `gorm:"size:64;not null" json:"nodeId"`
	Status      string     `gorm:"size:20;comment:running,success,failed" json:"status"`
	Input       JSON       `gorm:"type:json" json:"input"`
	Output      JSON       `gorm:"type:json" json:"output"`
	ErrorMsg    string     `gorm:"type:text" json:"errorMsg"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}
