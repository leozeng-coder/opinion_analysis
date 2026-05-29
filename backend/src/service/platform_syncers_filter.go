package service

import (
	"context"
	"fmt"

	"opinion-analysis/src/model"
)

// 各平台同步器读取源表行（id + 标题 + 正文），供「数据过滤」节点在入库前筛选。
// 字段映射与各自 Sync() 中 source→article 的映射保持一致。

func (s *XhsSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.XhsNote
	q := s.db.WithContext(ctx).Table("xhs_note")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query xhs_note failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Title, Content: r.Desc})
	}
	return out, nil
}

func (s *DouyinSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.DouyinAweme
	q := s.db.WithContext(ctx).Table("douyin_aweme")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query douyin_aweme failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Title, Content: r.Desc})
	}
	return out, nil
}

func (s *BilibiliSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.BilibiliVideo
	q := s.db.WithContext(ctx).Table("bilibili_video")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query bilibili_video failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Title, Content: r.Desc})
	}
	return out, nil
}

func (s *WeiboSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.WeiboNote
	q := s.db.WithContext(ctx).Table("weibo_note")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query weibo_note failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		// 微博无独立标题，与 Sync 一致用正文兜底
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Content, Content: r.Content})
	}
	return out, nil
}

func (s *KuaishouSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.KuaishouVideo
	q := s.db.WithContext(ctx).Table("kuaishou_video")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query kuaishou_video failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Title, Content: r.Desc})
	}
	return out, nil
}

func (s *TiebaSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.TiebaNote
	q := s.db.WithContext(ctx).Table("tieba_note")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query tieba_note failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Title, Content: r.Desc})
	}
	return out, nil
}

func (s *ZhihuSyncer) FetchFilterRows(ctx context.Context, minSourceID uint) ([]SourceFilterRow, error) {
	var rows []model.ZhihuContent
	q := s.db.WithContext(ctx).Table("zhihu_content")
	if minSourceID > 0 {
		q = q.Where("id > ?", minSourceID)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query zhihu_content failed: %w", err)
	}
	out := make([]SourceFilterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SourceFilterRow{SourceID: r.ID, Title: r.Title, Content: r.ContentText})
	}
	return out, nil
}
