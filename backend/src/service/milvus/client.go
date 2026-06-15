// Package milvus 封装 standalone Milvus 连接、集合管理及读写操作。
// 检索采用 dense 向量 + BM25 稀疏向量的多路召回，由 Milvus 内部完成 RRF 融合。
package milvus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

const (
	fieldChunkPK   = "chunk_pk"
	fieldArticleID = "article_id"
	fieldChunkIdx  = "chunk_idx"
	fieldEmbedding = "embedding"
	fieldTitle     = "title"
	fieldSnippet   = "snippet"
	fieldPlatform  = "platform"
	fieldChunkType = "chunk_type"
	fieldTopic     = "topic"
	// fieldContentText 供 BM25 分词的原文字段（开启 jieba analyzer）。
	fieldContentText = "content_text"
	// fieldSparse 由 BM25 Function 从 content_text 自动生成的稀疏向量。
	fieldSparse = "sparse_vector"

	bm25FunctionName = "content_bm25"
	contentTextMax   = 8192

	defaultPartition = "default"
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
	Topic     string
}

// SearchHit 单条检索结果。
type SearchHit struct {
	ChunkPK   string
	ArticleID int64
	ChunkIdx  int64
	Title     string
	Snippet   string
	Platform  string
	ChunkType string
	Topic     string
	Score     float32
}

// Service Milvus 服务封装。
type Service struct {
	uri        string
	collection string
	cli        *milvusclient.Client
}

// NewService 创建 Milvus service（不立即连接，lazy connect on first use）。
func NewService(uri, collection string) *Service {
	if collection == "" {
		collection = "opinion_chunks_kb"
	}
	return &Service{uri: uri, collection: collection}
}

func (s *Service) connect(ctx context.Context) (*milvusclient.Client, error) {
	if s.cli != nil {
		return s.cli, nil
	}
	addr := strings.TrimPrefix(strings.TrimPrefix(s.uri, "http://"), "https://")
	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("milvus connect %s: %w", s.uri, err)
	}
	s.cli = cli
	return cli, nil
}

// EnsureCollection 若集合不存在则按 dim 创建（含 BM25 稀疏字段），否则检查 schema 是否已含
// BM25 字段：旧版集合（无 sparse_vector）会自动 drop 重建，需重新全量同步。最后 LoadCollection。
func (s *Service) EnsureCollection(ctx context.Context, dim int) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	exists, err := cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(s.collection))
	if err != nil {
		return fmt.Errorf("has_collection: %w", err)
	}

	if exists {
		// 检查是否为旧 schema（缺少 BM25 稀疏字段），缺失则 drop 重建。
		desc, derr := cli.DescribeCollection(ctx, milvusclient.NewDescribeCollectionOption(s.collection))
		if derr != nil {
			return fmt.Errorf("describe collection: %w", derr)
		}
		if !hasField(desc.Schema, fieldSparse) {
			if e := cli.DropCollection(ctx, milvusclient.NewDropCollectionOption(s.collection)); e != nil {
				return fmt.Errorf("drop legacy collection: %w", e)
			}
			exists = false
		}
	}

	if !exists {
		if err := s.createCollection(ctx, dim); err != nil {
			return err
		}
	}

	task, err := cli.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(s.collection))
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	return task.Await(ctx)
}

// createCollection 按 BM25 + dense 双路 schema 创建集合并建索引。
func (s *Service) createCollection(ctx context.Context, dim int) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}

	schema := entity.NewSchema().WithName(s.collection).
		WithField(entity.NewField().WithName(fieldChunkPK).WithDataType(entity.FieldTypeVarChar).WithIsPrimaryKey(true).WithMaxLength(96)).
		WithField(entity.NewField().WithName(fieldArticleID).WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName(fieldChunkIdx).WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName(fieldEmbedding).WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dim))).
		WithField(entity.NewField().WithName(fieldTitle).WithDataType(entity.FieldTypeVarChar).WithMaxLength(500)).
		WithField(entity.NewField().WithName(fieldSnippet).WithDataType(entity.FieldTypeVarChar).WithMaxLength(4096)).
		WithField(entity.NewField().WithName(fieldPlatform).WithDataType(entity.FieldTypeVarChar).WithMaxLength(32)).
		WithField(entity.NewField().WithName(fieldChunkType).WithDataType(entity.FieldTypeVarChar).WithMaxLength(16)).
		WithField(entity.NewField().WithName(fieldTopic).WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		// BM25 输入字段：开启 jieba analyzer 做中文分词
		WithField(entity.NewField().WithName(fieldContentText).WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(contentTextMax).WithEnableAnalyzer(true).
			WithAnalyzerParams(map[string]any{"tokenizer": "jieba"})).
		// BM25 输出字段：由 Function 自动生成的稀疏向量
		WithField(entity.NewField().WithName(fieldSparse).WithDataType(entity.FieldTypeSparseVector)).
		WithFunction(entity.NewFunction().
			WithName(bm25FunctionName).
			WithType(entity.FunctionTypeBM25).
			WithInputFields(fieldContentText).
			WithOutputFields(fieldSparse))

	if err := cli.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(s.collection, schema)); err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	// dense 向量索引（AUTOINDEX + COSINE）
	denseTask, err := cli.CreateIndex(ctx, milvusclient.NewCreateIndexOption(s.collection, fieldEmbedding, index.NewAutoIndex(entity.COSINE)))
	if err != nil {
		return fmt.Errorf("create dense index: %w", err)
	}
	if err := denseTask.Await(ctx); err != nil {
		return fmt.Errorf("await dense index: %w", err)
	}

	// 稀疏向量索引（SPARSE_INVERTED_INDEX + BM25 metric）
	sparseTask, err := cli.CreateIndex(ctx, milvusclient.NewCreateIndexOption(s.collection, fieldSparse, index.NewSparseInvertedIndex(entity.BM25, 0.0)))
	if err != nil {
		return fmt.Errorf("create sparse index: %w", err)
	}
	return sparseTask.Await(ctx)
}

func hasField(schema *entity.Schema, name string) bool {
	if schema == nil {
		return false
	}
	for _, f := range schema.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// Insert 批量写入 chunks（先删同 article 的旧 chunk）。
// 注意：sparse_vector 字段由 BM25 Function 从 content_text 自动生成，不在此显式写入。
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
	topics := make([]string, len(rows))
	texts := make([]string, len(rows))

	for i, r := range rows {
		pks[i] = r.ChunkPK
		aids[i] = r.ArticleID
		idxs[i] = r.ChunkIdx
		embs[i] = r.Embedding
		titles[i] = clip(r.Title, 500)
		snippets[i] = clip(r.Snippet, 4000)
		platforms[i] = clip(r.Platform, 30)
		types[i] = r.ChunkType
		t := r.Topic
		if t == "" {
			t = defaultPartition
		}
		topics[i] = clip(t, 60)
		// BM25 分词文本：标题 + 正文，截断到 analyzer 上限内
		texts[i] = clip(bm25Text(r.Title, r.Snippet), contentTextMax-1)
	}

	opt := milvusclient.NewColumnBasedInsertOption(s.collection,
		column.NewColumnVarChar(fieldChunkPK, pks),
		column.NewColumnInt64(fieldArticleID, aids),
		column.NewColumnInt64(fieldChunkIdx, idxs),
		column.NewColumnFloatVector(fieldEmbedding, len(embs[0]), embs),
		column.NewColumnVarChar(fieldTitle, titles),
		column.NewColumnVarChar(fieldSnippet, snippets),
		column.NewColumnVarChar(fieldPlatform, platforms),
		column.NewColumnVarChar(fieldChunkType, types),
		column.NewColumnVarChar(fieldTopic, topics),
		column.NewColumnVarChar(fieldContentText, texts),
	)
	_, err = cli.Insert(ctx, opt)
	return err
}

// bm25Text 拼接标题和正文供 BM25 分词；标题给一定权重（重复一次）。
func bm25Text(title, snippet string) string {
	t := strings.TrimSpace(title)
	c := strings.TrimSpace(snippet)
	switch {
	case t == "":
		return c
	case c == "":
		return t
	default:
		return t + "\n" + c
	}
}

// DeleteByArticle 按 article_id 删除所有关联 chunks。
func (s *Service) DeleteByArticle(ctx context.Context, articleID int64) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	_, err = cli.Delete(ctx, milvusclient.NewDeleteOption(s.collection).
		WithExpr(fmt.Sprintf("%s == %d", fieldArticleID, articleID)))
	return err
}

// DeleteByPK 按 chunk_pk 删除单条 chunk。
func (s *Service) DeleteByPK(ctx context.Context, pk string) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	_, err = cli.Delete(ctx, milvusclient.NewDeleteOption(s.collection).
		WithStringIDs(fieldChunkPK, []string{pk}))
	return err
}

// outputFields 检索结果需返回的标量字段。
func searchOutputFields() []string {
	return []string{fieldArticleID, fieldChunkIdx, fieldTitle, fieldSnippet, fieldPlatform, fieldChunkType, fieldTopic}
}

// topicFilter 构造 topic 过滤表达式；topics 为空返回空串（搜全部）。
func topicFilter(topics []string) string {
	if len(topics) == 0 {
		return ""
	}
	quoted := make([]string, len(topics))
	for i, t := range topics {
		quoted[i] = fmt.Sprintf("%q", t)
	}
	return fmt.Sprintf("%s in [%s]", fieldTopic, strings.Join(quoted, ","))
}

// Search 多路召回：dense 向量(COSINE) + BM25 稀疏向量，Milvus 内部 RRF 融合。
// query 为原始查询文本（供 BM25 分词），vec 为其 dense embedding；topics 为空搜全部。
func (s *Service) Search(ctx context.Context, query string, vec []float32, topK int, topics []string) ([]SearchHit, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	if topK <= 0 {
		topK = 8
	}
	filter := topicFilter(topics)
	// 每路各取 topK*3 候选交给 RRF，提升召回覆盖面
	perPath := topK * 3

	q := strings.TrimSpace(query)
	hasText := len([]rune(q)) >= 1
	hasVec := len(vec) > 0

	// 退化场景：缺 query 文本或 dense 向量时退回单路检索
	if !hasText && hasVec {
		return s.denseSearch(ctx, vec, topK, filter)
	}
	if hasText && !hasVec {
		return s.bm25Search(ctx, q, topK, filter)
	}
	if !hasText && !hasVec {
		return nil, nil
	}

	denseReq := milvusclient.NewAnnRequest(fieldEmbedding, perPath, entity.FloatVector(vec))
	if filter != "" {
		denseReq = denseReq.WithFilter(filter)
	}
	bm25Req := milvusclient.NewAnnRequest(fieldSparse, perPath, entity.Text(q))
	if filter != "" {
		bm25Req = bm25Req.WithFilter(filter)
	}

	opt := milvusclient.NewHybridSearchOption(s.collection, topK, denseReq, bm25Req).
		WithReranker(milvusclient.NewRRFReranker()).
		WithOutputFields(searchOutputFields()...)

	results, err := cli.HybridSearch(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("milvus hybrid search: %w", err)
	}
	return resultSetsToHits(results), nil
}

// denseSearch 仅 dense 向量检索（退化路径）。
func (s *Service) denseSearch(ctx context.Context, vec []float32, topK int, filter string) ([]SearchHit, error) {
	cli, _ := s.connect(ctx)
	opt := milvusclient.NewSearchOption(s.collection, topK, []entity.Vector{entity.FloatVector(vec)}).
		WithANNSField(fieldEmbedding).
		WithOutputFields(searchOutputFields()...)
	if filter != "" {
		opt = opt.WithFilter(filter)
	}
	results, err := cli.Search(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("milvus dense search: %w", err)
	}
	return resultSetsToHits(results), nil
}

// bm25Search 仅 BM25 稀疏检索（退化路径）。
func (s *Service) bm25Search(ctx context.Context, query string, topK int, filter string) ([]SearchHit, error) {
	cli, _ := s.connect(ctx)
	opt := milvusclient.NewSearchOption(s.collection, topK, []entity.Vector{entity.Text(query)}).
		WithANNSField(fieldSparse).
		WithOutputFields(searchOutputFields()...)
	if filter != "" {
		opt = opt.WithFilter(filter)
	}
	results, err := cli.Search(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("milvus bm25 search: %w", err)
	}
	return resultSetsToHits(results), nil
}

// resultSetsToHits 将搜索结果集解析为 SearchHit。
func resultSetsToHits(results []milvusclient.ResultSet) []SearchHit {
	var hits []SearchHit
	for _, rs := range results {
		n := rs.ResultCount
		for i := 0; i < n; i++ {
			hit := SearchHit{}
			if rs.IDs != nil {
				if v, e := rs.IDs.GetAsString(i); e == nil {
					hit.ChunkPK = v
				}
			}
			if i < len(rs.Scores) {
				hit.Score = rs.Scores[i]
			}
			hit.ArticleID = colInt64(rs, fieldArticleID, i)
			hit.ChunkIdx = colInt64(rs, fieldChunkIdx, i)
			hit.Title = colString(rs, fieldTitle, i)
			hit.Snippet = colString(rs, fieldSnippet, i)
			hit.Platform = colString(rs, fieldPlatform, i)
			hit.ChunkType = colString(rs, fieldChunkType, i)
			hit.Topic = colString(rs, fieldTopic, i)
			hits = append(hits, hit)
		}
	}
	return hits
}

func colString(rs milvusclient.ResultSet, field string, idx int) string {
	col := rs.GetColumn(field)
	if col == nil {
		return ""
	}
	if v, err := col.GetAsString(idx); err == nil {
		return v
	}
	return ""
}

func colInt64(rs milvusclient.ResultSet, field string, idx int) int64 {
	col := rs.GetColumn(field)
	if col == nil {
		return 0
	}
	if v, err := col.GetAsInt64(idx); err == nil {
		return v
	}
	return 0
}

// QueryByArticle 按 article_id 标量查询，返回该文章所有 chunks（不做向量运算）。
func (s *Service) QueryByArticle(ctx context.Context, articleID int64) ([]SearchHit, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := cli.Query(ctx, milvusclient.NewQueryOption(s.collection).
		WithFilter(fmt.Sprintf("%s == %d", fieldArticleID, articleID)).
		WithOutputFields(append([]string{fieldChunkPK}, searchOutputFields()...)...).
		WithLimit(1000))
	if err != nil {
		return nil, fmt.Errorf("milvus query: %w", err)
	}
	return queryResultToHits(rs), nil
}

// QueryByPK 查询单条 chunk（用于 update snippet 前读旧数据）。
func (s *Service) QueryByPK(ctx context.Context, pk string) (*SearchHit, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := cli.Query(ctx, milvusclient.NewQueryOption(s.collection).
		WithFilter(fmt.Sprintf("%s == %q", fieldChunkPK, pk)).
		WithOutputFields(append([]string{fieldChunkPK}, searchOutputFields()...)...).
		WithLimit(1))
	if err != nil {
		return nil, err
	}
	hits := queryResultToHits(rs)
	if len(hits) == 0 {
		return nil, nil
	}
	return &hits[0], nil
}

// queryResultToHits 将标量查询结果集解析为 SearchHit（含 chunk_pk）。
func queryResultToHits(rs milvusclient.ResultSet) []SearchHit {
	n := rs.ResultCount
	hits := make([]SearchHit, 0, n)
	for i := 0; i < n; i++ {
		hits = append(hits, SearchHit{
			ChunkPK:   colString(rs, fieldChunkPK, i),
			ArticleID: colInt64(rs, fieldArticleID, i),
			ChunkIdx:  colInt64(rs, fieldChunkIdx, i),
			Title:     colString(rs, fieldTitle, i),
			Snippet:   colString(rs, fieldSnippet, i),
			Platform:  colString(rs, fieldPlatform, i),
			ChunkType: colString(rs, fieldChunkType, i),
			Topic:     colString(rs, fieldTopic, i),
		})
	}
	return hits
}

// HasCollection 检查集合是否存在。
func (s *Service) HasCollection(ctx context.Context) (bool, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return false, err
	}
	return cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(s.collection))
}

// DropCollection 删除集合（rebuild 用）。
func (s *Service) DropCollection(ctx context.Context) error {
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	exists, err := cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(s.collection))
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return cli.DropCollection(ctx, milvusclient.NewDropCollectionOption(s.collection))
}

// Ping 连通性检查。
func (s *Service) Ping(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cli, err := s.connect(ctx)
	if err != nil {
		return err
	}
	_, err = cli.ListCollections(ctx, milvusclient.NewListCollectionOption())
	return err
}

// CollectionName 返回当前集合名。
func (s *Service) CollectionName() string { return s.collection }

// ListTopics 列出集合中所有不同的 topic 值（通过查询去重）。
func (s *Service) ListTopics(ctx context.Context) ([]string, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := cli.Query(ctx, milvusclient.NewQueryOption(s.collection).
		WithOutputFields(fieldTopic).
		WithLimit(16384))
	if err != nil {
		return nil, fmt.Errorf("query topics: %w", err)
	}
	col := rs.GetColumn(fieldTopic)
	if col == nil {
		return nil, nil
	}
	topicSet := make(map[string]struct{})
	for i := 0; i < col.Len(); i++ {
		if v, e := col.GetAsString(i); e == nil && v != "" && v != defaultPartition {
			topicSet[v] = struct{}{}
		}
	}
	topics := make([]string, 0, len(topicSet))
	for t := range topicSet {
		topics = append(topics, t)
	}
	return topics, nil
}

// CountByTopic 统计指定 topic 的 chunk 数量。
func (s *Service) CountByTopic(ctx context.Context, topic string) (int64, error) {
	cli, err := s.connect(ctx)
	if err != nil {
		return 0, err
	}
	rs, err := cli.Query(ctx, milvusclient.NewQueryOption(s.collection).
		WithFilter(fmt.Sprintf("%s == %q", fieldTopic, topic)).
		WithOutputFields(fieldChunkPK).
		WithLimit(100000))
	if err != nil {
		return 0, err
	}
	return int64(rs.ResultCount), nil
}

func clip(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
