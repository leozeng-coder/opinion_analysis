package service

import (
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm/clause"
	"opinion-analysis/src/model"
)

// syncComments 同步平台评论到 article_comments 表（全量，保留层级）
func (b *BaseSyncer) syncComments(articleID uint, platform string, platformItemID string) error {
	if platformItemID == "" {
		return nil
	}

	type rawComment struct {
		CommentID       string
		ParentCommentID string
		Content         string
		Nickname        string
		LikeCount       int
		IPLocation      string
		CreateTime      int64
		PublishTime     string // tieba/zhihu 用字符串时间
	}

	var raws []rawComment

	switch platform {
	case "xhs":
		var rows []model.XhsNoteComment
		if err := b.db.Where("note_id = ?", platformItemID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query xhs comments: %w", err)
		}
		for _, r := range rows {
			lc, _ := strconv.Atoi(r.LikeCount)
			raws = append(raws, rawComment{
				CommentID:       r.CommentID,
				ParentCommentID: r.ParentCommentID,
				Content:         r.Content,
				Nickname:        r.Nickname,
				LikeCount:       lc,
				IPLocation:      r.IPLocation,
				CreateTime:      r.CreateTime,
			})
		}

	case "douyin":
		var rows []model.DouyinAwemeComment
		awemeID, _ := strconv.ParseInt(platformItemID, 10, 64)
		if err := b.db.Where("aweme_id = ?", awemeID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query douyin comments: %w", err)
		}
		for _, r := range rows {
			lc, _ := strconv.Atoi(r.LikeCount)
			raws = append(raws, rawComment{
				CommentID:       strconv.FormatInt(r.CommentID, 10),
				ParentCommentID: r.ParentCommentID,
				Content:         r.Content,
				Nickname:        r.Nickname,
				LikeCount:       lc,
				IPLocation:      r.IPLocation,
				CreateTime:      r.CreateTime,
			})
		}

	case "bilibili":
		var rows []model.BilibiliVideoComment
		videoID, _ := strconv.ParseInt(platformItemID, 10, 64)
		if err := b.db.Where("video_id = ?", videoID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query bilibili comments: %w", err)
		}
		for _, r := range rows {
			lc, _ := strconv.Atoi(r.LikeCount)
			raws = append(raws, rawComment{
				CommentID:       strconv.FormatInt(r.CommentID, 10),
				ParentCommentID: r.ParentCommentID,
				Content:         r.Content,
				Nickname:        r.Nickname,
				LikeCount:       lc,
				CreateTime:      r.CreateTime,
			})
		}

	case "weibo":
		var rows []model.WeiboNoteComment
		noteID, _ := strconv.ParseInt(platformItemID, 10, 64)
		if err := b.db.Where("note_id = ?", noteID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query weibo comments: %w", err)
		}
		for _, r := range rows {
			lc, _ := strconv.Atoi(r.CommentLikeCount)
			raws = append(raws, rawComment{
				CommentID:       strconv.FormatInt(r.CommentID, 10),
				ParentCommentID: r.ParentCommentID,
				Content:         r.Content,
				Nickname:        r.Nickname,
				LikeCount:       lc,
				IPLocation:      r.IPLocation,
				CreateTime:      r.CreateTime,
			})
		}

	case "kuaishou":
		var rows []model.KuaishouVideoComment
		if err := b.db.Where("video_id = ?", platformItemID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query kuaishou comments: %w", err)
		}
		for _, r := range rows {
			raws = append(raws, rawComment{
				CommentID:  strconv.FormatInt(r.CommentID, 10),
				Content:    r.Content,
				Nickname:   r.Nickname,
				CreateTime: r.CreateTime,
			})
		}

	case "tieba":
		var rows []model.TiebaComment
		if err := b.db.Where("note_id = ?", platformItemID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query tieba comments: %w", err)
		}
		for _, r := range rows {
			raws = append(raws, rawComment{
				CommentID:       r.CommentID,
				ParentCommentID: r.ParentCommentID,
				Content:         r.Content,
				Nickname:        r.UserNickname,
				IPLocation:      r.IPLocation,
				PublishTime:     r.PublishTime,
			})
		}

	case "zhihu":
		var rows []model.ZhihuComment
		if err := b.db.Where("content_id = ?", platformItemID).Find(&rows).Error; err != nil {
			return fmt.Errorf("query zhihu comments: %w", err)
		}
		for _, r := range rows {
			raws = append(raws, rawComment{
				CommentID:       r.CommentID,
				ParentCommentID: r.ParentCommentID,
				Content:         r.Content,
				Nickname:        r.UserNickname,
				LikeCount:       r.LikeCount,
				IPLocation:      r.IPLocation,
				PublishTime:     r.PublishTime,
			})
		}

	default:
		return nil
	}

	if len(raws) == 0 {
		return nil
	}

	// 第一轮：插入一级评论（无 parent）
	// platformCommentID -> 本表 ID 的映射，用于第二轮设置 ParentID
	idMap := make(map[string]uint, len(raws))

	for _, r := range raws {
		if r.ParentCommentID != "" && r.ParentCommentID != "0" {
			continue
		}
		publishedAt := parseCommentTime(r.CreateTime, r.PublishTime)
		pCID := platform + ":" + r.CommentID

		comment := model.ArticleComment{
			ArticleID:         articleID,
			PlatformCommentID: pCID,
			Content:           removeEmoji(r.Content),
			Nickname:          removeEmoji(r.Nickname),
			LikeCount:         r.LikeCount,
			IPLocation:        r.IPLocation,
			PublishedAt:       publishedAt,
		}

		result := b.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "platform_comment_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "like_count", "nickname"}),
		}).Create(&comment)
		if result.Error != nil {
			continue
		}
		idMap[r.CommentID] = comment.ID
	}

	// 第二轮：插入子评论（有 parent）
	for _, r := range raws {
		if r.ParentCommentID == "" || r.ParentCommentID == "0" {
			continue
		}
		publishedAt := parseCommentTime(r.CreateTime, r.PublishTime)
		pCID := platform + ":" + r.CommentID

		var parentID *uint
		if pid, ok := idMap[r.ParentCommentID]; ok {
			parentID = &pid
		}

		comment := model.ArticleComment{
			ArticleID:         articleID,
			PlatformCommentID: pCID,
			ParentID:          parentID,
			Content:           removeEmoji(r.Content),
			Nickname:          removeEmoji(r.Nickname),
			LikeCount:         r.LikeCount,
			IPLocation:        r.IPLocation,
			PublishedAt:       publishedAt,
		}

		result := b.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "platform_comment_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "like_count", "nickname", "parent_id"}),
		}).Create(&comment)
		if result.Error != nil {
			continue
		}
		idMap[r.CommentID] = comment.ID
	}

	return nil
}

func parseCommentTime(unixTs int64, timeStr string) time.Time {
	if unixTs > 0 {
		if unixTs > 9999999999 {
			unixTs = unixTs / 1000
		}
		return time.Unix(unixTs, 0)
	}
	if timeStr != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
			return t
		}
	}
	return time.Now()
}
