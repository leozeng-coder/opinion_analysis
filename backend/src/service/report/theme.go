package report

import (
	"math/rand"
	"time"
)

// ThemeVariant 控制单次生成的差异化风格参数，每次随机组合
type ThemeVariant struct {
	PieStyle   string `json:"pieStyle"`   // "rose" | "nightingale" | "donut"
	TrendStyle string `json:"trendStyle"` // "area" | "barline"
	CardGlass  bool   `json:"cardGlass"`  // 磨砂玻璃卡片
	AccentBar  bool   `json:"accentBar"`  // 话题卡片左侧强调色条
}

// ReportTheme 报告视觉主题
type ReportTheme struct {
	Name           string
	HeaderGradient string
	PageBackground string
	Primary        string
	Secondary      string
	Accent         string
	CardBg         string
	TextPrimary    string
	TextSecondary  string
	SentPos        string
	SentNeu        string
	SentNeg        string
	ChartColors    []string
	BorderRadius   string
	Shadow         string
	Icons          themeIcons
	Variant        ThemeVariant
}

type themeIcons struct {
	Report     string
	Articles   string
	Comments   string
	Positive   string
	Negative   string
	Neutral    string
	Platform   string
	Sentiment  string
	Tags       string
	Topics     string
	Trend      string
	Radar      string
	Heatmap    string
	Score      string
	Articles2  string
	Conclusion string
}

var reportThemes = []ReportTheme{
	{
		Name:           "深海蓝",
		HeaderGradient: "linear-gradient(135deg,#0f4c81 0%,#1a73e8 50%,#4facfe 100%)",
		PageBackground: "linear-gradient(180deg,#eef4fb 0%,#f5f7fa 40%,#eef2f7 100%)",
		Primary:        "#1a73e8",
		Secondary:      "#0f4c81",
		Accent:         "#4facfe",
		CardBg:         "#ffffff",
		TextPrimary:    "#1a1a2e",
		TextSecondary:  "#64748b",
		SentPos:        "#10b981",
		SentNeu:        "#94a3b8",
		SentNeg:        "#ef4444",
		ChartColors:    []string{"#1a73e8", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#06b6d4", "#ec4899", "#84cc16"},
		BorderRadius:   "14px",
		Shadow:         "0 4px 24px rgba(15,76,129,.08)",
		Icons:          themeIcons{"📊", "📰", "💬", "😊", "😟", "😐", "🌐", "💭", "🏷️", "🔍", "📈", "🎯", "🌡️", "📶", "⭐", "💡"},
	},
	{
		Name:           "暮光橙",
		HeaderGradient: "linear-gradient(135deg,#c2410c 0%,#ea580c 45%,#fbbf24 100%)",
		PageBackground: "linear-gradient(180deg,#fff7ed 0%,#fef3c7 50%,#fffbeb 100%)",
		Primary:        "#ea580c",
		Secondary:      "#9a3412",
		Accent:         "#fbbf24",
		CardBg:         "#ffffff",
		TextPrimary:    "#431407",
		TextSecondary:  "#78716c",
		SentPos:        "#16a34a",
		SentNeu:        "#a8a29e",
		SentNeg:        "#dc2626",
		ChartColors:    []string{"#ea580c", "#16a34a", "#eab308", "#dc2626", "#9333ea", "#0891b2", "#db2777", "#65a30d"},
		BorderRadius:   "16px",
		Shadow:         "0 6px 28px rgba(234,88,12,.10)",
		Icons:          themeIcons{"🔥", "📄", "🗨️", "👍", "⚠️", "➖", "📡", "❤️‍🔥", "🔖", "🧩", "📉", "🎪", "🌡️", "📊", "🏆", "✨"},
	},
	{
		Name:           "翡翠绿",
		HeaderGradient: "linear-gradient(135deg,#065f46 0%,#059669 50%,#34d399 100%)",
		PageBackground: "linear-gradient(180deg,#ecfdf5 0%,#f0fdf4 50%,#ecfdf5 100%)",
		Primary:        "#059669",
		Secondary:      "#065f46",
		Accent:         "#34d399",
		CardBg:         "#ffffff",
		TextPrimary:    "#064e3b",
		TextSecondary:  "#6b7280",
		SentPos:        "#22c55e",
		SentNeu:        "#9ca3af",
		SentNeg:        "#f43f5e",
		ChartColors:    []string{"#059669", "#22c55e", "#eab308", "#f43f5e", "#6366f1", "#14b8a6", "#f97316", "#a855f7"},
		BorderRadius:   "12px",
		Shadow:         "0 4px 20px rgba(5,150,105,.09)",
		Icons:          themeIcons{"🌿", "📋", "💭", "🌟", "🚨", "⚖️", "🔗", "🎭", "🏷", "🔬", "📊", "🛡️", "📶", "📈", "💎", "📝"},
	},
	{
		Name:           "紫夜",
		HeaderGradient: "linear-gradient(135deg,#4c1d95 0%,#7c3aed 50%,#c084fc 100%)",
		PageBackground: "linear-gradient(180deg,#f5f3ff 0%,#ede9fe 50%,#faf5ff 100%)",
		Primary:        "#7c3aed",
		Secondary:      "#4c1d95",
		Accent:         "#c084fc",
		CardBg:         "#ffffff",
		TextPrimary:    "#2e1065",
		TextSecondary:  "#7c7c8a",
		SentPos:        "#059669",
		SentNeu:        "#a1a1aa",
		SentNeg:        "#e11d48",
		ChartColors:    []string{"#7c3aed", "#059669", "#f59e0b", "#e11d48", "#2563eb", "#0891b2", "#db2777", "#84cc16"},
		BorderRadius:   "18px",
		Shadow:         "0 8px 32px rgba(124,58,237,.10)",
		Icons:          themeIcons{"🔮", "📑", "💬", "✅", "❌", "〰️", "🌍", "🎨", "🏷️", "🧠", "📈", "🎚️", "🌡️", "📊", "🌟", "🎯"},
	},
	{
		Name:           "青墨",
		HeaderGradient: "linear-gradient(135deg,#0e7490 0%,#0891b2 50%,#22d3ee 100%)",
		PageBackground: "linear-gradient(180deg,#ecfeff 0%,#f0fdfa 50%,#ecfeff 100%)",
		Primary:        "#0891b2",
		Secondary:      "#0e7490",
		Accent:         "#22d3ee",
		CardBg:         "#ffffff",
		TextPrimary:    "#164e63",
		TextSecondary:  "#64748b",
		SentPos:        "#10b981",
		SentNeu:        "#94a3b8",
		SentNeg:        "#f97316",
		ChartColors:    []string{"#0891b2", "#10b981", "#eab308", "#f97316", "#8b5cf6", "#ec4899", "#6366f1", "#14b8a6"},
		BorderRadius:   "14px",
		Shadow:         "0 4px 22px rgba(8,145,178,.09)",
		Icons:          themeIcons{"🌊", "📰", "💬", "😄", "😠", "😐", "📱", "💡", "🔖", "🔎", "📊", "⚡", "🌡️", "📈", "🔝", "📌"},
	},
	{
		Name:           "玫瑰金",
		HeaderGradient: "linear-gradient(135deg,#9d174d 0%,#db2777 50%,#fda4af 100%)",
		PageBackground: "linear-gradient(180deg,#fff1f2 0%,#fce7f3 50%,#fdf2f8 100%)",
		Primary:        "#db2777",
		Secondary:      "#9d174d",
		Accent:         "#fda4af",
		CardBg:         "#ffffff",
		TextPrimary:    "#500724",
		TextSecondary:  "#9ca3af",
		SentPos:        "#16a34a",
		SentNeu:        "#a8a29e",
		SentNeg:        "#b91c1c",
		ChartColors:    []string{"#db2777", "#16a34a", "#ca8a04", "#b91c1c", "#6366f1", "#0891b2", "#ea580c", "#7c3aed"},
		BorderRadius:   "20px",
		Shadow:         "0 6px 26px rgba(219,39,119,.10)",
		Icons:          themeIcons{"🌸", "📃", "💬", "💚", "💔", "💛", "🌐", "🎭", "🏷", "🔍", "📈", "🎀", "🌡️", "📊", "👑", "💡"},
	},
	{
		Name:           "星辰灰",
		HeaderGradient: "linear-gradient(135deg,#1e293b 0%,#334155 50%,#64748b 100%)",
		PageBackground: "linear-gradient(180deg,#f8fafc 0%,#f1f5f9 50%,#e2e8f0 100%)",
		Primary:        "#475569",
		Secondary:      "#1e293b",
		Accent:         "#94a3b8",
		CardBg:         "#ffffff",
		TextPrimary:    "#0f172a",
		TextSecondary:  "#64748b",
		SentPos:        "#0ea5e9",
		SentNeu:        "#94a3b8",
		SentNeg:        "#f43f5e",
		ChartColors:    []string{"#475569", "#0ea5e9", "#f59e0b", "#f43f5e", "#8b5cf6", "#10b981", "#ec4899", "#06b6d4"},
		BorderRadius:   "12px",
		Shadow:         "0 4px 20px rgba(15,23,42,.08)",
		Icons:          themeIcons{"🌌", "📝", "💬", "🌤️", "🌧️", "☁️", "🛰️", "🔭", "🔖", "🔬", "📊", "🧭", "🌡️", "📈", "🏅", "🔍"},
	},
	{
		Name:           "琥珀金",
		HeaderGradient: "linear-gradient(135deg,#92400e 0%,#d97706 50%,#fcd34d 100%)",
		PageBackground: "linear-gradient(180deg,#fffbeb 0%,#fef9c3 50%,#fefce8 100%)",
		Primary:        "#d97706",
		Secondary:      "#92400e",
		Accent:         "#fcd34d",
		CardBg:         "#ffffff",
		TextPrimary:    "#451a03",
		TextSecondary:  "#78716c",
		SentPos:        "#16a34a",
		SentNeu:        "#a8a29e",
		SentNeg:        "#dc2626",
		ChartColors:    []string{"#d97706", "#16a34a", "#6366f1", "#dc2626", "#0891b2", "#ec4899", "#059669", "#7c3aed"},
		BorderRadius:   "16px",
		Shadow:         "0 5px 24px rgba(217,119,6,.12)",
		Icons:          themeIcons{"✨", "📜", "💬", "🌞", "⛈️", "🌤️", "🏛️", "🔆", "🏷️", "🔍", "📈", "🧿", "🌡️", "📊", "🏆", "📖"},
	},
}

func pickReportTheme(name string) ReportTheme {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var theme ReportTheme
	if name != "" && name != "random" {
		for _, t := range reportThemes {
			if t.Name == name {
				theme = t
				break
			}
		}
	}
	if theme.Name == "" {
		theme = reportThemes[r.Intn(len(reportThemes))]
	}
	theme.Variant = ThemeVariant{
		PieStyle:   []string{"rose", "nightingale", "donut"}[r.Intn(3)],
		TrendStyle: []string{"area", "barline"}[r.Intn(2)],
		CardGlass:  r.Intn(2) == 0,
		AccentBar:  r.Intn(2) == 0,
	}
	return theme
}
