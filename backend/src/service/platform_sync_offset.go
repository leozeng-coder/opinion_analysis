package service

import (
	"context"
	"log"

	"opinion-analysis/src/model"
)

// getOffset 读取平台同步偏移量（不存在则返回 0，即首次全量扫描）。
func (s *PlatformSyncService) getOffset(platform string) uint {
	var off model.PlatformSyncOffset
	if err := s.db.Where("platform = ?", platform).First(&off).Error; err != nil {
		return 0
	}
	return off.LastSourceID
}

// advanceOffset 将平台偏移量前移到 id（仅在更大时推进，保证单调不回退）。
func (s *PlatformSyncService) advanceOffset(platform, sourceTable string, id uint) {
	var off model.PlatformSyncOffset
	err := s.db.Where("platform = ?", platform).First(&off).Error
	if err != nil {
		off = model.PlatformSyncOffset{Platform: platform, SourceTable: sourceTable, LastSourceID: id}
		if cerr := s.db.Create(&off).Error; cerr != nil {
			log.Printf("[PlatformSync] create offset %s=%d failed: %v", platform, id, cerr)
		}
		return
	}
	if id > off.LastSourceID {
		off.LastSourceID = id
		off.SourceTable = sourceTable
		if uerr := s.db.Save(&off).Error; uerr != nil {
			log.Printf("[PlatformSync] advance offset %s=%d failed: %v", platform, id, uerr)
		}
	}
}

// syncPlatformScan 以「源表主键扫描 + id>minSourceID 过滤」的方式同步，成功后推进偏移量。
//
//   - minSourceID = 0     → 真正的全表扫描（首次同步 / 全量对账）
//   - minSourceID = offset → 增量同步，只处理新增行，O(新增行数)
//
// 两种模式都依赖 syncer 内部的 origin_url 去重，天然幂等；即使重复扫描也不会重复写入。
// 偏移量推进策略：扫描前先捕获源表 max(id) 作为目标位置，本次处理区间 (minSourceID, maxID]，
// 此后新插入的行 id 必然 > maxID，会在下次同步被捕获，因此不会漏行。
func (s *PlatformSyncService) syncPlatformScan(ctx context.Context, platform string, minSourceID uint, enableSentiment bool) (*SyncResult, error) {
	syncer, err := s.factory.GetSyncer(platform)
	if err != nil {
		return nil, err
	}
	sourceTable := syncer.GetSourceTable()

	maxID, err := s.MaxSourceTableID(ctx, sourceTable)
	if err != nil {
		return nil, err
	}

	config := SyncConfig{
		Platform:        platform,
		SyncMode:        "full", // 配合 MinSourceID：不按时间过滤，仅按主键过滤
		SourceID:        s.getOrCreateDefaultSource(),
		EnableSentiment: enableSentiment,
		MinSourceID:     minSourceID,
	}

	progress := s.progressTracker.StartProgress(platform, 0)
	if err := syncer.Sync(ctx, config, progress); err != nil {
		progress.SetError(err)
		return s.progressToResult(progress), err
	}
	progress.SetStatus("completed")

	snap := progress.GetSnapshot()
	if snap.ErrorCount > 0 {
		// 有行处理失败：不推进偏移量，下次从同一位置重扫（去重保证已成功的行被跳过），
		// 让失败行有机会重试，避免静默漏数据。失败详情见 syncer 日志。
		log.Printf("[PlatformSync] %s 有 %d 行处理失败，偏移量保持 %d 不推进（待下次重试）",
			platform, snap.ErrorCount, minSourceID)
	} else {
		s.advanceOffset(platform, sourceTable, maxID)
	}

	return s.progressToResult(progress), nil
}

// SyncPlatformByOffset 基于持久化偏移量的增量同步（推荐路径，gap-free 且 O(新增行数)）。
func (s *PlatformSyncService) SyncPlatformByOffset(ctx context.Context, platform string, enableSentiment bool) (*SyncResult, error) {
	return s.syncPlatformScan(ctx, platform, s.getOffset(platform), enableSentiment)
}

// SyncPlatformFull 真正的全表扫描同步（用于全量对账 / 首次导入），并对齐偏移量。
// 修复了旧 SyncSinglePlatform 在 syncMode=full 下仍按 LastSyncTime 过滤、无法补回老数据的问题。
func (s *PlatformSyncService) SyncPlatformFull(ctx context.Context, platform string, enableSentiment bool) (*SyncResult, error) {
	return s.syncPlatformScan(ctx, platform, 0, enableSentiment)
}
