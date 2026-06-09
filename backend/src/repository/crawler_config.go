package repository

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/model"
)

// CrawlerConfigData 爬虫运行时配置（来自 system_settings）。
type CrawlerConfigData struct {
	// 性能
	MaxNotesCount       int
	MaxCommentsCount    int
	MaxSubCommentsCount int
	MaxConcurrency      int
	SleepSecMin         int
	SleepSecMax         int
	// IP 代理
	EnableIPProxy    bool
	IPProxyPoolCount int
	IPProxyProvider  string
	ProxyKdlSecretID  string
	ProxyKdlSignature string
	ProxyKdlUsername  string
	ProxyKdlPassword  string
	ProxyWandouAppKey string
	// 平台排序
	XhsSortType     string
	WeiboSearchType string
	DySortType      int
	ZhihuSort       string
	ZhihuSearchTime string
	// Cookie（明文存储，读取时脱敏展示）
	CookieXhs   string
	CookieDy    string
	CookieKs    string
	CookieBili  string
	CookieWb    string
	CookieTieba string
	CookieZhihu string
}

var crawlerConfigKeys = []string{
	"crawler.max_notes_count",
	"crawler.max_comments_count",
	"crawler.max_sub_comments_count",
	"crawler.max_concurrency_num",
	"crawler.sleep_sec_min",
	"crawler.sleep_sec_max",
	"crawler.enable_ip_proxy",
	"crawler.ip_proxy_pool_count",
	"crawler.ip_proxy_provider",
	"crawler.proxy_kdl_secret_id",
	"crawler.proxy_kdl_signature",
	"crawler.proxy_kdl_username",
	"crawler.proxy_kdl_password",
	"crawler.proxy_wandou_app_key",
	"crawler.xhs.sort_type",
	"crawler.wb.search_type",
	"crawler.dy.sort_type",
	"crawler.zhihu.sort",
	"crawler.zhihu.search_time",
	"crawler.cookie.xhs",
	"crawler.cookie.dy",
	"crawler.cookie.ks",
	"crawler.cookie.bili",
	"crawler.cookie.wb",
	"crawler.cookie.tieba",
	"crawler.cookie.zhihu",
}

func crawlerConfigDefaults() map[string]string {
	return map[string]string{
		"crawler.max_notes_count":        "50",
		"crawler.max_comments_count":     "50",
		"crawler.max_sub_comments_count": "20",
		"crawler.max_concurrency_num":    "3",
		"crawler.sleep_sec_min":      "1",
		"crawler.sleep_sec_max":      "3",
		"crawler.enable_ip_proxy":    "false",
		"crawler.ip_proxy_pool_count": "10",
		"crawler.ip_proxy_provider":  "kuaidaili",
		"crawler.proxy_kdl_secret_id":  "",
		"crawler.proxy_kdl_signature":  "",
		"crawler.proxy_kdl_username":   "",
		"crawler.proxy_kdl_password":   "",
		"crawler.proxy_wandou_app_key": "",
		"crawler.xhs.sort_type":      "time_descending",
		"crawler.wb.search_type":     "real_time",
		"crawler.dy.sort_type":       "2",
		"crawler.zhihu.sort":         "created_time",
		"crawler.zhihu.search_time":  "",
		"crawler.cookie.xhs":   "",
		"crawler.cookie.dy":    "",
		"crawler.cookie.ks":    "",
		"crawler.cookie.bili":  "",
		"crawler.cookie.wb":    "",
		"crawler.cookie.tieba": "",
		"crawler.cookie.zhihu": "",
	}
}

func isCrawlerSensitiveKey(key string) bool {
	return strings.Contains(key, "cookie") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "signature") ||
		strings.Contains(key, "app_key")
}

func (r *SystemRepository) loadCrawlerSettingMap() (map[string]string, error) {
	defaults := crawlerConfigDefaults()
	out := make(map[string]string, len(defaults))
	for k, v := range defaults {
		out[k] = v
	}
	var rows []model.SystemSetting
	if err := r.db.Where("`key` LIKE ?", "crawler.%").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func mapToCrawlerConfigData(m map[string]string) CrawlerConfigData {
	d := crawlerConfigDefaults()
	for k, v := range m {
		d[k] = v
	}
	return CrawlerConfigData{
		MaxNotesCount:       parseIntSetting(d["crawler.max_notes_count"], 50),
		MaxCommentsCount:    parseIntSetting(d["crawler.max_comments_count"], 50),
		MaxSubCommentsCount: parseIntSetting(d["crawler.max_sub_comments_count"], 20),
		MaxConcurrency:      parseIntSetting(d["crawler.max_concurrency_num"], 3),
		SleepSecMin:       parseIntSetting(d["crawler.sleep_sec_min"], 1),
		SleepSecMax:       parseIntSetting(d["crawler.sleep_sec_max"], 3),
		EnableIPProxy:     parseBoolSetting(d["crawler.enable_ip_proxy"]),
		IPProxyPoolCount:  parseIntSetting(d["crawler.ip_proxy_pool_count"], 10),
		IPProxyProvider:   strings.TrimSpace(d["crawler.ip_proxy_provider"]),
		ProxyKdlSecretID:  d["crawler.proxy_kdl_secret_id"],
		ProxyKdlSignature: d["crawler.proxy_kdl_signature"],
		ProxyKdlUsername:  d["crawler.proxy_kdl_username"],
		ProxyKdlPassword:  d["crawler.proxy_kdl_password"],
		ProxyWandouAppKey: d["crawler.proxy_wandou_app_key"],
		XhsSortType:       strings.TrimSpace(d["crawler.xhs.sort_type"]),
		WeiboSearchType:   strings.TrimSpace(d["crawler.wb.search_type"]),
		DySortType:        parseIntSetting(d["crawler.dy.sort_type"], 2),
		ZhihuSort:         strings.TrimSpace(d["crawler.zhihu.sort"]),
		ZhihuSearchTime:   strings.TrimSpace(d["crawler.zhihu.search_time"]),
		CookieXhs:   d["crawler.cookie.xhs"],
		CookieDy:    d["crawler.cookie.dy"],
		CookieKs:    d["crawler.cookie.ks"],
		CookieBili:  d["crawler.cookie.bili"],
		CookieWb:    d["crawler.cookie.wb"],
		CookieTieba: d["crawler.cookie.tieba"],
		CookieZhihu: d["crawler.cookie.zhihu"],
	}
}

// GetCrawlerConfig 从 system_settings 读取爬虫配置。
func (r *SystemRepository) GetCrawlerConfig() (CrawlerConfigData, error) {
	m, err := r.loadCrawlerSettingMap()
	if err != nil {
		return CrawlerConfigData{}, err
	}
	return mapToCrawlerConfigData(m), nil
}

// SaveCrawlerConfig 写入爬虫配置到 system_settings。
func (r *SystemRepository) SaveCrawlerConfig(cfg CrawlerConfigData, updatedBy uint) error {
	pairs := map[string]string{
		"crawler.max_notes_count":        strconv.Itoa(cfg.MaxNotesCount),
		"crawler.max_comments_count":     strconv.Itoa(cfg.MaxCommentsCount),
		"crawler.max_sub_comments_count": strconv.Itoa(cfg.MaxSubCommentsCount),
		"crawler.max_concurrency_num":    strconv.Itoa(cfg.MaxConcurrency),
		"crawler.sleep_sec_min":       strconv.Itoa(cfg.SleepSecMin),
		"crawler.sleep_sec_max":       strconv.Itoa(cfg.SleepSecMax),
		"crawler.enable_ip_proxy":     boolToSetting(cfg.EnableIPProxy),
		"crawler.ip_proxy_pool_count": strconv.Itoa(cfg.IPProxyPoolCount),
		"crawler.ip_proxy_provider":   cfg.IPProxyProvider,
		"crawler.proxy_kdl_secret_id":  cfg.ProxyKdlSecretID,
		"crawler.proxy_kdl_signature":  cfg.ProxyKdlSignature,
		"crawler.proxy_kdl_username":   cfg.ProxyKdlUsername,
		"crawler.proxy_kdl_password":   cfg.ProxyKdlPassword,
		"crawler.proxy_wandou_app_key": cfg.ProxyWandouAppKey,
		"crawler.xhs.sort_type":       cfg.XhsSortType,
		"crawler.wb.search_type":      cfg.WeiboSearchType,
		"crawler.dy.sort_type":        strconv.Itoa(cfg.DySortType),
		"crawler.zhihu.sort":          cfg.ZhihuSort,
		"crawler.zhihu.search_time":   cfg.ZhihuSearchTime,
		"crawler.cookie.xhs":   cfg.CookieXhs,
		"crawler.cookie.dy":    cfg.CookieDy,
		"crawler.cookie.ks":    cfg.CookieKs,
		"crawler.cookie.bili":  cfg.CookieBili,
		"crawler.cookie.wb":    cfg.CookieWb,
		"crawler.cookie.tieba": cfg.CookieTieba,
		"crawler.cookie.zhihu": cfg.CookieZhihu,
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, key := range crawlerConfigKeys {
			newVal := pairs[key]
			var existing model.SystemSetting
			err := tx.Where("`key` = ?", key).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&model.SystemSetting{
					Key: key, Value: newVal, UpdatedBy: updatedBy,
				}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if existing.Value != newVal {
				if err := tx.Model(&model.SystemSetting{}).Where("`key` = ?", key).
					Updates(map[string]any{
						"value": newVal, "updated_by": updatedBy,
					}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// CrawlerConfigResponse 管理端 API 响应（敏感字段脱敏）。
type CrawlerConfigResponse struct {
	MaxNotesCount       int    `json:"maxNotesCount"`
	MaxCommentsCount    int    `json:"maxCommentsCount"`
	MaxSubCommentsCount int    `json:"maxSubCommentsCount"`
	MaxConcurrency      int    `json:"maxConcurrency"`
	SleepSecMin       int    `json:"sleepSecMin"`
	SleepSecMax       int    `json:"sleepSecMax"`
	EnableIPProxy     bool   `json:"enableIPProxy"`
	IPProxyPoolCount  int    `json:"ipProxyPoolCount"`
	IPProxyProvider   string `json:"ipProxyProvider"`
	ProxyKdlSecretID  string `json:"proxyKdlSecretId"`
	ProxyKdlSignature string `json:"proxyKdlSignature"`
	ProxyKdlUsername  string `json:"proxyKdlUsername"`
	ProxyKdlPassword  string `json:"proxyKdlPassword"`
	ProxyWandouAppKey string `json:"proxyWandouAppKey"`
	XhsSortType       string `json:"xhsSortType"`
	WeiboSearchType   string `json:"weiboSearchType"`
	DySortType        int    `json:"dySortType"`
	ZhihuSort         string `json:"zhihuSort"`
	ZhihuSearchTime   string `json:"zhihuSearchTime"`
	Cookies           map[string]crawlerCookieInfo `json:"cookies"`
}

type crawlerCookieInfo struct {
	Masked bool   `json:"set"`
	Value  string `json:"masked"`
}

// BuildCrawlerConfigResponse 将内部 data 转为 API 响应（Cookie / 密钥脱敏）。
func BuildCrawlerConfigResponse(cfg CrawlerConfigData) CrawlerConfigResponse {
	maskSecret := func(s string) string {
		if s == "" {
			return ""
		}
		return utils.MaskString(s)
	}
	cookieInfo := func(raw string) crawlerCookieInfo {
		if raw == "" {
			return crawlerCookieInfo{Masked: false, Value: ""}
		}
		return crawlerCookieInfo{Masked: true, Value: utils.MaskString(raw)}
	}
	return CrawlerConfigResponse{
		MaxNotesCount:       cfg.MaxNotesCount,
		MaxCommentsCount:    cfg.MaxCommentsCount,
		MaxSubCommentsCount: cfg.MaxSubCommentsCount,
		MaxConcurrency:      cfg.MaxConcurrency,
		SleepSecMin:       cfg.SleepSecMin,
		SleepSecMax:       cfg.SleepSecMax,
		EnableIPProxy:     cfg.EnableIPProxy,
		IPProxyPoolCount:  cfg.IPProxyPoolCount,
		IPProxyProvider:   cfg.IPProxyProvider,
		ProxyKdlSecretID:  maskSecret(cfg.ProxyKdlSecretID),
		ProxyKdlSignature: maskSecret(cfg.ProxyKdlSignature),
		ProxyKdlUsername:  cfg.ProxyKdlUsername,
		ProxyKdlPassword:  maskSecret(cfg.ProxyKdlPassword),
		ProxyWandouAppKey: maskSecret(cfg.ProxyWandouAppKey),
		XhsSortType:       cfg.XhsSortType,
		WeiboSearchType:   cfg.WeiboSearchType,
		DySortType:        cfg.DySortType,
		ZhihuSort:         cfg.ZhihuSort,
		ZhihuSearchTime:   cfg.ZhihuSearchTime,
		Cookies: map[string]crawlerCookieInfo{
			"xhs":   cookieInfo(cfg.CookieXhs),
			"dy":    cookieInfo(cfg.CookieDy),
			"ks":    cookieInfo(cfg.CookieKs),
			"bili":  cookieInfo(cfg.CookieBili),
			"wb":    cookieInfo(cfg.CookieWb),
			"tieba": cookieInfo(cfg.CookieTieba),
			"zhihu": cookieInfo(cfg.CookieZhihu),
		},
	}
}
