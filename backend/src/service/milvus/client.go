// Package milvus 封装 standalone Milvus 连接、集合管理及读写操作。
package milvus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	fieldChunkPK    = "chunk_pk"
	fieldArticleID  = "article_id"
	fieldChunkIdx   = "chunk_idx"
	fieldEmbedding  = "embedding"
	fieldTitle      = "title"
	fieldSnippet    = "snippet"
	fieldPlatform   = "platform"
	fieldChunkType  = "chunk_type"
)

// ChunkRow 单条 chunk 数据（insert 用）。
type ChunkRow struct {
	ChunkPK   string
	ArticleID int64
	ChunkIdx  int64
	Embedding []float32
	Title     string
	Snippet   string
	Platform  string
	ChunkType string // "content" | "comment"
}

// SearchHit 单条向量检索结果。
type SearchHit struct {
	ChunkPK   string
	ArticleID int64
	ChunkIdx  int64
	Title     string
	Snippet   string
	Platform  string
	ChunkType string
	Score     float32
}

// Service Milvus 服务封装。
type Service struct {
	uri        string
	collection string
	cli        client.Client
}

// NewService 创建 Milvus service（不立即连接，lazy connect on first use）。
func NewService(uri, collection string) *Service {
	if collection == "" {
		collection = "opinion_chunks_kb"
	}
	return &Service{uri: uri, collection: collection}
}

func (s *Service) connect(ctx context.Context) (client.Client, error) {
	if s.cli != nil {
		return s.cli, nil
	}
	addr := strings.TrimPrefix(strings.TrimPrefix(s.uri, "http://"), "https://")
	cli, err := client.NewClient(ctx, client.Config{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("milvus connect %s: %w", s.uri, err)
	}
	s.cli = cli
	return cli, nil
}

// EnsureCollection 若集合不存在则按 dim 创建，否则直接 LoadCollection。
func (s *Service) EnsureCollection(ctx context.Context, dim int) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	exists, err := cli.HasCollection(ctx, s.collection)
	if err != nil {
		return fmt.Errorf("has_collection: %w", err)
	}
	if !exists {
		schema := &entity.Schema{
			CollectionName: s.collection,
			AutoID:         false,
			Fields: []*entity.Field{
				{Name: fieldChunkPK, DataType: entity.FieldTypeVarChar, PrimaryKey: true, TypeParams: map[string]string{"max_length": "96"}},
				{Name: fieldArticleID, DataType: entity.FieldTypeInt64},
				{Name: fieldChunkIdx, DataType: entity.FieldTypeInt64},
				{Name: fieldEmbedding, DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": fmt.Sprintf("%d", dim)}},
				{Name: fieldTitle, DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": "500"}},
				{Name: fieldSnippet, DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": "4096"}},
				{Name: fieldPlatform, DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": "32"}},
				{Name: fieldChunkType, DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": "16"}},
			},
		}
		idx, err := entity.NewIndexAUTOINDEX(entity.COSINE)
		if err != nil {
			return fmt.Errorf("create index param: %w", err)
		}
		if err := cli.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
		if err := cli.CreateIndex(ctx, s.collection, fieldEmbedding, idx, false); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}
	if err := cli.LoadCollection(ctx, s.collection, false); err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	return nil
}

// Insert 批量写入 chunks（先删同 article 的旧 chunk）。
func (s *Service) Insert(ctx context.Context, rows []ChunkRow) error {
	if len(rows) == 0 {
		return nil
	}
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}

	pks := make([]string, len(rows))
	aids := make([]int64, len(rows))
	idxs := make([]int64, len(rows))
	embs := make([][]float32, len(rows))
	titles := make([]string, len(rows))
	snippets := make([]string, len(rows))
	platforms := make([]string, len(rows))
	types := make([]string, len(rows))

	for i, r := range rows {
		pks[i] = r.ChunkPK
		aids[i] = r.ArticleID
		idxs[i] = r.ChunkIdx
		embs[i] = r.Embedding
		titles[i] = clip(r.Title, 500)
		snippets[i] = clip(r.Snippet, 4000)
		platforms[i] = clip(r.Platform, 30)
		types[i] = r.ChunkType
	}

	cols := []entity.Column{
		entity.NewColumnVarChar(fieldChunkPK, pks),
		entity.NewColumnInt64(fieldArticleID, aids),
		entity.NewColumnInt64(fieldChunkIdx, idxs),
		entity.NewColumnFloatVector(fieldEmbedding, len(embs[0]), embs),
		entity.NewColumnVarChar(fieldTitle, titles),
		entity.NewColumnVarChar(fieldSnippet, snippets),
		entity.NewColumnVarChar(fieldPlatform, platforms),
		entity.NewColumnVarChar(fieldChunkType, types),
	}
	_, err = cli.Insert(ctx, s.collection, "", cols...)
	return err
}

// DeleteByArticle 按 article_id 删除所有关联 chunks。
func (s *Service) DeleteByArticle(ctx context.Context, articleID int64) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	return cli.Delete(ctx, s.collection, "", fmt.Sprintf("article_id == %d", articleID))
}

// DeleteByPK 按 chunk_pk 删除单条 chunk。
func (s *Service) DeleteByPK(ctx context.Context, pk string) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	return cli.DeleteByPks(ctx, s.collection, "", entity.NewColumnVarChar(fieldChunkPK, []string{pk}))
}

// Search 向量相似度检索，返回 top-k 结果。
func (s *Service) Search(ctx context.Context, vec []float32, topK int) ([]SearchHit, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	sp, _ := entity.NewIndexAUTOINDEXSearchParam(1)
	results, err := cli.Search(
		ctx,
		s.collection,
		nil,
		"",
		[]string{fieldArticleID, fieldTitle, fieldSnippet, fieldPlatform, fieldChunkIdx, fieldChunkType},
		[]entity.Vector{entity.FloatVector(vec)},
		fieldEmbedding,
		entity.COSINE,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("milvus search: %w", err)
	}
	var hits []SearchHit
	if len(results) == 0 {
		return hits, nil
	}
	for i := 0; i < results[0].ResultCount; i++ {
		pk, _ := results[0].IDs.GetAsString(i)
		score := results[0].Scores[i]
		hit := SearchHit{ChunkPK: pk, Score: score}
		for _, col := range results[0].Fields {
			switch col.Name() {
			case fieldArticleID:
				if v, e := col.GetAsInt64(i); e == nil {
					hit.ArticleID = v
				}
			case fieldChunkIdx:
				if v, e := col.GetAsInt64(i); e == nil {
					hit.ChunkIdx = v
				}
			case fieldTitle:
				if v, e := col.GetAsString(i); e == nil {
					hit.Title = v
				}
			case fieldSnippet:
				if v, e := col.GetAsString(i); e == nil {
					hit.Snippet = v
				}
			case fieldPlatform:
				if v, e := col.GetAsString(i); e == nil {
					hit.Platform = v
				}
			case fieldChunkType:
				if v, e := col.GetAsString(i); e == nil {
					hit.ChunkType = v
				}
			}
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

// QueryByArticle 按 article_id 标量查询，返回该文章所有 chunks（不做向量运算）。
func (s *Service) QueryByArticle(ctx context.Context, articleID int64) ([]SearchHit, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	res, err := cli.Query(
		ctx,
		s.collection,
		nil,
		fmt.Sprintf("article_id == %d", articleID),
		[]string{fieldChunkPK, fieldArticleID, fieldChunkIdx, fieldTitle, fieldSnippet, fieldPlatform, fieldChunkType},
	)
	if err != nil {
		return nil, fmt.Errorf("milvus query: %w", err)
	}
	if len(res) == 0 {
		return nil, nil
	}
	n := res[0].Len()
	hits := make([]SearchHit, n)
	for _, col := range res {
		for i := 0; i < n; i++ {
			switch col.Name() {
			case fieldChunkPK:
				if v, e := col.GetAsString(i); e == nil {
					hits[i].ChunkPK = v
				}
			case fieldArticleID:
				if v, e := col.GetAsInt64(i); e == nil {
					hits[i].ArticleID = v
				}
			case fieldChunkIdx:
				if v, e := col.GetAsInt64(i); e == nil {
					hits[i].ChunkIdx = v
				}
			case fieldTitle:
				if v, e := col.GetAsString(i); e == nil {
					hits[i].Title = v
				}
			case fieldSnippet:
				if v, e := col.GetAsString(i); e == nil {
					hits[i].Snippet = v
				}
			case fieldPlatform:
				if v, e := col.GetAsString(i); e == nil {
					hits[i].Platform = v
				}
			case fieldChunkType:
				if v, e := col.GetAsString(i); e == nil {
					hits[i].ChunkType = v
				}
			}
		}
	}
	return hits, nil
}

// QueryByPK 查询单条 chunk（用于 update snippet 前读旧数据）。
func (s *Service) QueryByPK(ctx context.Context, pk string) (*SearchHit, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	res, err := cli.QueryByPks(
		ctx,
		s.collection,
		nil,
		entity.NewColumnVarChar(fieldChunkPK, []string{pk}),
		[]string{fieldChunkPK, fieldArticleID, fieldChunkIdx, fieldTitle, fieldSnippet, fieldPlatform, fieldChunkType},
	)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 || res[0].Len() == 0 {
		return nil, nil
	}
	hit := &SearchHit{}
	for _, col := range res {
		switch col.Name() {
		case fieldChunkPK:
			if v, e := col.GetAsString(0); e == nil {
				hit.ChunkPK = v
			}
		case fieldArticleID:
			if v, e := col.GetAsInt64(0); e == nil {
				hit.ArticleID = v
			}
		case fieldChunkIdx:
			if v, e := col.GetAsInt64(0); e == nil {
				hit.ChunkIdx = v
			}
		case fieldTitle:
			if v, e := col.GetAsString(0); e == nil {
				hit.Title = v
			}
		case fieldSnippet:
			if v, e := col.GetAsString(0); e == nil {
				hit.Snippet = v
			}
		case fieldPlatform:
			if v, e := col.GetAsString(0); e == nil {
				hit.Platform = v
			}
		case fieldChunkType:
			if v, e := col.GetAsString(0); e == nil {
				hit.ChunkType = v
			}
		}
	}
	return hit, nil
}

// HasCollection 检查集合是否存在。
func (s *Service) HasCollection(ctx context.Context) (bool, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return false, err
	}
	return cli.HasCollection(ctx, s.collection)
}

// DropCollection 删除集合（rebuild 用）。
func (s *Service) DropCollection(ctx context.Context) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	exists, err := cli.HasCollection(ctx, s.collection)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return cli.DropCollection(ctx, s.collection)
}

// Ping 连通性检查。
func (s *Service) Ping(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	_, err = cli.ListCollections(ctx)
	return err
}

// CollectionName 返回当前集合名。
func (s *Service) CollectionName() string { return s.collection }

func clip(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
