package model

import (
	"time"
	"gorm.io/gorm"
)

// 用户表
type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Username  string         `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Password  string         `gorm:"size:256;not null" json:"-"`
	Email     string         `gorm:"uniqueIndex;size:128" json:"email"`
	Nickname  string         `gorm:"size:64" json:"nickname"`
	Role      string         `gorm:"size:32;default:viewer" json:"role"` // admin | analyst | viewer
	Status    int8           `gorm:"default:1" json:"status"`            // 1=active 0=disabled
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// 舆情数据来源
type DataSource struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Type        string         `gorm:"size:32;not null" json:"type"` // weibo | weixin | news | forum
	URL         string         `gorm:"size:512" json:"url"`
	Config      string         `gorm:"type:json" json:"config"` // 爬虫/API配置 JSON
	Status      int8           `gorm:"default:1" json:"status"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// 舆情文章/信息
type Article struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	SourceID    uint           `gorm:"index" json:"sourceId"`
	Source      DataSource     `gorm:"foreignKey:SourceID" json:"source,omitempty"`
	Title       string         `gorm:"size:512" json:"title"`
	Content     string         `gorm:"type:longtext" json:"content"`
	Author      string         `gorm:"size:128" json:"author"`
	OriginURL   string         `gorm:"size:1024" json:"originUrl"`
	Platform    string         `gorm:"size:32;index" json:"platform"`
	Sentiment   string         `gorm:"size:16;index" json:"sentiment"` // positive | neutral | negative
	SentScore   float64        `json:"sentScore"`                       // 情感分值 -1~1
	Keywords    string         `gorm:"type:json" json:"keywords"`       // JSON数组（jieba 抽词）
	AITags      *string        `gorm:"column:ai_tags;type:json" json:"aiTags"` // JSON 字符串数组；NULL=未打标
	PublishedAt time.Time      `gorm:"index" json:"publishedAt"`
	// 向量知识库同步：content 变更时需重算 embedding 并重写 Milvus
	EmbeddingContentHash *string    `gorm:"size:64;column:embedding_content_hash" json:"-"`
	EmbeddingSyncedAt     *time.Time `gorm:"column:embedding_synced_at" json:"-"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// 热点话题
type Topic struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"size:256;not null" json:"name"`
	Keywords    string         `gorm:"type:json" json:"keywords"` // JSON数组
	HeatScore   float64        `gorm:"index" json:"heatScore"`
	ArticleCount int           `json:"articleCount"`
	Trend       string         `gorm:"size:16" json:"trend"` // rising | stable | falling
	StartAt     time.Time      `json:"startAt"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// 预警规则
type AlertRule struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Keywords    string         `gorm:"type:json" json:"keywords"`
	Sentiment   string         `gorm:"size:16" json:"sentiment"`    // 触发情感类型
	Threshold   int            `json:"threshold"`                   // 触发数量阈值
	Interval    int            `json:"interval"`                    // 检测间隔(分钟)
	NotifyType  string         `gorm:"size:32" json:"notifyType"`   // email | webhook | sms
	NotifyConf  string         `gorm:"type:json" json:"notifyConf"`
	Status      int8           `gorm:"default:1" json:"status"`
	CreatedBy   uint           `json:"createdBy"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// 预警记录
type AlertRecord struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	RuleID    uint      `gorm:"index" json:"ruleId"`
	Rule      AlertRule `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
	Title     string    `gorm:"size:512" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Status    string    `gorm:"size:16;default:pending" json:"status"` // pending | read
	CreatedAt time.Time `json:"createdAt"`
}

// 爬虫调度（与 Python scheduler 共用表）
type CrawlerSpiderConfig struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	SpiderKey       string    `gorm:"uniqueIndex;size:32;not null" json:"spiderKey"`
	DisplayName     string    `gorm:"size:64" json:"displayName"`
	IntervalMinutes int       `gorm:"not null" json:"intervalMinutes"`
	Enabled         int8      `gorm:"default:1" json:"enabled"` // 1 启用 0 停用
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// 立即执行记录（API 触发的子进程）
// Params 用 longtext：MySQL JSON 类型不可写入空串，基础模式会 Params="" 导致 INSERT 失败
type CrawlerRunLog struct {
	ID          uint       `gorm:"primarykey" json:"id"`
	Spiders     string     `gorm:"size:256" json:"spiders"`
	Mode        string     `gorm:"size:16" json:"mode"` // basic | advanced
	Params      string     `gorm:"type:longtext" json:"params"`           // advanced 过滤条件 JSON；基础模式可为 "{}"
	Status      string     `gorm:"size:16;index" json:"status"`           // running | success | failed
	Message        string     `gorm:"type:text" json:"message"`
	Progress       int        `gorm:"default:0" json:"progress"`
	ProgressDetail string     `gorm:"type:text" json:"progressDetail"`
	TriggeredBy    uint       `json:"triggeredBy"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

// RagSyncLog 向量知识库增量同步任务记录（与 RAG Python 服务写同一表）。
type RagSyncLog struct {
	ID                 uint       `gorm:"primarykey" json:"id"`
	Status             string     `gorm:"size:16;index" json:"status"` // running | success | failed
	Progress           int        `gorm:"default:0" json:"progress"`
	ProgressDetail     string     `gorm:"type:text" json:"progressDetail"`
	Message            string     `gorm:"type:text" json:"message"`
	ArticlesProcessed  int        `json:"articlesProcessed"`
	ChunksUpserted     int        `json:"chunksUpserted"`
	ChunksDeleted      int        `json:"chunksDeleted"`
	Mode               string     `gorm:"size:16" json:"mode"` // scheduled | manual
	StartedAt          time.Time  `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
}

func (RagSyncLog) TableName() string { return "rag_sync_logs" }

// 系统设置：键值对，承载开放注册等运行时开关
type SystemSetting struct {
	Key       string    `gorm:"primarykey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	Desc      string    `gorm:"size:256" json:"desc"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy uint      `json:"updatedBy"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// 审计日志：写操作统一落库
type AuditLog struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	ActorID    uint      `gorm:"index" json:"actorId"`
	ActorName  string    `gorm:"size:64" json:"actorName"`
	Action     string    `gorm:"size:32;index" json:"action"`
	Resource   string    `gorm:"size:64;index" json:"resource"`
	ResourceID string    `gorm:"size:64" json:"resourceId"`
	Method     string    `gorm:"size:8" json:"method"`
	Path       string    `gorm:"size:256" json:"path"`
	Status     int       `json:"status"`
	Payload    string    `gorm:"type:text" json:"payload"`
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:256" json:"userAgent"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// AI 对话会话（按用户隔离，持久化）
type ChatSession struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	UserID    uint           `gorm:"index;not null" json:"userId"`
	Title     string         `gorm:"size:256;not null" json:"title"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// AI 对话消息
type ChatMessage struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	SessionID uint      `gorm:"index;not null" json:"sessionId"`
	Role      string    `gorm:"size:16;not null" json:"role"` // user | assistant
	Content   string    `gorm:"type:longtext;not null" json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// 分析报告
type Report struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Title       string         `gorm:"size:256;not null" json:"title"`
	Type        string         `gorm:"size:32" json:"type"` // daily | weekly | custom
	StartAt     time.Time      `json:"startAt"`
	EndAt       time.Time      `json:"endAt"`
	Content     string         `gorm:"type:longtext" json:"content"` // 富文本报告内容
	CreatedBy   uint           `json:"createdBy"`
	Creator     User           `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
