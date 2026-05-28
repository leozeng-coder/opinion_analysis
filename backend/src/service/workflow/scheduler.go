package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"opinion-analysis/src/repository"
)

// Scheduler 工作流调度器
type Scheduler struct {
	store  *repository.Store
	engine *Engine
	cron   *cron.Cron
	logger *zap.Logger
}

// NewScheduler 创建调度器
func NewScheduler(store *repository.Store, engine *Engine, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		store:  store,
		engine: engine,
		cron:   cron.New(),
		logger: logger,
	}
}

// Start 启动调度器
func (s *Scheduler) Start() {
	// 每分钟扫描一次需要执行的工作流
	s.cron.AddFunc("@every 1m", func() {
		s.scanAndExecute()
	})
	s.cron.Start()
	s.logger.Info("workflow scheduler started")
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info("workflow scheduler stopped")
}

// scanAndExecute 扫描并执行工作流
func (s *Scheduler) scanAndExecute() {
	// 查询所有启用的定时工作流
	workflows, err := s.store.Workflow.FindActiveScheduledWorkflows()
	if err != nil {
		s.logger.Error("failed to scan workflows", zap.Error(err))
		return
	}

	now := time.Now()
	for _, wf := range workflows {
		// 解析cron表达式
		var triggerConfig map[string]interface{}
		if err := json.Unmarshal(wf.TriggerConfig, &triggerConfig); err != nil {
			s.logger.Error("failed to parse trigger config",
				zap.Int64("workflowId", wf.ID),
				zap.Error(err))
			continue
		}

		cronExpr, ok := triggerConfig["cron"].(string)
		if !ok || cronExpr == "" {
			s.logger.Warn("invalid cron expression",
				zap.Int64("workflowId", wf.ID))
			continue
		}

		// 判断是否应该执行
		schedule, err := cron.ParseStandard(cronExpr)
		if err != nil {
			s.logger.Error("failed to parse cron expression",
				zap.Int64("workflowId", wf.ID),
				zap.String("cron", cronExpr),
				zap.Error(err))
			continue
		}

		// 计算上次更新时间之后的下一次执行时间
		next := schedule.Next(wf.UpdatedAt)

		// 如果下一次执行时间已经过去，则执行工作流
		if next.Before(now) {
			s.logger.Info("triggering scheduled workflow",
				zap.Int64("workflowId", wf.ID),
				zap.String("workflowName", wf.Name),
				zap.String("cron", cronExpr))

			// 异步执行工作流
			go func(workflowID int64, workflowName string) {
				ctx := context.Background()
				_, err := s.engine.Execute(ctx, workflowID, nil)
				if err != nil {
					s.logger.Error("workflow execution failed",
						zap.Int64("workflowId", workflowID),
						zap.String("workflowName", workflowName),
						zap.Error(err))
				}
			}(wf.ID, wf.Name)
		}
	}
}
