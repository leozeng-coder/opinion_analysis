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
		SentPos:        "#34d399",
		SentNeu:        "#cbd5e1",
		SentNeg:        "#fca5a5",
		ChartColors:    []string{"#60a5fa", "#34d399", "#fcd34d", "#fca5a5", "#a78bfa", "#67e8f9", "#f9a8d4", "#bef264"},
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
		SentPos:        "#4ade80",
		SentNeu:        "#d6d3d1",
		SentNeg:        "#f87171",
		ChartColors:    []string{"#fb923c", "#4ade80", "#fde68a", "#f87171", "#c084fc", "#38bdf8", "#f0abfc", "#a3e635"},
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
		SentPos:        "#6ee7b7",
		SentNeu:        "#d1d5db",
		SentNeg:        "#fda4af",
		ChartColors:    []string{"#34d399", "#86efac", "#fde68a", "#fda4af", "#a5b4fc", "#5eead4", "#fdba74", "#d8b4fe"},
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
		SentPos:        "#34d399",
		SentNeu:        "#d4d4d8",
		SentNeg:        "#fda4af",
		ChartColors:    []string{"#c084fc", "#34d399", "#fcd34d", "#fda4af", "#93c5fd", "#67e8f9", "#f9a8d4", "#bef264"},
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
		SentPos:        "#34d399",
		SentNeu:        "#cbd5e1",
		SentNeg:        "#fdba74",
		ChartColors:    []string{"#38bdf8", "#34d399", "#fde68a", "#fdba74", "#c084fc", "#f9a8d4", "#a5b4fc", "#5eead4"},
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
		SentPos:        "#4ade80",
		SentNeu:        "#d6d3d1",
		SentNeg:        "#fca5a5",
		ChartColors:    []string{"#f9a8d4", "#4ade80", "#fde68a", "#fca5a5", "#a5b4fc", "#67e8f9", "#fdba74", "#d8b4fe"},
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
		SentPos:        "#7dd3fc",
		SentNeu:        "#cbd5e1",
		SentNeg:        "#fda4af",
		ChartColors:    []string{"#94a3b8", "#7dd3fc", "#fcd34d", "#fda4af", "#c084fc", "#6ee7b7", "#f9a8d4", "#67e8f9"},
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
		SentPos:        "#4ade80",
		SentNeu:        "#d6d3d1",
		SentNeg:        "#f87171",
		ChartColors:    []string{"#fcd34d", "#4ade80", "#a5b4fc", "#f87171", "#67e8f9", "#f9a8d4", "#6ee7b7", "#c084fc"},
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
