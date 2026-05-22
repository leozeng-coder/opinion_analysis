package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"opinion-analysis/src/model"
)

type PlatformCommentRepository struct {
	db *gorm.DB
}

func NewPlatformCommentRepository(db *gorm.DB) *PlatformCommentRepository {
	return &PlatformCommentRepository{db: db}
}

// QueryPlatformComments 查询平台评论（统一接口）
func (r *PlatformCommentRepository) QueryPlatformComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	switch query.Platform {
	case "xhs":
		return r.queryXhsComments(ctx, query)
	case "dy":
		return r.queryDouyinComments(ctx, query)
	case "bili":
		return r.queryBilibiliComments(ctx, query)
	case "wb":
		return r.queryWeiboComments(ctx, query)
	case "ks":
		return r.queryKuaishouComments(ctx, query)
	case "tieba":
		return r.queryTiebaComments(ctx, query)
	case "zhihu":
		return r.queryZhihuComments(ctx, query)
	default:
		return nil, 0, fmt.Errorf("unsupported platform: %s", query.Platform)
	}
}

// queryXhsComments 查询小红书评论
func (r *PlatformCommentRepository) queryXhsComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	// 先查询 note_id
	var noteID string
	if err := r.db.WithContext(ctx).Table("xhs_note").
		Select("note_id").
		Where("id = ?", query.ItemID).
		Scan(&noteID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("xhs_note_comment").
		Where("note_id = ?", noteID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		parent_comment_id,
		content,
		user_id,
		nickname,
		avatar,
		ip_location,
		FROM_UNIXTIME(create_time) as create_time,
		like_count,
		sub_comment_count
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var likeCountStr string
		var subCommentCount int

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&item.CreateTime, &likeCountStr, &subCommentCount,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.SubCommentCount = &subCommentCount

		items = append(items, item)
	}

	return items, total, nil
}

// queryDouyinComments 查询抖音评论
func (r *PlatformCommentRepository) queryDouyinComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	var awemeID string
	if err := r.db.WithContext(ctx).Table("douyin_aweme").
		Select("aweme_id").
		Where("id = ?", query.ItemID).
		Scan(&awemeID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("douyin_aweme_comment").
		Where("aweme_id = ?", awemeID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		parent_comment_id,
		content,
		user_id,
		nickname,
		avatar,
		ip_location,
		FROM_UNIXTIME(create_time) as create_time,
		like_count,
		sub_comment_count
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var likeCountStr, subCommentCountStr string

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&item.CreateTime, &likeCountStr, &subCommentCountStr,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.SubCommentCount = parseIntPtr(subCommentCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryBilibiliComments 查询B站评论
func (r *PlatformCommentRepository) queryBilibiliComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	var videoID string
	if err := r.db.WithContext(ctx).Table("bilibili_video").
		Select("video_id").
		Where("id = ?", query.ItemID).
		Scan(&videoID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("bilibili_video_comment").
		Where("video_id = ?", videoID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		'' as parent_comment_id,
		content,
		user_id,
		nickname,
		avatar,
		'' as ip_location,
		FROM_UNIXTIME(create_time) as create_time,
		like_count,
		sub_comment_count
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var likeCountStr, subCommentCountStr string

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&item.CreateTime, &likeCountStr, &subCommentCountStr,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.SubCommentCount = parseIntPtr(subCommentCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryWeiboComments 查询微博评论
func (r *PlatformCommentRepository) queryWeiboComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	var noteID string
	if err := r.db.WithContext(ctx).Table("weibo_note").
		Select("note_id").
		Where("id = ?", query.ItemID).
		Scan(&noteID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("weibo_note_comment").
		Where("note_id = ?", noteID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		parent_comment_id,
		content,
		user_id,
		nickname,
		avatar,
		ip_location,
		FROM_UNIXTIME(create_time) as create_time,
		comment_like_count,
		sub_comment_count
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var likeCountStr, subCommentCountStr string

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&item.CreateTime, &likeCountStr, &subCommentCountStr,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.SubCommentCount = parseIntPtr(subCommentCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryKuaishouComments 查询快手评论
func (r *PlatformCommentRepository) queryKuaishouComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	var videoID string
	if err := r.db.WithContext(ctx).Table("kuaishou_video").
		Select("video_id").
		Where("id = ?", query.ItemID).
		Scan(&videoID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("kuaishou_video_comment").
		Where("video_id = ?", videoID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		'' as parent_comment_id,
		content,
		user_id,
		nickname,
		avatar,
		'' as ip_location,
		FROM_UNIXTIME(create_time) as create_time,
		0 as like_count,
		sub_comment_count
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var likeCount int
		var subCommentCountStr string

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&item.CreateTime, &likeCount, &subCommentCountStr,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = &likeCount
		item.SubCommentCount = parseIntPtr(subCommentCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryTiebaComments 查询贴吧评论
func (r *PlatformCommentRepository) queryTiebaComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	var noteID string
	if err := r.db.WithContext(ctx).Table("tieba_note").
		Select("note_id").
		Where("id = ?", query.ItemID).
		Scan(&noteID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("tieba_comment").
		Where("note_id = ?", noteID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		parent_comment_id,
		content,
		'' as user_id,
		user_nickname,
		user_avatar,
		ip_location,
		STR_TO_DATE(publish_time, '%Y-%m-%d %H:%i:%s') as create_time,
		0 as like_count,
		sub_comment_count
	`).Order("publish_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var likeCount, subCommentCount int

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&item.CreateTime, &likeCount, &subCommentCount,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = &likeCount
		item.SubCommentCount = &subCommentCount

		items = append(items, item)
	}

	return items, total, nil
}

// queryZhihuComments 查询知乎评论
func (r *PlatformCommentRepository) queryZhihuComments(ctx context.Context, query model.PlatformCommentQuery) ([]model.PlatformCommentItem, int64, error) {
	var items []model.PlatformCommentItem
	var total int64

	var contentID string
	if err := r.db.WithContext(ctx).Table("zhihu_content").
		Select("content_id").
		Where("id = ?", query.ItemID).
		Scan(&contentID).Error; err != nil {
		return nil, 0, err
	}

	db := r.db.WithContext(ctx).Table("zhihu_comment").
		Where("content_id = ?", contentID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		comment_id,
		parent_comment_id,
		content,
		user_id,
		user_nickname,
		user_avatar,
		ip_location,
		STR_TO_DATE(publish_time, '%Y-%m-%d %H:%i:%s') as create_time,
		like_count,
		sub_comment_count
	`).Order("publish_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformCommentItem
		var publishTimeStr string
		var likeCount, subCommentCount int

		if err := rows.Scan(
			&item.ID, &item.CommentID, &item.ParentCommentID, &item.Content,
			&item.UserID, &item.Nickname, &item.Avatar, &item.IPLocation,
			&publishTimeStr, &likeCount, &subCommentCount,
		); err != nil {
			return nil, 0, err
		}

		if t, err := time.Parse("2006-01-02 15:04:05", publishTimeStr); err == nil {
			item.CreateTime = &t
		}

		item.LikeCount = &likeCount
		item.SubCommentCount = &subCommentCount

		items = append(items, item)
	}

	return items, total, nil
}
