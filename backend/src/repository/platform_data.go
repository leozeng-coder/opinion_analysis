package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"opinion-analysis/src/model"
)

type PlatformDataRepository struct {
	db *gorm.DB
}

func NewPlatformDataRepository(db *gorm.DB) *PlatformDataRepository {
	return &PlatformDataRepository{db: db}
}

// QueryPlatformData 查询平台数据（统一接口）
func (r *PlatformDataRepository) QueryPlatformData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	// 根据平台类型查询不同的表
	switch query.Platform {
	case "xhs":
		return r.queryXhsData(ctx, query)
	case "dy":
		return r.queryDouyinData(ctx, query)
	case "bili":
		return r.queryBilibiliData(ctx, query)
	case "wb":
		return r.queryWeiboData(ctx, query)
	case "ks":
		return r.queryKuaishouData(ctx, query)
	case "tieba":
		return r.queryTiebaData(ctx, query)
	case "zhihu":
		return r.queryZhihuData(ctx, query)
	case "":
		// 查询所有平台
		return r.queryAllPlatforms(ctx, query)
	default:
		return items, total, fmt.Errorf("unsupported platform: %s", query.Platform)
	}
}

// queryXhsData 查询小红书数据
func (r *PlatformDataRepository) queryXhsData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("xhs_note")

	// 时间范围过滤
	if query.StartDate != "" {
		db = db.Where("FROM_UNIXTIME(time) >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("FROM_UNIXTIME(time) <= ?", query.EndDate+" 23:59:59")
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 查询数据
	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'xhs' as platform,
		title,
		` + "`desc`" + ` as content,
		nickname as author,
		avatar,
		note_url as url,
		FROM_UNIXTIME(time) as publish_time,
		liked_count,
		comment_count,
		share_count,
		collected_count,
		image_list as cover_url,
		ip_location
	`).Order("time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var likeCountStr, commentCountStr, shareCountStr, collectCountStr string

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &item.PublishTime,
			&likeCountStr, &commentCountStr, &shareCountStr, &collectCountStr,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		// 转换字符串数字为 int
		item.LikeCount = parseIntPtr(likeCountStr)
		item.CommentCount = parseIntPtr(commentCountStr)
		item.ShareCount = parseIntPtr(shareCountStr)
		item.CollectCount = parseIntPtr(collectCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryDouyinData 查询抖音数据
func (r *PlatformDataRepository) queryDouyinData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("douyin_aweme")

	if query.StartDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) <= ?", query.EndDate+" 23:59:59")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'dy' as platform,
		title,
		` + "`desc`" + ` as content,
		nickname as author,
		avatar,
		aweme_url as url,
		FROM_UNIXTIME(create_time) as publish_time,
		liked_count,
		comment_count,
		share_count,
		collected_count,
		cover_url,
		ip_location
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var likeCountStr, commentCountStr, shareCountStr, collectCountStr string

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &item.PublishTime,
			&likeCountStr, &commentCountStr, &shareCountStr, &collectCountStr,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.CommentCount = parseIntPtr(commentCountStr)
		item.ShareCount = parseIntPtr(shareCountStr)
		item.CollectCount = parseIntPtr(collectCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryBilibiliData 查询B站数据
func (r *PlatformDataRepository) queryBilibiliData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("bilibili_video")

	if query.StartDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) <= ?", query.EndDate+" 23:59:59")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'bili' as platform,
		title,
		` + "`desc`" + ` as content,
		nickname as author,
		avatar,
		video_url as url,
		FROM_UNIXTIME(create_time) as publish_time,
		liked_count,
		video_comment as comment_count,
		video_share_count as share_count,
		video_play_count as view_count,
		video_favorite_count as collect_count,
		video_cover_url as cover_url,
		'' as ip_location
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var likeCountStr, commentCountStr, shareCountStr, viewCountStr, collectCountStr string

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &item.PublishTime,
			&likeCountStr, &commentCountStr, &shareCountStr, &viewCountStr, &collectCountStr,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.CommentCount = parseIntPtr(commentCountStr)
		item.ShareCount = parseIntPtr(shareCountStr)
		item.ViewCount = parseIntPtr(viewCountStr)
		item.CollectCount = parseIntPtr(collectCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryWeiboData 查询微博数据
func (r *PlatformDataRepository) queryWeiboData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("weibo_note")

	if query.StartDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) <= ?", query.EndDate+" 23:59:59")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'wb' as platform,
		'' as title,
		content,
		nickname as author,
		avatar,
		note_url as url,
		FROM_UNIXTIME(create_time) as publish_time,
		liked_count,
		comments_count as comment_count,
		shared_count as share_count,
		'' as cover_url,
		ip_location
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var likeCountStr, commentCountStr, shareCountStr string

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &item.PublishTime,
			&likeCountStr, &commentCountStr, &shareCountStr,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.CommentCount = parseIntPtr(commentCountStr)
		item.ShareCount = parseIntPtr(shareCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryKuaishouData 查询快手数据
func (r *PlatformDataRepository) queryKuaishouData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("kuaishou_video")

	if query.StartDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("FROM_UNIXTIME(create_time) <= ?", query.EndDate+" 23:59:59")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'ks' as platform,
		title,
		` + "`desc`" + ` as content,
		nickname as author,
		avatar,
		video_url as url,
		FROM_UNIXTIME(create_time) as publish_time,
		liked_count,
		'' as comment_count,
		'' as share_count,
		viewd_count as view_count,
		video_cover_url as cover_url,
		'' as ip_location
	`).Order("create_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var likeCountStr, commentCountStr, shareCountStr, viewCountStr string

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &item.PublishTime,
			&likeCountStr, &commentCountStr, &shareCountStr, &viewCountStr,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		item.LikeCount = parseIntPtr(likeCountStr)
		item.CommentCount = parseIntPtr(commentCountStr)
		item.ShareCount = parseIntPtr(shareCountStr)
		item.ViewCount = parseIntPtr(viewCountStr)

		items = append(items, item)
	}

	return items, total, nil
}

// queryTiebaData 查询贴吧数据
func (r *PlatformDataRepository) queryTiebaData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("tieba_note")

	if query.StartDate != "" {
		db = db.Where("publish_time >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("publish_time <= ?", query.EndDate+" 23:59:59")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'tieba' as platform,
		title,
		` + "`desc`" + ` as content,
		user_nickname as author,
		user_avatar as avatar,
		note_url as url,
		publish_time,
		total_replay_num as comment_count,
		'' as cover_url,
		ip_location
	`).Order("publish_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var publishTimeStr *string // 改为指针类型以支持 NULL
		var commentCount int

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &publishTimeStr,
			&commentCount,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		if publishTimeStr != nil && *publishTimeStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", *publishTimeStr); err == nil {
				item.PublishTime = &t
			}
		}

		item.CommentCount = &commentCount

		items = append(items, item)
	}

	return items, total, nil
}

// queryZhihuData 查询知乎数据
func (r *PlatformDataRepository) queryZhihuData(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var items []model.PlatformDataItem
	var total int64

	db := r.db.WithContext(ctx).Table("zhihu_content")

	if query.StartDate != "" {
		db = db.Where("created_time >= ?", query.StartDate)
	}
	if query.EndDate != "" {
		db = db.Where("created_time <= ?", query.EndDate+" 23:59:59")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	rows, err := db.Select(`
		id,
		'zhihu' as platform,
		title,
		` + "`desc`" + ` as content,
		user_nickname as author,
		user_avatar as avatar,
		content_url as url,
		created_time as publish_time,
		voteup_count as like_count,
		comment_count,
		0 as share_count,
		'' as cover_url,
		'' as ip_location
	`).Order("created_time DESC").Limit(query.PageSize).Offset(offset).Rows()

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.PlatformDataItem
		var publishTimeStr *string // 改为指针类型以支持 NULL
		var likeCount, commentCount int

		if err := rows.Scan(
			&item.ID, &item.Platform, &item.Title, &item.Content, &item.Author,
			&item.Avatar, &item.URL, &publishTimeStr,
			&likeCount, &commentCount, &item.ShareCount,
			&item.CoverURL, &item.IPLocation,
		); err != nil {
			return nil, 0, err
		}

		// 解析时间字符串
		if publishTimeStr != nil && *publishTimeStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", *publishTimeStr); err == nil {
				item.PublishTime = &t
			}
		}

		item.LikeCount = &likeCount
		item.CommentCount = &commentCount

		items = append(items, item)
	}

	return items, total, nil
}

// queryAllPlatforms 查询所有平台数据（合并查询）
func (r *PlatformDataRepository) queryAllPlatforms(ctx context.Context, query model.PlatformDataQuery) ([]model.PlatformDataItem, int64, error) {
	var allItems []model.PlatformDataItem
	var totalCount int64

	platforms := []string{"xhs", "dy", "bili", "wb", "ks", "tieba", "zhihu"}

	for _, platform := range platforms {
		platformQuery := query
		platformQuery.Platform = platform
		platformQuery.Page = 1
		platformQuery.PageSize = 100 // 每个平台取前100条

		items, _, err := r.QueryPlatformData(ctx, platformQuery)
		if err != nil {
			// 忽略单个平台的错误，继续查询其他平台
			continue
		}

		allItems = append(allItems, items...)
	}

	// 按时间排序
	// 这里简化处理，实际应该在数据库层面做 UNION 查询
	totalCount = int64(len(allItems))

	// 分页
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize
	if start > len(allItems) {
		return []model.PlatformDataItem{}, totalCount, nil
	}
	if end > len(allItems) {
		end = len(allItems)
	}

	return allItems[start:end], totalCount, nil
}

// parseIntPtr 将字符串转换为 int 指针
func parseIntPtr(s string) *int {
	if s == "" {
		return nil
	}
	if val, err := strconv.Atoi(s); err == nil {
		return &val
	}
	return nil
}
