package crawler

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"
	platformSync "opinion-analysis/src/service"
	"opinion-analysis/src/service/workflow/nodes"
)

// DataPatchNode 补数节点：计算平台源表与 articles 中心表的差集，
// 将未同步的源表 ID 列表传给下游 platform_sync 节点补录。
type DataPatchNode struct {
	*nodes.BaseNode
	db *gorm.DB
}

func NewDataPatchNode(db *gorm.DB) *DataPatchNode {
	return &DataPatchNode{
		BaseNode: nodes.NewBaseNode("data_patch"),
		db:       db,
	}
}

func (n *DataPatchNode) Validate(config map[string]interface{}) error {
	if platforms := n.GetStringSlice(config, "platforms"); len(platforms) == 0 {
		return fmt.Errorf("平台（platforms）为必填项")
	}
	return nil
}

func (n *DataPatchNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	platforms := n.GetStringSlice(config, "platforms")
	syncCodes := platformSync.ResolveSyncCodes(platforms)
	if len(syncCodes) == 0 {
		return nil, fmt.Errorf("无法解析平台配置: %v", platforms)
	}

	var topics []string
	if t := n.GetStringSlice(config, "topics"); len(t) > 0 {
		topics = t
	} else {
		topics = nodes.GetStringSliceFromInput(input, "topics")
	}

	syncSvc := platformSync.NewPlatformSyncService(n.db)

	totalMissing := 0
	patchResults := make(map[string]interface{}, len(syncCodes))
	perPlatformIDs := make(map[string]interface{}, len(syncCodes))

	for _, code := range syncCodes {
		sourceTable := platformSync.SyncCodeToSourceTable(code)
		if sourceTable == "" {
			continue
		}

		offset := syncSvc.GetOffset(code)

		var missingIDs []uint
		if err := n.db.WithContext(ctx).Table(sourceTable).
			Where("id > ?", offset).
			Pluck("id", &missingIDs).Error; err != nil {
			return nil, fmt.Errorf("查询平台 %s 差集失败: %w", code, err)
		}

		log.Printf("[DataPatchNode] platform=%s offset=%d missing=%d", code, offset, len(missingIDs))
		totalMissing += len(missingIDs)
		patchResults[code] = map[string]interface{}{
			"offset":       offset,
			"missingCount": len(missingIDs),
		}

		if len(missingIDs) > 0 {
			arr := make([]interface{}, len(missingIDs))
			for i, id := range missingIDs {
				arr[i] = float64(id)
			}
			perPlatformIDs[code] = arr
		}
	}

	if totalMissing == 0 {
		return nil, fmt.Errorf("所选平台暂无缺失数据，无需补数")
	}

	var topic string
	if len(topics) > 0 {
		topic = topics[0]
	}

	produced := map[string]interface{}{
		"syncPlatformCodes":          syncCodes,
		"includeSourceIdsByPlatform": perPlatformIDs,
		"patchResults":               patchResults,
		"missingCount":               totalMissing,
		"topics":                     topics,
		"topic":                      topic,
	}

	return nodes.CarryForward(input, produced), nil
}
