package model

import "time"

// PlatformDataQuery 平台数据查询参数
type PlatformDataQuery struct {
	Platform  string `form:"platform"`
	StartDate string `form:"startDate"`
	EndDate   string `form:"endDate"`
	Page      int    `form:"page" binding:"required,min=1"`
	PageSize  int    `form:"pageSize" binding:"required,min=1,max=100"`
}

// PlatformDataItem 平台数据项（统一格式）
type PlatformDataItem struct {
	ID           uint       `json:"id"`
	Platform     string     `json:"platform"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	Author       string     `json:"author"`
	Avatar       string     `json:"avatar,omitempty"`
	URL          string     `json:"url"`
	PublishTime  *time.Time `json:"publishTime"`
	LikeCount    *int       `json:"likeCount,omitempty"`
	CommentCount *int       `json:"commentCount,omitempty"`
	ShareCount   *int       `json:"shareCount,omitempty"`
	ViewCount    *int       `json:"viewCount,omitempty"`
	CollectCount *int       `json:"collectCount,omitempty"`
	CoverURL     string     `json:"coverUrl,omitempty"`
	IPLocation   string     `json:"ipLocation,omitempty"`
}

// PlatformDataResponse 平台数据响应
type PlatformDataResponse struct {
	Data  []PlatformDataItem `json:"data"`
	Total int64              `json:"total"`
}
