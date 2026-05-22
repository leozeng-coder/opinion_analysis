package model

import "time"

// PlatformCommentQuery 平台评论查询参数
type PlatformCommentQuery struct {
	Platform string `form:"platform" binding:"required"`
	ItemID   uint   `form:"itemId" binding:"required"`
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"pageSize" binding:"required,min=1,max=100"`
}

// PlatformCommentItem 平台评论项（统一格式）
type PlatformCommentItem struct {
	ID               uint       `json:"id"`
	CommentID        string     `json:"commentId"`
	ParentCommentID  string     `json:"parentCommentId,omitempty"`
	Content          string     `json:"content"`
	UserID           string     `json:"userId"`
	Nickname         string     `json:"nickname"`
	Avatar           string     `json:"avatar,omitempty"`
	IPLocation       string     `json:"ipLocation,omitempty"`
	CreateTime       *time.Time `json:"createTime"`
	LikeCount        *int       `json:"likeCount,omitempty"`
	SubCommentCount  *int       `json:"subCommentCount,omitempty"`
}

// PlatformCommentResponse 平台评论响应
type PlatformCommentResponse struct {
	Data  []PlatformCommentItem `json:"data"`
	Total int64                 `json:"total"`
}
