-- 工作流表
CREATE TABLE IF NOT EXISTS workflows (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL COMMENT '工作流名称',
    description TEXT COMMENT '描述',
    status TINYINT DEFAULT 1 COMMENT '状态: 1=启用, 0=禁用',
    trigger_type VARCHAR(50) COMMENT '触发类型: schedule, manual, webhook',
    trigger_config JSON COMMENT '触发配置',
    nodes JSON COMMENT '节点配置',
    edges JSON COMMENT '连线配置',
    created_by BIGINT COMMENT '创建人ID',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_created_by (created_by),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工作流定义表';

-- 工作流执行记录表
CREATE TABLE IF NOT EXISTS workflow_executions (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    workflow_id BIGINT NOT NULL COMMENT '工作流ID',
    status VARCHAR(20) COMMENT '执行状态: running, success, failed',
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '开始时间',
    finished_at TIMESTAMP NULL COMMENT '结束时间',
    error_msg TEXT COMMENT '错误信息',
    INDEX idx_workflow_id (workflow_id),
    INDEX idx_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工作流执行记录表';

-- 节点执行记录表
CREATE TABLE IF NOT EXISTS workflow_node_executions (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    execution_id BIGINT NOT NULL COMMENT '执行记录ID',
    node_id VARCHAR(64) NOT NULL COMMENT '节点ID',
    status VARCHAR(20) COMMENT '执行状态: running, success, failed',
    input JSON COMMENT '输入数据',
    output JSON COMMENT '输出数据',
    error_msg TEXT COMMENT '错误信息',
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '开始时间',
    finished_at TIMESTAMP NULL COMMENT '结束时间',
    INDEX idx_execution_id (execution_id),
    INDEX idx_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='节点执行记录表';
