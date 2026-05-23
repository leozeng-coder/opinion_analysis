package model

// MediaCrawler 平台表模型定义
// 这些表由 MediaCrawler 爬虫模块使用，存储各平台的原始数据

// BilibiliVideo B站视频信息
type BilibiliVideo struct {
	ID                  uint   `gorm:"primarykey" json:"id"`
	VideoID             int64  `gorm:"not null;index;unique" json:"videoId"`
	VideoURL            string `gorm:"type:text" json:"videoUrl"`
	UserID              int64  `gorm:"index" json:"userId"`
	Nickname            string `gorm:"type:text" json:"nickname"`
	Avatar              string `gorm:"type:text" json:"avatar"`
	LikedCount          int    `json:"likedCount"`
	AddTs               int64  `json:"addTs"`
	LastModifyTs        int64  `json:"lastModifyTs"`
	VideoType           string `gorm:"type:text" json:"videoType"`
	Title               string `gorm:"type:text" json:"title"`
	Desc                string `gorm:"type:text" json:"desc"`
	CreateTime          int64  `gorm:"index" json:"createTime"`
	DislikedCount       string `gorm:"type:text" json:"dislikedCount"`
	VideoPlayCount      string `gorm:"type:text" json:"videoPlayCount"`
	VideoFavoriteCount  string `gorm:"type:text" json:"videoFavoriteCount"`
	VideoShareCount     string `gorm:"type:text" json:"videoShareCount"`
	VideoCoinCount      string `gorm:"type:text" json:"videoCoinCount"`
	VideoDanmaku        string `gorm:"type:text" json:"videoDanmaku"`
	VideoComment        string `gorm:"type:text" json:"videoComment"`
	VideoCoverURL       string `gorm:"type:text" json:"videoCoverUrl"`
	SourceKeyword       string `gorm:"type:text" json:"sourceKeyword"`
}

func (BilibiliVideo) TableName() string {
	return "bilibili_video"
}

// BilibiliVideoComment B站视频评论
type BilibiliVideoComment struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	UserID          string `gorm:"size:255" json:"userId"`
	Nickname        string `gorm:"type:text" json:"nickname"`
	Sex             string `gorm:"type:text" json:"sex"`
	Sign            string `gorm:"type:text" json:"sign"`
	Avatar          string `gorm:"type:text" json:"avatar"`
	AddTs           int64  `json:"addTs"`
	LastModifyTs    int64  `json:"lastModifyTs"`
	CommentID       int64  `gorm:"index" json:"commentId"`
	VideoID         int64  `gorm:"index" json:"videoId"`
	Content         string `gorm:"type:text" json:"content"`
	CreateTime      int64  `json:"createTime"`
	SubCommentCount string `gorm:"type:text" json:"subCommentCount"`
	ParentCommentID string `gorm:"size:255" json:"parentCommentId"`
	LikeCount       string `gorm:"type:text" json:"likeCount"`
}

func (BilibiliVideoComment) TableName() string {
	return "bilibili_video_comment"
}

// BilibiliUpInfo B站UP主信息
type BilibiliUpInfo struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	UserID       int64  `gorm:"index" json:"userId"`
	Nickname     string `gorm:"type:text" json:"nickname"`
	Sex          string `gorm:"type:text" json:"sex"`
	Sign         string `gorm:"type:text" json:"sign"`
	Avatar       string `gorm:"type:text" json:"avatar"`
	AddTs        int64  `json:"addTs"`
	LastModifyTs int64  `json:"lastModifyTs"`
	TotalFans    int    `json:"totalFans"`
	TotalLiked   int    `json:"totalLiked"`
	UserRank     int    `json:"userRank"`
	IsOfficial   int    `json:"isOfficial"`
}

func (BilibiliUpInfo) TableName() string {
	return "bilibili_up_info"
}

// BilibiliContactInfo B站联系人信息
type BilibiliContactInfo struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	UpID         int64  `gorm:"index" json:"upId"`
	FanID        int64  `gorm:"index" json:"fanId"`
	UpName       string `gorm:"type:text" json:"upName"`
	FanName      string `gorm:"type:text" json:"fanName"`
	UpSign       string `gorm:"type:text" json:"upSign"`
	FanSign      string `gorm:"type:text" json:"fanSign"`
	UpAvatar     string `gorm:"type:text" json:"upAvatar"`
	FanAvatar    string `gorm:"type:text" json:"fanAvatar"`
	AddTs        int64  `json:"addTs"`
	LastModifyTs int64  `json:"lastModifyTs"`
}

func (BilibiliContactInfo) TableName() string {
	return "bilibili_contact_info"
}

// BilibiliUpDynamic B站UP主动态
type BilibiliUpDynamic struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	DynamicID     int64  `gorm:"index" json:"dynamicId"`
	UserID        string `gorm:"size:255" json:"userId"`
	UserName      string `gorm:"type:text" json:"userName"`
	Text          string `gorm:"type:text" json:"text"`
	Type          string `gorm:"type:text" json:"type"`
	PubTs         int64  `json:"pubTs"`
	TotalComments int    `json:"totalComments"`
	TotalForwards int    `json:"totalForwards"`
	TotalLiked    int    `json:"totalLiked"`
	AddTs         int64  `json:"addTs"`
	LastModifyTs  int64  `json:"lastModifyTs"`
}

func (BilibiliUpDynamic) TableName() string {
	return "bilibili_up_dynamic"
}

// DouyinAweme 抖音作品信息
type DouyinAweme struct {
	ID               uint   `gorm:"primarykey" json:"id"`
	UserID           string `gorm:"size:255" json:"userId"`
	SecUID           string `gorm:"size:255" json:"secUid"`
	ShortUserID      string `gorm:"size:255" json:"shortUserId"`
	UserUniqueID     string `gorm:"size:255" json:"userUniqueId"`
	Nickname         string `gorm:"type:text" json:"nickname"`
	Avatar           string `gorm:"type:text" json:"avatar"`
	UserSignature    string `gorm:"type:text" json:"userSignature"`
	IPLocation       string `gorm:"type:text" json:"ipLocation"`
	AddTs            int64  `json:"addTs"`
	LastModifyTs     int64  `json:"lastModifyTs"`
	AwemeID          int64  `gorm:"index" json:"awemeId"`
	AwemeType        string `gorm:"type:text" json:"awemeType"`
	Title            string `gorm:"type:text" json:"title"`
	Desc             string `gorm:"type:text" json:"desc"`
	CreateTime       int64  `gorm:"index" json:"createTime"`
	LikedCount       string `gorm:"type:text" json:"likedCount"`
	CommentCount     string `gorm:"type:text" json:"commentCount"`
	ShareCount       string `gorm:"type:text" json:"shareCount"`
	CollectedCount   string `gorm:"type:text" json:"collectedCount"`
	AwemeURL         string `gorm:"type:text" json:"awemeUrl"`
	CoverURL         string `gorm:"type:text" json:"coverUrl"`
	VideoDownloadURL string `gorm:"type:text" json:"videoDownloadUrl"`
	MusicDownloadURL string `gorm:"type:text" json:"musicDownloadUrl"`
	NoteDownloadURL  string `gorm:"type:text" json:"noteDownloadUrl"`
	SourceKeyword    string `gorm:"type:text" json:"sourceKeyword"`
}

func (DouyinAweme) TableName() string {
	return "douyin_aweme"
}

// DouyinAwemeComment 抖音作品评论
type DouyinAwemeComment struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	UserID          string `gorm:"size:255" json:"userId"`
	SecUID          string `gorm:"size:255" json:"secUid"`
	ShortUserID     string `gorm:"size:255" json:"shortUserId"`
	UserUniqueID    string `gorm:"size:255" json:"userUniqueId"`
	Nickname        string `gorm:"type:text" json:"nickname"`
	Avatar          string `gorm:"type:text" json:"avatar"`
	UserSignature   string `gorm:"type:text" json:"userSignature"`
	IPLocation      string `gorm:"type:text" json:"ipLocation"`
	AddTs           int64  `json:"addTs"`
	LastModifyTs    int64  `json:"lastModifyTs"`
	CommentID       int64  `gorm:"index" json:"commentId"`
	AwemeID         int64  `gorm:"index" json:"awemeId"`
	Content         string `gorm:"type:text" json:"content"`
	CreateTime      int64  `json:"createTime"`
	SubCommentCount string `gorm:"type:text" json:"subCommentCount"`
	ParentCommentID string `gorm:"size:255" json:"parentCommentId"`
	LikeCount       string `gorm:"type:text" json:"likeCount"`
	Pictures        string `gorm:"type:text" json:"pictures"`
}

func (DouyinAwemeComment) TableName() string {
	return "douyin_aweme_comment"
}

// DyCreator 抖音创作者信息
type DyCreator struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	UserID       string `gorm:"size:255" json:"userId"`
	Nickname     string `gorm:"type:text" json:"nickname"`
	Avatar       string `gorm:"type:text" json:"avatar"`
	IPLocation   string `gorm:"type:text" json:"ipLocation"`
	AddTs        int64  `json:"addTs"`
	LastModifyTs int64  `json:"lastModifyTs"`
	Desc         string `gorm:"type:text" json:"desc"`
	Gender       string `gorm:"type:text" json:"gender"`
	Follows      string `gorm:"type:text" json:"follows"`
	Fans         string `gorm:"type:text" json:"fans"`
	Interaction  string `gorm:"type:text" json:"interaction"`
	VideosCount  string `gorm:"size:255" json:"videosCount"`
}

func (DyCreator) TableName() string {
	return "dy_creator"
}

// KuaishouVideo 快手视频信息
type KuaishouVideo struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	UserID        string `gorm:"size:64" json:"userId"`
	Nickname      string `gorm:"type:text" json:"nickname"`
	Avatar        string `gorm:"type:text" json:"avatar"`
	AddTs         int64  `json:"addTs"`
	LastModifyTs  int64  `json:"lastModifyTs"`
	VideoID       string `gorm:"size:255;index" json:"videoId"`
	VideoType     string `gorm:"type:text" json:"videoType"`
	Title         string `gorm:"type:text" json:"title"`
	Desc          string `gorm:"type:text" json:"desc"`
	CreateTime    int64  `gorm:"index" json:"createTime"`
	LikedCount    string `gorm:"type:text" json:"likedCount"`
	ViewdCount    string `gorm:"type:text" json:"viewdCount"`
	VideoURL      string `gorm:"type:text" json:"videoUrl"`
	VideoCoverURL string `gorm:"type:text" json:"videoCoverUrl"`
	VideoPlayURL  string `gorm:"type:text" json:"videoPlayUrl"`
	SourceKeyword string `gorm:"type:text" json:"sourceKeyword"`
}

func (KuaishouVideo) TableName() string {
	return "kuaishou_video"
}

// KuaishouVideoComment 快手视频评论
type KuaishouVideoComment struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	UserID          string `gorm:"type:text" json:"userId"`
	Nickname        string `gorm:"type:text" json:"nickname"`
	Avatar          string `gorm:"type:text" json:"avatar"`
	AddTs           int64  `json:"addTs"`
	LastModifyTs    int64  `json:"lastModifyTs"`
	CommentID       int64  `gorm:"index" json:"commentId"`
	VideoID         string `gorm:"size:255;index" json:"videoId"`
	Content         string `gorm:"type:text" json:"content"`
	CreateTime      int64  `json:"createTime"`
	SubCommentCount string `gorm:"type:text" json:"subCommentCount"`
}

func (KuaishouVideoComment) TableName() string {
	return "kuaishou_video_comment"
}

// WeiboNote 微博笔记信息
type WeiboNote struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	UserID         string `gorm:"size:255" json:"userId"`
	Nickname       string `gorm:"type:text" json:"nickname"`
	Avatar         string `gorm:"type:text" json:"avatar"`
	Gender         string `gorm:"type:text" json:"gender"`
	ProfileURL     string `gorm:"type:text" json:"profileUrl"`
	IPLocation     string `gorm:"type:text" json:"ipLocation"`
	AddTs          int64  `json:"addTs"`
	LastModifyTs   int64  `json:"lastModifyTs"`
	NoteID         int64  `gorm:"index" json:"noteId"`
	Content        string `gorm:"type:text" json:"content"`
	CreateTime     int64  `gorm:"index" json:"createTime"`
	CreateDateTime string `gorm:"size:255;index" json:"createDateTime"`
	LikedCount     string `gorm:"type:text" json:"likedCount"`
	CommentsCount  string `gorm:"type:text" json:"commentsCount"`
	SharedCount    string `gorm:"type:text" json:"sharedCount"`
	NoteURL        string `gorm:"type:text" json:"noteUrl"`
	SourceKeyword  string `gorm:"type:text" json:"sourceKeyword"`
}

func (WeiboNote) TableName() string {
	return "weibo_note"
}

// WeiboNoteComment 微博笔记评论
type WeiboNoteComment struct {
	ID               uint   `gorm:"primarykey" json:"id"`
	UserID           string `gorm:"size:255" json:"userId"`
	Nickname         string `gorm:"type:text" json:"nickname"`
	Avatar           string `gorm:"type:text" json:"avatar"`
	Gender           string `gorm:"type:text" json:"gender"`
	ProfileURL       string `gorm:"type:text" json:"profileUrl"`
	IPLocation       string `gorm:"type:text" json:"ipLocation"`
	AddTs            int64  `json:"addTs"`
	LastModifyTs     int64  `json:"lastModifyTs"`
	CommentID        int64  `gorm:"index" json:"commentId"`
	NoteID           int64  `gorm:"index" json:"noteId"`
	Content          string `gorm:"type:text" json:"content"`
	CreateTime       int64  `json:"createTime"`
	CreateDateTime   string `gorm:"size:255;index" json:"createDateTime"`
	CommentLikeCount string `gorm:"type:text" json:"commentLikeCount"`
	SubCommentCount  string `gorm:"type:text" json:"subCommentCount"`
	ParentCommentID  string `gorm:"size:255" json:"parentCommentId"`
}

func (WeiboNoteComment) TableName() string {
	return "weibo_note_comment"
}

// WeiboCreator 微博创作者信息
type WeiboCreator struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	UserID       string `gorm:"size:255" json:"userId"`
	Nickname     string `gorm:"type:text" json:"nickname"`
	Avatar       string `gorm:"type:text" json:"avatar"`
	IPLocation   string `gorm:"type:text" json:"ipLocation"`
	AddTs        int64  `json:"addTs"`
	LastModifyTs int64  `json:"lastModifyTs"`
	Desc         string `gorm:"type:text" json:"desc"`
	Gender       string `gorm:"type:text" json:"gender"`
	Follows      string `gorm:"type:text" json:"follows"`
	Fans         string `gorm:"type:text" json:"fans"`
	TagList      string `gorm:"type:text" json:"tagList"`
}

func (WeiboCreator) TableName() string {
	return "weibo_creator"
}

// XhsCreator 小红书创作者信息
type XhsCreator struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	UserID       string `gorm:"size:255" json:"userId"`
	Nickname     string `gorm:"type:text" json:"nickname"`
	Avatar       string `gorm:"type:text" json:"avatar"`
	IPLocation   string `gorm:"type:text" json:"ipLocation"`
	AddTs        int64  `json:"addTs"`
	LastModifyTs int64  `json:"lastModifyTs"`
	Desc         string `gorm:"type:text" json:"desc"`
	Gender       string `gorm:"type:text" json:"gender"`
	Follows      string `gorm:"type:text" json:"follows"`
	Fans         string `gorm:"type:text" json:"fans"`
	Interaction  string `gorm:"type:text" json:"interaction"`
	TagList      string `gorm:"type:text" json:"tagList"`
}

func (XhsCreator) TableName() string {
	return "xhs_creator"
}

// XhsNote 小红书笔记信息
type XhsNote struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	UserID         string `gorm:"size:255" json:"userId"`
	Nickname       string `gorm:"type:text" json:"nickname"`
	Avatar         string `gorm:"type:text" json:"avatar"`
	IPLocation     string `gorm:"type:text" json:"ipLocation"`
	AddTs          int64  `json:"addTs"`
	LastModifyTs   int64  `json:"lastModifyTs"`
	NoteID         string `gorm:"size:255;index" json:"noteId"`
	Type           string `gorm:"type:text" json:"type"`
	Title          string `gorm:"type:text" json:"title"`
	Desc           string `gorm:"type:text" json:"desc"`
	VideoURL       string `gorm:"type:text" json:"videoUrl"`
	Time           int64  `gorm:"index" json:"time"`
	LastUpdateTime int64  `json:"lastUpdateTime"`
	LikedCount     string `gorm:"type:text" json:"likedCount"`
	CollectedCount string `gorm:"type:text" json:"collectedCount"`
	CommentCount   string `gorm:"type:text" json:"commentCount"`
	ShareCount     string `gorm:"type:text" json:"shareCount"`
	ImageList      string `gorm:"type:text" json:"imageList"`
	TagList        string `gorm:"type:text" json:"tagList"`
	NoteURL        string `gorm:"type:text" json:"noteUrl"`
	SourceKeyword  string `gorm:"type:text" json:"sourceKeyword"`
	XsecToken      string `gorm:"type:text" json:"xsecToken"`
}

func (XhsNote) TableName() string {
	return "xhs_note"
}

// XhsNoteComment 小红书笔记评论
type XhsNoteComment struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	UserID          string `gorm:"size:255" json:"userId"`
	Nickname        string `gorm:"type:text" json:"nickname"`
	Avatar          string `gorm:"type:text" json:"avatar"`
	IPLocation      string `gorm:"type:text" json:"ipLocation"`
	AddTs           int64  `json:"addTs"`
	LastModifyTs    int64  `json:"lastModifyTs"`
	CommentID       string `gorm:"size:255;index" json:"commentId"`
	CreateTime      int64  `gorm:"index" json:"createTime"`
	NoteID          string `gorm:"size:255" json:"noteId"`
	Content         string `gorm:"type:text" json:"content"`
	SubCommentCount int    `json:"subCommentCount"`
	Pictures        string `gorm:"type:text" json:"pictures"`
	ParentCommentID string `gorm:"size:255" json:"parentCommentId"`
	LikeCount       string `gorm:"type:text" json:"likeCount"`
}

func (XhsNoteComment) TableName() string {
	return "xhs_note_comment"
}

// TiebaNote 贴吧帖子信息
type TiebaNote struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	NoteID         string `gorm:"size:644;index" json:"noteId"`
	Title          string `gorm:"type:text" json:"title"`
	Desc           string `gorm:"type:text" json:"desc"`
	NoteURL        string `gorm:"type:text" json:"noteUrl"`
	PublishTime    string `gorm:"size:255;index" json:"publishTime"`
	UserLink       string `gorm:"type:text" json:"userLink"`
	UserNickname   string `gorm:"type:text" json:"userNickname"`
	UserAvatar     string `gorm:"type:text" json:"userAvatar"`
	TiebaID        string `gorm:"size:255" json:"tiebaId"`
	TiebaName      string `gorm:"type:text" json:"tiebaName"`
	TiebaLink      string `gorm:"type:text" json:"tiebaLink"`
	TotalReplayNum int    `gorm:"default:0" json:"totalReplayNum"`
	TotalReplayPage int   `gorm:"default:0" json:"totalReplayPage"`
	IPLocation     string `gorm:"type:text" json:"ipLocation"`
	AddTs          int64  `json:"addTs"`
	LastModifyTs   int64  `json:"lastModifyTs"`
	SourceKeyword  string `gorm:"type:text" json:"sourceKeyword"`
}

func (TiebaNote) TableName() string {
	return "tieba_note"
}

// TiebaComment 贴吧评论
type TiebaComment struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	CommentID       string `gorm:"size:255;index" json:"commentId"`
	ParentCommentID string `gorm:"size:255" json:"parentCommentId"`
	Content         string `gorm:"type:text" json:"content"`
	UserLink        string `gorm:"type:text" json:"userLink"`
	UserNickname    string `gorm:"type:text" json:"userNickname"`
	UserAvatar      string `gorm:"type:text" json:"userAvatar"`
	TiebaID         string `gorm:"size:255" json:"tiebaId"`
	TiebaName       string `gorm:"type:text" json:"tiebaName"`
	TiebaLink       string `gorm:"type:text" json:"tiebaLink"`
	PublishTime     string `gorm:"size:255;index" json:"publishTime"`
	IPLocation      string `gorm:"type:text" json:"ipLocation"`
	SubCommentCount int    `gorm:"default:0" json:"subCommentCount"`
	NoteID          string `gorm:"size:255;index" json:"noteId"`
	NoteURL         string `gorm:"type:text" json:"noteUrl"`
	AddTs           int64  `json:"addTs"`
	LastModifyTs    int64  `json:"lastModifyTs"`
}

func (TiebaComment) TableName() string {
	return "tieba_comment"
}

// TiebaCreator 贴吧创作者信息
type TiebaCreator struct {
	ID                   uint   `gorm:"primarykey" json:"id"`
	UserID               string `gorm:"size:64" json:"userId"`
	UserName             string `gorm:"type:text" json:"userName"`
	Nickname             string `gorm:"type:text" json:"nickname"`
	Avatar               string `gorm:"type:text" json:"avatar"`
	IPLocation           string `gorm:"type:text" json:"ipLocation"`
	AddTs                int64  `json:"addTs"`
	LastModifyTs         int64  `json:"lastModifyTs"`
	Gender               string `gorm:"type:text" json:"gender"`
	Follows              string `gorm:"type:text" json:"follows"`
	Fans                 string `gorm:"type:text" json:"fans"`
	RegistrationDuration string `gorm:"type:text" json:"registrationDuration"`
}

func (TiebaCreator) TableName() string {
	return "tieba_creator"
}

// ZhihuContent 知乎内容信息
type ZhihuContent struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	ContentID     string `gorm:"size:64;index" json:"contentId"`
	ContentType   string `gorm:"type:text" json:"contentType"`
	ContentText   string `gorm:"type:text" json:"contentText"`
	ContentURL    string `gorm:"type:text" json:"contentUrl"`
	QuestionID    string `gorm:"size:255" json:"questionId"`
	Title         string `gorm:"type:text" json:"title"`
	Desc          string `gorm:"type:text" json:"desc"`
	CreatedTime   string `gorm:"size:32;index" json:"createdTime"`
	UpdatedTime   string `gorm:"type:text" json:"updatedTime"`
	VoteupCount   int    `gorm:"default:0" json:"voteupCount"`
	CommentCount  int    `gorm:"default:0" json:"commentCount"`
	SourceKeyword string `gorm:"type:text" json:"sourceKeyword"`
	UserID        string `gorm:"size:255" json:"userId"`
	UserLink      string `gorm:"type:text" json:"userLink"`
	UserNickname  string `gorm:"type:text" json:"userNickname"`
	UserAvatar    string `gorm:"type:text" json:"userAvatar"`
	UserURLToken  string `gorm:"type:text" json:"userUrlToken"`
	AddTs         int64  `json:"addTs"`
	LastModifyTs  int64  `json:"lastModifyTs"`
}

func (ZhihuContent) TableName() string {
	return "zhihu_content"
}

// ZhihuComment 知乎评论
type ZhihuComment struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	CommentID       string `gorm:"size:64;index" json:"commentId"`
	ParentCommentID string `gorm:"size:64" json:"parentCommentId"`
	Content         string `gorm:"type:text" json:"content"`
	PublishTime     string `gorm:"size:32;index" json:"publishTime"`
	IPLocation      string `gorm:"type:text" json:"ipLocation"`
	SubCommentCount int    `gorm:"default:0" json:"subCommentCount"`
	LikeCount       int    `gorm:"default:0" json:"likeCount"`
	DislikeCount    int    `gorm:"default:0" json:"dislikeCount"`
	ContentID       string `gorm:"size:64;index" json:"contentId"`
	ContentType     string `gorm:"type:text" json:"contentType"`
	UserID          string `gorm:"size:64" json:"userId"`
	UserLink        string `gorm:"type:text" json:"userLink"`
	UserNickname    string `gorm:"type:text" json:"userNickname"`
	UserAvatar      string `gorm:"type:text" json:"userAvatar"`
	AddTs           int64  `json:"addTs"`
	LastModifyTs    int64  `json:"lastModifyTs"`
}

func (ZhihuComment) TableName() string {
	return "zhihu_comment"
}

// ZhihuCreator 知乎创作者信息
type ZhihuCreator struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	UserID         string `gorm:"size:64;unique;index" json:"userId"`
	UserLink       string `gorm:"type:text" json:"userLink"`
	UserNickname   string `gorm:"type:text" json:"userNickname"`
	UserAvatar     string `gorm:"type:text" json:"userAvatar"`
	URLToken       string `gorm:"type:text" json:"urlToken"`
	Gender         string `gorm:"type:text" json:"gender"`
	IPLocation     string `gorm:"type:text" json:"ipLocation"`
	Follows        int    `gorm:"default:0" json:"follows"`
	Fans           int    `gorm:"default:0" json:"fans"`
	AnwserCount    int    `gorm:"default:0" json:"anwserCount"`
	VideoCount     int    `gorm:"default:0" json:"videoCount"`
	QuestionCount  int    `gorm:"default:0" json:"questionCount"`
	ArticleCount   int    `gorm:"default:0" json:"articleCount"`
	ColumnCount    int    `gorm:"default:0" json:"columnCount"`
	GetVoteupCount int    `gorm:"default:0" json:"getVoteupCount"`
	AddTs          int64  `json:"addTs"`
	LastModifyTs   int64  `json:"lastModifyTs"`
}

func (ZhihuCreator) TableName() string {
	return "zhihu_creator"
}
