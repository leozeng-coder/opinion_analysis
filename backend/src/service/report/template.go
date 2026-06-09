package report

// htmlTemplate is the Go html/template source for the HTML analysis report.
const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>舆情分析报告 — 采集任务 #{{.CrawlerRunID}}</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&family=Noto+Sans+SC:wght@300;400;500;700;900&display=swap" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
<style>
:root{
  --primary:        {{.Theme.Primary}};
  --secondary:      {{.Theme.Secondary}};
  --accent:         {{.Theme.Accent}};
  --card-bg:        {{.Theme.CardBg}};
  --text-primary:   {{.Theme.TextPrimary}};
  --text-secondary: {{.Theme.TextSecondary}};
  --sent-pos:       {{.Theme.SentPos}};
  --sent-neu:       {{.Theme.SentNeu}};
  --sent-neg:       {{.Theme.SentNeg}};
  --radius:         {{.Theme.BorderRadius}};
  --shadow:         {{.Theme.Shadow | css}};
  --page-bg:        {{.Theme.PageBackground | css}};
  --ease-soft:      cubic-bezier(0.4, 0, 0.2, 1);
  --ease-spring:    cubic-bezier(0.34, 1.56, 0.64, 1);
}
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0;}
html{scroll-behavior:smooth;}
body{
  font-family:'Inter','Noto Sans SC','PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;
  background:var(--page-bg);
  color:var(--text-primary);
  line-height:1.65;
  font-feature-settings:'cv02','cv03','cv04','cv11';
  -webkit-font-smoothing:antialiased;
  -moz-osx-font-smoothing:grayscale;
  position:relative;
  min-height:100vh;
  isolation:isolate;
}
/* ── Aurora background blobs ── */
/* position:absolute (not fixed) 避免 fixed+filter 引发的渲染层泄漏遮挡问题 */
.aurora{position:absolute;top:0;left:0;width:100%;min-height:100%;z-index:0;overflow:hidden;pointer-events:none;}
.aurora .blob{position:absolute;border-radius:50%;filter:blur(100px);opacity:0.28;animation:float 24s ease-in-out infinite;}
.aurora .blob.b1{width:560px;height:560px;top:-180px;left:-140px;background:var(--primary);animation-delay:0s;}
.aurora .blob.b2{width:460px;height:460px;top:28%;right:-110px;background:var(--accent);animation-delay:-8s;}
.aurora .blob.b3{width:400px;height:400px;bottom:-120px;left:32%;background:var(--secondary);animation-delay:-16s;}
@keyframes float{
  0%,100%{transform:translate(0,0) scale(1);}
  33%{transform:translate(35px,-25px) scale(1.05);}
  66%{transform:translate(-25px,18px) scale(0.97);}
}
/* noise 同样改 absolute，避免 fixed 层在滚动时产生异常叠层 */
.noise{position:absolute;top:0;left:0;width:100%;min-height:100%;z-index:1;pointer-events:none;opacity:0.018;background-image:url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='200' height='200'><filter id='n'><feTurbulence baseFrequency='0.85' numOctaves='2' stitchTiles='stitch'/></filter><rect width='100%25' height='100%25' filter='url(%23n)'/></svg>");}
/* ── Scrollbar ── */
::-webkit-scrollbar{width:10px;height:10px;}
::-webkit-scrollbar-track{background:transparent;}
::-webkit-scrollbar-thumb{background:rgba(100,116,139,0.35);border-radius:5px;border:2px solid transparent;background-clip:padding-box;}
::-webkit-scrollbar-thumb:hover{background:rgba(100,116,139,0.55);background-clip:padding-box;}
/* ── Page ── */
.page{max-width:1280px;margin:0 auto;padding:32px 24px 48px;position:relative;z-index:2;}
/* ── Hero ── */
.hero{
  background:linear-gradient(rgba(255,255,255,0.13),rgba(255,255,255,0.13)),{{.Theme.HeaderGradient | css}};
  border-radius:calc(var(--radius) + 4px);
  padding:48px 52px 44px;
  margin-bottom:32px;
  position:relative;
  overflow:hidden;
  color:#fff;
  box-shadow:0 20px 50px -20px rgba(0,0,0,0.25),0 0 0 1px rgba(255,255,255,0.1) inset;
}
.hero::before{content:'';position:absolute;top:-100px;right:-80px;width:340px;height:340px;border-radius:50%;background:radial-gradient(circle,rgba(255,255,255,0.18) 0%,transparent 70%);pointer-events:none;}
.hero::after{content:'';position:absolute;bottom:-80px;left:30%;width:280px;height:280px;border-radius:50%;background:radial-gradient(circle,rgba(255,255,255,0.12) 0%,transparent 70%);pointer-events:none;}
.hero-grid{position:absolute;inset:0;background-image:linear-gradient(rgba(255,255,255,0.04) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,0.04) 1px,transparent 1px);background-size:40px 40px;mask-image:radial-gradient(ellipse at center,#000 30%,transparent 75%);-webkit-mask-image:radial-gradient(ellipse at center,#000 30%,transparent 75%);pointer-events:none;}
.hero-inner{position:relative;z-index:1;}
.hero-title{font-size:32px;font-weight:700;display:flex;align-items:center;gap:14px;margin-bottom:12px;letter-spacing:-0.02em;}
.hero-title .icon{font-size:36px;filter:drop-shadow(0 2px 8px rgba(0,0,0,0.15));}
.theme-badge{display:inline-flex;align-items:center;gap:6px;background:rgba(255,255,255,0.16);backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);border:1px solid rgba(255,255,255,0.22);border-radius:24px;padding:5px 14px;font-size:12px;font-weight:500;margin-bottom:18px;letter-spacing:0.02em;}
.hero-meta{display:flex;flex-wrap:wrap;gap:10px 28px;font-size:13px;opacity:0.92;font-weight:400;}
.hero-meta span{display:flex;align-items:center;gap:6px;}
/* ── KPI ── */
.kpi-grid{display:grid;grid-template-columns:repeat(5,1fr);gap:18px;margin-bottom:32px;}
.kpi{
  position:relative;
  background:rgba(255,255,255,0.55);
  backdrop-filter:blur(24px) saturate(180%);
  -webkit-backdrop-filter:blur(24px) saturate(180%);
  border-radius:var(--radius);
  padding:22px 20px 18px;
  border:1px solid rgba(255,255,255,0.6);
  box-shadow:0 8px 32px rgba(31,38,135,0.08),inset 0 1px 0 rgba(255,255,255,0.7);
  transition:transform 350ms var(--ease-spring),box-shadow 350ms var(--ease-soft);
  overflow:hidden;
}
.kpi::before{content:'';position:absolute;top:-30px;right:-30px;width:100px;height:100px;border-radius:50%;background:var(--primary);opacity:0.08;pointer-events:none;transition:transform 500ms var(--ease-soft);}
.kpi:hover{transform:translateY(-6px);box-shadow:0 16px 48px rgba(31,38,135,0.14),inset 0 1px 0 rgba(255,255,255,0.8);}
.kpi:hover::before{transform:scale(1.4);}
.kpi-icon{
  position:relative;
  z-index:1;
  display:inline-flex;
  align-items:center;
  justify-content:center;
  width:44px;
  height:44px;
  border-radius:14px;
  background:linear-gradient(135deg,rgba(255,255,255,0.7),rgba(255,255,255,0.3));
  border:1px solid rgba(255,255,255,0.7);
  font-size:22px;
  margin-bottom:10px;
  box-shadow:0 4px 12px rgba(0,0,0,0.04);
}
.kpi-label{font-size:12px;color:var(--text-secondary);margin-bottom:4px;font-weight:500;letter-spacing:0.02em;}
.kpi-value{font-size:32px;font-weight:700;color:var(--primary);line-height:1.1;font-variant-numeric:tabular-nums;letter-spacing:-0.02em;}
.kpi-sub{font-size:11px;color:var(--text-secondary);margin-top:4px;font-weight:400;}
/* ── Risk banner ── */
.risk-banner{
  background:linear-gradient(135deg,rgba(255,243,205,0.85),rgba(254,249,231,0.85));
  backdrop-filter:blur(16px);
  -webkit-backdrop-filter:blur(16px);
  border:1px solid rgba(251,191,36,0.5);
  border-left:4px solid #f59e0b;
  border-radius:var(--radius);
  padding:14px 22px;
  margin-bottom:32px;
  display:flex;
  align-items:center;
  gap:14px;
  font-size:14px;
  box-shadow:0 4px 20px rgba(251,191,36,0.12);
}
.pulse-dot{width:10px;height:10px;border-radius:50%;background:#ef4444;flex-shrink:0;animation:pulse 1.6s infinite;}
@keyframes pulse{0%,100%{box-shadow:0 0 0 0 rgba(239,68,68,0.55);}50%{box-shadow:0 0 0 9px rgba(239,68,68,0);}}
/* ── Sections ── */
.section{margin-bottom:40px;opacity:0;transform:translateY(18px);transition:opacity 480ms var(--ease-soft),transform 480ms var(--ease-soft);}
.section.visible{opacity:1;transform:translateY(0);}
.section-header{display:flex;align-items:center;gap:12px;margin-bottom:20px;}
.section-icon{
  display:inline-flex;
  align-items:center;
  justify-content:center;
  width:36px;height:36px;
  border-radius:12px;
  background:linear-gradient(135deg,rgba(255,255,255,0.7),rgba(255,255,255,0.3));
  border:1px solid rgba(255,255,255,0.6);
  font-size:18px;
  box-shadow:0 4px 12px rgba(0,0,0,0.04);
}
.section-title-group{display:flex;flex-direction:column;}
.section-title{font-size:18px;font-weight:600;color:var(--text-primary);letter-spacing:-0.01em;}
.section-title-underline{height:3px;width:48px;background:linear-gradient(90deg,var(--primary),var(--accent));border-radius:2px;margin-top:6px;}
.section-line{flex:1;height:1px;background:linear-gradient(90deg,rgba(0,0,0,0.08),transparent);margin-left:8px;}
/* ── Grid layouts ── */
.grid-2{display:grid;grid-template-columns:1fr 1fr;gap:22px;}
.grid-1{display:grid;grid-template-columns:1fr;gap:22px;}
.chart-grid{margin-bottom:18px;}
.topic-grid{display:grid;grid-template-columns:1fr 1fr;gap:18px;}
.grid-3{display:grid;grid-template-columns:1fr 1fr 1fr;gap:22px;}
/* ── Chart box (frosted glass) ── */
.chart-box{
  background:rgba(255,255,255,0.62);
  backdrop-filter:blur(24px) saturate(180%);
  -webkit-backdrop-filter:blur(24px) saturate(180%);
  border-radius:var(--radius);
  padding:22px;
  border:1px solid rgba(255,255,255,0.6);
  box-shadow:0 8px 32px rgba(31,38,135,0.06),inset 0 1px 0 rgba(255,255,255,0.7);
  transition:transform 350ms var(--ease-soft),box-shadow 350ms var(--ease-soft);
}
.chart-box:hover{box-shadow:0 12px 40px rgba(31,38,135,0.10),inset 0 1px 0 rgba(255,255,255,0.8);}
.chart-box h4{
  font-size:11px;
  font-weight:600;
  color:var(--text-secondary);
  margin-bottom:14px;
  text-transform:uppercase;
  letter-spacing:0.08em;
  opacity:0.8;
  display:flex;align-items:center;gap:6px;
}
.chart-box h4::before{content:'';width:3px;height:12px;border-radius:1.5px;background:linear-gradient(180deg,var(--primary),var(--accent));}
/* ── Glass variant (deepens glass) ── */
.card-glass .chart-box{background:rgba(255,255,255,0.42);backdrop-filter:blur(30px) saturate(200%);-webkit-backdrop-filter:blur(30px) saturate(200%);}
.card-glass .kpi{background:rgba(255,255,255,0.40);}
.card-glass .topic-card{background:rgba(255,255,255,0.42);}
.card-glass .conclusion-box{background:rgba(255,255,255,0.40);}
/* ── Accent-bar variant ── */
.accent-bar .topic-card{border-left:none;position:relative;}
.accent-bar .topic-card::before{content:'';position:absolute;left:0;top:14%;bottom:14%;width:4px;border-radius:0 3px 3px 0;background:linear-gradient(180deg,var(--primary),var(--accent));}
/* ── Platform heatmap ── */
.heat-grid{display:grid;grid-template-columns:90px repeat(3,1fr);gap:8px;}
.heat-cell{border-radius:10px;padding:14px 6px;text-align:center;transition:transform 250ms var(--ease-spring);box-shadow:inset 0 0 0 1px rgba(255,255,255,0.3);}
.heat-cell:hover{transform:scale(1.04);}
.heat-header{font-size:11px;font-weight:600;color:var(--text-secondary);display:flex;align-items:center;justify-content:center;padding:6px 0;letter-spacing:0.05em;text-transform:uppercase;}
.heat-label{font-size:12px;font-weight:500;color:var(--text-primary);display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.03);border-radius:10px;padding:6px 4px;}
.heat-val{font-size:19px;font-weight:700;display:block;font-variant-numeric:tabular-nums;}
.heat-pct{font-size:11px;opacity:0.8;}
/* ── Topic cards ── */
.topic-card-grid{display:grid;grid-template-columns:1fr 1fr;gap:20px;}
.topic-card{
  background:rgba(255,255,255,0.62);
  backdrop-filter:blur(24px) saturate(180%);
  -webkit-backdrop-filter:blur(24px) saturate(180%);
  border-radius:var(--radius);
  padding:20px 22px;
  border:1px solid rgba(255,255,255,0.6);
  box-shadow:0 8px 28px rgba(31,38,135,0.06),inset 0 1px 0 rgba(255,255,255,0.7);
  transition:transform 350ms var(--ease-spring),box-shadow 350ms var(--ease-soft);
}
.topic-card:hover{transform:translateY(-3px);box-shadow:0 14px 36px rgba(31,38,135,0.10);}
.topic-name{font-size:15px;font-weight:600;margin-bottom:6px;display:flex;align-items:center;gap:8px;flex-wrap:wrap;letter-spacing:-0.01em;}
.topic-count-badge{font-size:11px;background:linear-gradient(135deg,var(--primary),var(--accent));color:#fff;border-radius:12px;padding:2px 10px;font-weight:600;box-shadow:0 2px 8px rgba(0,0,0,0.08);}
.sent-bar{height:8px;border-radius:4px;display:flex;overflow:hidden;margin:10px 0;box-shadow:inset 0 1px 2px rgba(0,0,0,0.06);}
.sent-bar-pos{background:linear-gradient(90deg,var(--sent-pos),color-mix(in srgb,var(--sent-pos) 80%,#fff));}
.sent-bar-neu{background:linear-gradient(90deg,var(--sent-neu),color-mix(in srgb,var(--sent-neu) 80%,#fff));}
.sent-bar-neg{background:linear-gradient(90deg,var(--sent-neg),color-mix(in srgb,var(--sent-neg) 80%,#fff));}
.sent-legend{display:flex;gap:14px;font-size:12px;color:var(--text-secondary);margin-bottom:10px;flex-wrap:wrap;}
.sent-legend span{display:flex;align-items:center;gap:5px;}
.sent-dot{width:8px;height:8px;border-radius:50%;display:inline-block;box-shadow:0 0 0 2px rgba(255,255,255,0.6);}
.topic-summary{font-size:13px;color:var(--text-secondary);line-height:1.7;}
/* ── Tag cloud ── */
.tag-cloud-wrap{position:relative;min-height:300px;padding:14px 4px;}
.tag-chip{display:inline-block;padding:2px 12px;border-radius:20px;background:rgba(0,0,0,0.05);cursor:default;transition:transform 0.2s var(--ease-spring),background 0.2s;}
.tag-chip:hover{transform:scale(1.14);background:rgba(0,0,0,0.10);}
/* ── Table ── */
.report-table{width:100%;border-collapse:collapse;font-size:13px;}
.report-table th{
  background:linear-gradient(180deg,rgba(0,0,0,0.04),rgba(0,0,0,0.02));
  font-weight:600;
  color:var(--text-secondary);
  padding:12px 14px;
  text-align:left;
  border-bottom:2px solid rgba(0,0,0,0.06);
  font-size:11px;
  text-transform:uppercase;
  letter-spacing:0.05em;
}
.report-table th:first-child{border-top-left-radius:10px;}
.report-table th:last-child{border-top-right-radius:10px;}
.report-table td{padding:11px 14px;border-bottom:1px solid rgba(0,0,0,0.04);vertical-align:middle;}
.report-table tr:last-child td{border-bottom:none;}
.report-table tr{transition:background 200ms;}
.report-table tbody tr:hover{background:rgba(0,0,0,0.025);}
.badge{display:inline-block;padding:3px 10px;border-radius:12px;font-size:11px;font-weight:600;letter-spacing:0.02em;}
.badge-pos{background:#d1fae5;color:#065f46;box-shadow:0 1px 2px rgba(16,185,129,0.15);}
.badge-neu{background:#e2e8f0;color:#475569;box-shadow:0 1px 2px rgba(100,116,139,0.15);}
.badge-neg{background:#fee2e2;color:#991b1b;box-shadow:0 1px 2px rgba(239,68,68,0.15);}
.score-bar-wrap{display:flex;align-items:center;gap:8px;}
.score-bar-bg{width:72px;height:6px;background:rgba(0,0,0,0.05);border-radius:3px;overflow:hidden;}
.score-bar-fill{height:100%;border-radius:3px;transition:width 600ms var(--ease-soft);}
/* ── Conclusion ── */
.conclusion-box{
  background:rgba(255,255,255,0.62);
  backdrop-filter:blur(24px) saturate(180%);
  -webkit-backdrop-filter:blur(24px) saturate(180%);
  border-radius:var(--radius);
  padding:36px 40px;
  border:1px solid rgba(255,255,255,0.6);
  box-shadow:0 12px 40px rgba(31,38,135,0.08),inset 0 1px 0 rgba(255,255,255,0.7);
  position:relative;
}
.conclusion-quote{position:absolute;top:18px;left:24px;font-size:72px;background:linear-gradient(135deg,var(--primary),var(--accent));-webkit-background-clip:text;background-clip:text;color:transparent;opacity:0.20;font-family:Georgia,'Times New Roman',serif;line-height:1;font-weight:700;pointer-events:none;}
.conclusion-text{font-size:14.5px;line-height:1.95;color:var(--text-primary);position:relative;z-index:1;white-space:pre-wrap;padding-top:8px;}
.conclusion-signature{margin-top:24px;padding-top:16px;border-top:1px dashed rgba(0,0,0,0.08);font-size:12px;color:var(--text-secondary);font-style:italic;text-align:right;}
/* ── Subtitle ── */
.chart-subtitle{font-size:12px;color:var(--text-secondary);margin-bottom:14px;margin-top:-4px;}
/* ── Risk Matrix SVG ── */
.risk-svg-wrap{position:relative;width:100%;}
.risk-tooltip{position:absolute;background:rgba(15,23,42,0.95);backdrop-filter:blur(8px);-webkit-backdrop-filter:blur(8px);color:#fff;padding:10px 14px;border-radius:10px;font-size:12px;pointer-events:none;opacity:0;transition:opacity 200ms var(--ease-soft);z-index:10;box-shadow:0 10px 30px rgba(0,0,0,0.25);max-width:240px;}
.risk-tooltip strong{font-weight:600;display:block;margin-bottom:4px;font-size:13px;}
.risk-tooltip .tt-row{display:flex;justify-content:space-between;gap:14px;line-height:1.6;}
.risk-tooltip .tt-row span:last-child{font-weight:600;font-variant-numeric:tabular-nums;}
.risk-bubble{cursor:pointer;transition:transform 250ms var(--ease-spring),filter 250ms;transform-origin:center;transform-box:fill-box;}
.risk-bubble:hover{transform:scale(1.08);filter:brightness(1.05);}
/* ── Footer ── */
.footer{text-align:center;padding:28px 0 10px;border-top:1px solid rgba(0,0,0,0.06);color:var(--text-secondary);font-size:12px;margin-top:24px;letter-spacing:0.02em;}
.footer strong{font-weight:600;background:linear-gradient(90deg,var(--primary),var(--accent));-webkit-background-clip:text;background-clip:text;color:transparent;}
/* ── Print ── */
@media print{
  .aurora,.noise{display:none;}
  body{background:#fff;}
  .section{opacity:1;transform:none;}
  .chart-box,.topic-card,.kpi,.conclusion-box{break-inside:avoid;}
}
/* ── Responsive ── */
@media(max-width:960px){.grid-2,.topic-card-grid,.topic-grid{grid-template-columns:1fr;}.grid-3{grid-template-columns:1fr 1fr;}}
@media(max-width:900px){.kpi-grid{grid-template-columns:repeat(3,1fr);}}
@media(max-width:600px){.kpi-grid{grid-template-columns:1fr 1fr;}.grid-3{grid-template-columns:1fr;}.hero{padding:32px 24px 28px;}.hero-title{font-size:23px;}.page{padding:20px 16px 32px;}}
</style>
</head>
<body>
<div class="aurora">
  <div class="blob b1"></div>
  <div class="blob b2"></div>
  <div class="blob b3"></div>
</div>
<div class="noise"></div>
<div class="page{{if .Theme.Variant.CardGlass}} card-glass{{end}}{{if .Theme.Variant.AccentBar}} accent-bar{{end}}">

<!-- ══ Hero ══ -->
<div class="hero">
  <div class="hero-grid"></div>
  <div class="hero-inner">
    <div class="hero-title"><span class="icon">{{.Theme.Icons.Report}}</span>舆情分析报告</div>
    <div class="theme-badge">🎨 主题：{{.Theme.Name}}</div>
    <div class="hero-meta">
      <span>🕐 生成时间：{{.GeneratedAt}}</span>
      <span>📅 数据范围：{{.TimeRange}}</span>
      <span>🌐 平台：{{.Platforms}}</span>
      <span>📌 话题：{{.Topics}}</span>
    </div>
  </div>
</div>

<!-- ══ KPI Grid ══ -->
<div class="kpi-grid">
  <div class="kpi">
    <div class="kpi-icon">{{.Theme.Icons.Articles}}</div>
    <div class="kpi-label">文章总量</div>
    <div class="kpi-value" data-target="{{.ArticleCount}}">0</div>
    <div class="kpi-sub">篇内容</div>
  </div>
  <div class="kpi">
    <div class="kpi-icon">{{.Theme.Icons.Comments}}</div>
    <div class="kpi-label">评论总量</div>
    <div class="kpi-value" data-target="{{.CommentCount}}">0</div>
    <div class="kpi-sub">条评论</div>
  </div>
  <div class="kpi">
    <div class="kpi-icon">{{.Theme.Icons.Positive}}</div>
    <div class="kpi-label">正面情感</div>
    <div class="kpi-value" style="color:var(--sent-pos);" data-target="{{.SentimentPos}}">0</div>
    <div class="kpi-sub">占比 {{printf "%.1f" .SentPosRate}}%</div>
  </div>
  <div class="kpi">
    <div class="kpi-icon">{{.Theme.Icons.Neutral}}</div>
    <div class="kpi-label">中性情感</div>
    <div class="kpi-value" style="color:var(--sent-neu);" data-target="{{.SentimentNeu}}">0</div>
    <div class="kpi-sub">占比 {{printf "%.1f" .SentNeuRate}}%</div>
  </div>
  <div class="kpi">
    <div class="kpi-icon">{{.Theme.Icons.Negative}}</div>
    <div class="kpi-label">负面情感</div>
    <div class="kpi-value" style="color:var(--sent-neg);" data-target="{{.SentimentNeg}}">0</div>
    <div class="kpi-sub">占比 {{printf "%.1f" .SentNegRate}}%</div>
  </div>
</div>

{{if .RiskAlert}}
<!-- ══ Risk Banner ══ -->
<div class="risk-banner">
  <div class="pulse-dot"></div>
  <strong>风险预警：</strong>当前舆情风险等级为「{{.RiskLevel}}」，请相关部门重点关注负面内容趋势，及时制定应对措施。
</div>
{{end}}

<!-- ══ Section: 舆情概览 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Platform}}</span>
    <div class="section-title-group">
      <span class="section-title">舆情概览</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="grid-2">
    <div class="chart-box">
      <h4>平台内容分布</h4>
      <div id="chart-platform" class="chart-div" style="height:300px;"></div>
    </div>
    <div class="chart-box">
      <h4>整体情感结构</h4>
      <div id="chart-sentiment" class="chart-div" style="height:300px;"></div>
    </div>
  </div>
</div>

<!-- ══ Section: 情感多维拆解 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Sentiment}}</span>
    <div class="section-title-group">
      <span class="section-title">情感多维拆解</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="grid-2" style="margin-bottom:22px;">
    <div class="chart-box">
      <h4>{{.Theme.Icons.Heatmap}} 平台情感热力图</h4>
      <div id="plat-heat-grid"></div>
    </div>
    <div class="chart-box">
      <h4>话题情感交叉分析</h4>
      <div id="chart-topic-sent" class="chart-div" style="height:280px;"></div>
    </div>
  </div>
  <div class="grid-3">
    <div class="chart-box">
      <h4>{{.Theme.Icons.Score}} 情感强度分布</h4>
      <div id="chart-score" class="chart-div" style="height:240px;"></div>
    </div>
    <div class="chart-box">
      <h4>{{.Theme.Icons.Trend}} 发布时序趋势</h4>
      <div id="chart-trend" class="chart-div" style="height:240px;"></div>
    </div>
    <div class="chart-box">
      <h4>{{.Theme.Icons.Radar}} 平台情感指数雷达</h4>
      <div id="chart-radar" class="chart-div" style="height:240px;"></div>
    </div>
  </div>
</div>

<!-- ══ Section: 话题热度 Top 10 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Tags}}</span>
    <div class="section-title-group">
      <span class="section-title">话题热度 Top 10</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="chart-box">
    <div id="chart-tags" class="chart-div" style="height:320px;"></div>
  </div>
</div>

<!-- ══ Section: 话题风险矩阵 (Custom SVG) ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Topics}}</span>
    <div class="section-title-group">
      <span class="section-title">话题风险矩阵</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="chart-box">
    <p class="chart-subtitle">横轴：负面率 · 纵轴：文章量 · 气泡大小：影响力 · 颜色：风险等级</p>
    <div class="risk-svg-wrap">
      <div id="chart-risk" style="height:380px;"></div>
    </div>
  </div>
</div>

<!-- ══ Section: 话题深度分析 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Topics}}</span>
    <div class="section-title-group">
      <span class="section-title">话题深度分析</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="topic-card-grid">
    {{range .TopicCards}}
    <div class="topic-card">
      <div class="topic-name">
        {{.Topic}}
        <span class="topic-count-badge">{{.Count}} 篇</span>
      </div>
      <div class="sent-bar">
        <div class="sent-bar-pos" style="width:{{printf "%.1f" .PosRate}}%;"></div>
        <div class="sent-bar-neu" style="width:{{printf "%.1f" .NeuRate}}%;"></div>
        <div class="sent-bar-neg" style="width:{{printf "%.1f" .NegRate}}%;"></div>
      </div>
      <div class="sent-legend">
        <span><span class="sent-dot" style="background:var(--sent-pos);"></span>正面 {{.Pos}} ({{printf "%.0f" .PosRate}}%)</span>
        <span><span class="sent-dot" style="background:var(--sent-neu);"></span>中性 {{.Neu}} ({{printf "%.0f" .NeuRate}}%)</span>
        <span><span class="sent-dot" style="background:var(--sent-neg);"></span>负面 {{.Neg}} ({{printf "%.0f" .NegRate}}%)</span>
      </div>
      <p class="topic-summary">{{.Summary}}</p>
    </div>
    {{end}}
  </div>
</div>

<!-- ══ Section: 热词标签云 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Tags}}</span>
    <div class="section-title-group">
      <span class="section-title">热词标签云</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="chart-box">
    <div id="tag-cloud" class="tag-cloud-wrap"></div>
  </div>
</div>

<!-- ══ Section: 评论深度分析 ══ -->
{{if .HasCommentAnalysis}}
<div class="section">
  <div class="section-header">
    <span class="section-icon">💬</span>
    <div class="section-title-group">
      <span class="section-title">评论深度分析</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="chart-grid grid-2">
    <div class="chart-box">
      <h4>评论情感分布</h4>
      <div id="chart-comment-sent" style="height:240px;"></div>
    </div>
    <div class="chart-box">
      <h4>评论平台分布</h4>
      <div id="chart-comment-plat" style="height:240px;"></div>
    </div>
  </div>
  <div class="chart-grid grid-1">
    <div class="chart-box">
      <h4>评论趋势</h4>
      <div id="chart-comment-trend" style="height:220px;"></div>
    </div>
  </div>
  <div class="chart-box" style="margin-top:18px;">
    <h4>话题评论观点</h4>
    <div id="comment-topic-cards" class="topic-grid"></div>
  </div>
  <div class="chart-box" style="margin-top:18px;">
    <h4>热门评论 Top 10</h4>
    <table class="report-table" id="hot-comments-table">
      <thead>
        <tr>
          <th style="width:36px;">#</th>
          <th>评论内容</th>
          <th style="width:80px;">用户</th>
          <th style="width:80px;">平台</th>
          <th style="width:60px;">点赞</th>
          <th style="width:70px;">情感</th>
        </tr>
      </thead>
      <tbody id="hot-comments-tbody"></tbody>
    </table>
  </div>
</div>
{{end}}

<!-- ══ Section: 高影响力内容 Top 10 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Articles2}}</span>
    <div class="section-title-group">
      <span class="section-title">高影响力内容 Top 10</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="chart-box">
    <table class="report-table" id="top-articles-table">
      <thead>
        <tr>
          <th style="width:36px;">#</th>
          <th>标题</th>
          <th style="width:90px;">平台</th>
          <th style="width:80px;">情感</th>
          <th style="width:120px;">情感指数</th>
        </tr>
      </thead>
      <tbody id="top-articles-tbody"></tbody>
    </table>
  </div>
</div>

<!-- ══ Section: 综合分析结论 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Conclusion}}</span>
    <div class="section-title-group">
      <span class="section-title">综合分析结论</span>
      <div class="section-title-underline"></div>
    </div>
    <div class="section-line"></div>
  </div>
  <div class="conclusion-box">
    <div class="conclusion-quote">"</div>
    <p class="conclusion-text">{{.Conclusion}}</p>
    <div class="conclusion-signature">— 舆情监控平台 · AI 分析引擎</div>
  </div>
</div>

<!-- ══ Footer ══ -->
<div class="footer">
  本报告由 <strong>舆情监控平台</strong> 自动生成 · 主题「{{.Theme.Name}}」 · 数据有效期 7 天
</div>

</div><!-- .page -->

<script>
(function(){
'use strict';
var variant      = {{.ChartVariantJSON}};
var chartColors  = {{.ChartColorsJSON}};
var rawSentPos   = '{{.Theme.SentPos}}';
var rawSentNeu   = '{{.Theme.SentNeu}}';
var rawSentNeg   = '{{.Theme.SentNeg}}';
var rawPrimary   = '{{.Theme.Primary}}';
var rawAccent    = '{{.Theme.Accent}}';
var platformData = {{.PlatformJSON}};
var sentimentData= {{.SentimentJSON}};
var topTagsData  = {{.TopTagsJSON}};
var platformSentData = {{.PlatformSentJSON}};
var topicSentData    = {{.TopicSentJSON}};
var dailyTrendData   = {{.DailyTrendJSON}};
var scoreBucketData  = {{.ScoreBucketJSON}};
var radarData        = {{.RadarJSON}};
var topArticlesData  = {{.TopArticlesJSON}};
var topicBubbleData  = {{.TopicBubbleJSON}};
var tagCloudData     = {{.TagCloudJSON}};
var commentSentData  = {{.CommentSentJSON}};
var commentTopicData = {{.CommentTopicJSON}};
var hotCommentsData  = {{.HotCommentsJSON}};
var commentTrendData = {{.CommentTrendJSON}};
var commentPlatData  = {{.CommentPlatformJSON}};

var allCharts = [];

// 颜色混白
function softenColor(hex,amt){
  if(!hex||!/^#[0-9a-fA-F]{6}$/.test(hex))return hex;
  var r=parseInt(hex.slice(1,3),16),g=parseInt(hex.slice(3,5),16),b=parseInt(hex.slice(5,7),16);
  r=Math.round(r+(255-r)*amt); g=Math.round(g+(255-g)*amt); b=Math.round(b+(255-b)*amt);
  return '#'+[r,g,b].map(function(v){return v.toString(16).padStart(2,'0');}).join('');
}
// 颜色加深
function darkenColor(hex,amt){
  if(!hex||!/^#[0-9a-fA-F]{6}$/.test(hex))return hex;
  var r=parseInt(hex.slice(1,3),16),g=parseInt(hex.slice(3,5),16),b=parseInt(hex.slice(5,7),16);
  r=Math.round(r*(1-amt)); g=Math.round(g*(1-amt)); b=Math.round(b*(1-amt));
  return '#'+[r,g,b].map(function(v){return v.toString(16).padStart(2,'0');}).join('');
}
var sentColors = [softenColor(rawSentPos,0.30),softenColor(rawSentNeu,0.18),softenColor(rawSentNeg,0.28)];
var softColors = chartColors.map(function(c){ return /^#[0-9a-fA-F]{6}$/.test(c)?softenColor(c,0.25):c; });

// ECharts 通用配置
var commonAnim = {animation:true,animationDuration:900,animationEasing:'cubicOut',animationDelay:function(idx){return idx*40;}};
function mkChart(id){
  var el=document.getElementById(id); if(!el)return null;
  var c=echarts.init(el,null,{renderer:'canvas'});
  allCharts.push(c);
  return c;
}
function escHtml(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');}

/* ── Platform Distribution ── */
(function(){
  var c = mkChart('chart-platform'); if(!c)return;
  var pie = variant.pieStyle;
  var radiusCfg, roseType;
  if(pie==='donut'){    radiusCfg=['44%','72%']; roseType=undefined; }
  else if(pie==='nightingale'){ radiusCfg=['18%','72%']; roseType='radius'; }
  else {                radiusCfg=['22%','70%']; roseType='area'; }
  var seriesCfg = {
    type:'pie', radius:radiusCfg,
    itemStyle:{borderRadius:10,borderColor:'#fff',borderWidth:2,shadowBlur:8,shadowColor:'rgba(0,0,0,0.06)'},
    emphasis:{itemStyle:{shadowBlur:18,shadowColor:'rgba(0,0,0,0.25)'},scale:true,scaleSize:6},
    data:platformData.map(function(d,i){return{name:d.name,value:d.value,itemStyle:{color:softColors[i%softColors.length]}};})
  };
  if(roseType) seriesCfg.roseType = roseType;
  var opt = {tooltip:{trigger:'item',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},legend:{orient:'horizontal',bottom:0,textStyle:{fontSize:11,color:'#64748b'},itemGap:14},series:[seriesCfg]};
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Sentiment Structure ── */
(function(){
  var c = mkChart('chart-sentiment'); if(!c)return;
  var opt = {
    tooltip:{trigger:'item',formatter:'{b}: {c} ({d}%)',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    legend:{orient:'horizontal',bottom:0,textStyle:{fontSize:11,color:'#64748b'},itemGap:14},
    series:[{
      type:'pie', radius:['50%','76%'],
      itemStyle:{borderRadius:10,borderColor:'#fff',borderWidth:2,shadowBlur:8,shadowColor:'rgba(0,0,0,0.06)'},
      label:{formatter:function(p){return p.name+'\n'+p.percent.toFixed(1)+'%';},fontSize:12,color:'#475569'},
      emphasis:{label:{show:true,fontSize:14,fontWeight:'bold'},itemStyle:{shadowBlur:20,shadowColor:'rgba(0,0,0,0.25)'},scale:true,scaleSize:6},
      data:sentimentData.map(function(d,i){return{name:d.name,value:d.value,itemStyle:{color:sentColors[i%3]}};})
    }]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Platform Heatmap (CSS) ── */
(function(){
  var el = document.getElementById('plat-heat-grid'); if(!el)return;
  if(!platformSentData||!platformSentData.platforms||!platformSentData.series)return;
  var platforms = platformSentData.platforms;
  var series = platformSentData.series;
  var posPalette=['#d1fae5','#6ee7b7','#10b981'];
  var neuPalette=['#f1f5f9','#cbd5e1','#64748b'];
  var negPalette=['#fee2e2','#fca5a5','#ef4444'];
  var palettes=[posPalette,neuPalette,negPalette];
  var darkText='#1e293b'; var lightText='#fff';
  var grid = document.createElement('div'); grid.className='heat-grid';
  var corner=document.createElement('div'); corner.className='heat-header'; grid.appendChild(corner);
  ['正面','中性','负面'].forEach(function(h){
    var d=document.createElement('div'); d.className='heat-header'; d.textContent=h; grid.appendChild(d);
  });
  platforms.forEach(function(plat,pi){
    var lbl=document.createElement('div'); lbl.className='heat-label'; lbl.textContent=plat; grid.appendChild(lbl);
    series.forEach(function(ser,si){
      var val = ser.data[pi]||0;
      var total=0; series.forEach(function(s){total+=s.data[pi]||0;});
      var pct = total>0?(val/total*100):0;
      var idx = pct<25?0:(pct<50?1:2);
      var cell=document.createElement('div'); cell.className='heat-cell';
      cell.style.background=palettes[si][idx];
      cell.style.color=idx===2?lightText:darkText;
      cell.innerHTML='<span class="heat-val">'+val+'</span><span class="heat-pct">'+pct.toFixed(0)+'%</span>';
      grid.appendChild(cell);
    });
  });
  el.appendChild(grid);
})();

/* ── Topic Sentiment Cross ── */
(function(){
  var c = mkChart('chart-topic-sent'); if(!c)return;
  if(!topicSentData||!topicSentData.topics)return;
  // 预计算每个话题行第一个和最后一个非零系列的索引（用于圆角）
  var numCats = topicSentData.topics.length;
  var firstNZ = []; var lastNZ = [];
  for(var di=0;di<numCats;di++){firstNZ[di]=-1;lastNZ[di]=-1;}
  topicSentData.series.forEach(function(s,i){
    s.data.forEach(function(v,di){
      if(v>0){
        if(firstNZ[di]<0) firstNZ[di]=i;
        lastNZ[di]=i;
      }
    });
  });
  var opt = {
    tooltip:{trigger:'axis',axisPointer:{type:'shadow'},backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    legend:{bottom:0,textStyle:{fontSize:11,color:'#64748b'}},
    grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
    xAxis:{type:'value',axisLabel:{fontSize:11,color:'#64748b'},axisLine:{lineStyle:{color:'#cbd5e1'}},splitLine:{lineStyle:{color:'#f1f5f9'}}},
    yAxis:{type:'category',data:topicSentData.topics,axisLabel:{fontSize:11,width:80,overflow:'truncate',color:'#475569'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
    series:topicSentData.series.map(function(s,i){
      return{
        name:s.name,type:'bar',stack:'total',
        data:s.data.map(function(v,di){
          var isF=firstNZ[di]===i, isL=lastNZ[di]===i;
          var br=isF&&isL?[4,4,4,4]:isF?[4,0,0,4]:isL?[0,4,4,0]:0;
          return{value:v,itemStyle:{borderRadius:br}};
        }),
        itemStyle:{color:sentColors[i%3]},
        label:{show:false},
        emphasis:{itemStyle:{shadowBlur:8,shadowColor:'rgba(0,0,0,0.15)'}}
      };
    })
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Score Bucket ── */
(function(){
  var c = mkChart('chart-score'); if(!c)return;
  var gradColors=['#ef4444','#f97316','#eab308','#84cc16','#22c55e'];
  var opt = {
    tooltip:{trigger:'axis',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    grid:{top:28,bottom:30,left:'3%',right:'3%',containLabel:true},
    xAxis:{type:'category',data:scoreBucketData.map(function(d){return d.name;}),axisLabel:{fontSize:10,color:'#64748b'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
    yAxis:{type:'value',axisLabel:{fontSize:10,color:'#64748b'},axisLine:{show:false},splitLine:{lineStyle:{color:'#f1f5f9'}}},
    series:[{
      type:'bar',
      clip:false,
      data:scoreBucketData.map(function(d,i){
        var c1=softenColor(gradColors[i%gradColors.length],0.10);
        var c2=softenColor(gradColors[i%gradColors.length],0.45);
        return{value:d.value,itemStyle:{borderRadius:[10,10,0,0],color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:c1},{offset:1,color:c2}]}}};
      }),
      label:{show:true,position:'top',fontSize:10,color:'#475569'},
      barMaxWidth:36
    }]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Daily Trend ── */
(function(){
  var c = mkChart('chart-trend'); if(!c)return;
  var dates = dailyTrendData.map(function(d){return d.date;});
  var tipCfg = {backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}};
  var opt;
  if(variant.trendStyle==='area'){
    opt = {
      tooltip:Object.assign({trigger:'axis'},tipCfg),
      legend:{bottom:0,textStyle:{fontSize:10,color:'#64748b'}},
      grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
      xAxis:{type:'category',data:dates,axisLabel:{fontSize:9,rotate:30,color:'#64748b'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
      yAxis:{type:'value',axisLabel:{fontSize:10,color:'#64748b'},axisLine:{show:false},splitLine:{lineStyle:{color:'#f1f5f9'}}},
      series:[
        {name:'正面',type:'line',smooth:true,stack:'trend',areaStyle:{opacity:0.5,color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:sentColors[0]},{offset:1,color:softenColor(sentColors[0],0.6)}]}},data:dailyTrendData.map(function(d){return d.positive;}),itemStyle:{color:sentColors[0]},lineStyle:{color:sentColors[0],width:2},symbol:'circle',symbolSize:6},
        {name:'中性',type:'line',smooth:true,stack:'trend',areaStyle:{opacity:0.5,color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:sentColors[1]},{offset:1,color:softenColor(sentColors[1],0.6)}]}},data:dailyTrendData.map(function(d){return d.neutral;}),itemStyle:{color:sentColors[1]},lineStyle:{color:sentColors[1],width:2},symbol:'circle',symbolSize:6},
        {name:'负面',type:'line',smooth:true,stack:'trend',areaStyle:{opacity:0.5,color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:sentColors[2]},{offset:1,color:softenColor(sentColors[2],0.6)}]}},data:dailyTrendData.map(function(d){return d.negative;}),itemStyle:{color:sentColors[2]},lineStyle:{color:sentColors[2],width:2},symbol:'circle',symbolSize:6}
      ]
    };
  } else {
    opt = {
      tooltip:Object.assign({trigger:'axis'},tipCfg),
      legend:{bottom:0,textStyle:{fontSize:10,color:'#64748b'}},
      grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
      xAxis:{type:'category',data:dates,axisLabel:{fontSize:9,rotate:30,color:'#64748b'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
      yAxis:{type:'value',axisLabel:{fontSize:10,color:'#64748b'},axisLine:{show:false},splitLine:{lineStyle:{color:'#f1f5f9'}}},
      series:[
        {name:'总量',type:'bar',data:dailyTrendData.map(function(d){return d.total;}),itemStyle:{color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:'#cbd5e1'},{offset:1,color:'#e2e8f0'}]},borderRadius:[6,6,0,0]},barMaxWidth:22},
        {name:'正面',type:'line',smooth:true,data:dailyTrendData.map(function(d){return d.positive;}),itemStyle:{color:sentColors[0]},lineStyle:{color:sentColors[0],width:2},symbol:'circle',symbolSize:6},
        {name:'负面',type:'line',smooth:true,data:dailyTrendData.map(function(d){return d.negative;}),itemStyle:{color:sentColors[2]},lineStyle:{color:sentColors[2],width:2},symbol:'circle',symbolSize:6}
      ]
    };
  }
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Radar ── */
(function(){
  var c = mkChart('chart-radar'); if(!c)return;
  if(!radarData||!radarData.length)return;
  var maxVal = Math.max.apply(null,radarData.map(function(d){return d.value;}))||1;
  var opt = {
    tooltip:{backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    radar:{
      indicator:radarData.map(function(d){return{name:d.name,max:Math.ceil(maxVal*1.2)};}),
      radius:'65%',
      axisName:{fontSize:10,color:'#64748b'},
      splitArea:{areaStyle:{color:['rgba(250,250,252,0.5)','rgba(245,245,250,0.5)']}},
      splitLine:{lineStyle:{color:'#e2e8f0'}},
      axisLine:{lineStyle:{color:'#e2e8f0'}}
    },
    series:[{
      type:'radar',
      data:[{
        value:radarData.map(function(d){return d.value;}),
        name:'情感指数',
        itemStyle:{color:softColors[0]},
        areaStyle:{color:{type:'radial',x:0.5,y:0.5,r:0.5,colorStops:[{offset:0,color:softenColor(rawPrimary,0.35)},{offset:1,color:softenColor(rawPrimary,0.7)}]},opacity:0.6},
        lineStyle:{color:softColors[0],width:2},
        symbol:'circle',
        symbolSize:6
      }]
    }]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Tags Bar ── */
(function(){
  var c = mkChart('chart-tags'); if(!c)return;
  var names=topTagsData.map(function(d){return d.name;});
  var vals=topTagsData.map(function(d){return d.value;});
  var opt = {
    tooltip:{trigger:'axis',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    grid:{top:10,bottom:10,left:'3%',right:'10%',containLabel:true},
    xAxis:{type:'value',axisLabel:{fontSize:10,color:'#64748b'},axisLine:{show:false},splitLine:{lineStyle:{color:'#f1f5f9'}}},
    yAxis:{type:'category',data:names,axisLabel:{fontSize:11,color:'#475569'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
    series:[{
      type:'bar',
      data:vals.map(function(v,i){
        var base=softColors[i%softColors.length];
        return{
          value:v,
          itemStyle:{
            borderRadius:[0,10,10,0],
            color:{type:'linear',x:0,y:0,x2:1,y2:0,colorStops:[{offset:0,color:softenColor(base,0.35)},{offset:1,color:base}]}
          }
        };
      }),
      label:{show:true,position:'right',fontSize:10,color:'#475569'}
    }]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Topic Bubble / Risk Matrix (Custom SVG) ── */
function renderRiskMatrix(){
  var container = document.getElementById('chart-risk'); if(!container)return;
  container.innerHTML='';
  if(!topicBubbleData||!topicBubbleData.length){
    container.innerHTML='<div style="height:100%;display:flex;align-items:center;justify-content:center;color:#94a3b8;font-size:14px;">暂无话题数据</div>';
    return;
  }
  var W = container.clientWidth||800;
  var H = 380;
  var pad = {top:40, right:50, bottom:60, left:64};
  var maxCount = Math.max.apply(null, topicBubbleData.map(function(d){return d.value[1];}));
  if(maxCount<1) maxCount=1;
  var maxNeg = Math.max(50, Math.ceil(Math.max.apply(null, topicBubbleData.map(function(d){return d.value[0];}))/10)*10);
  function xScale(v){ return pad.left + (v/maxNeg) * (W-pad.left-pad.right); }
  function yScale(v){ return H - pad.bottom - (v/maxCount) * (H-pad.top-pad.bottom); }
  // 缩小气泡：最大半径 32，基础 13
  function rScale(c){ return Math.max(14, Math.min(32, 13 + Math.sqrt(c/maxCount)*19)); }

  var posColor = softenColor(rawSentPos, 0.05);
  var midColor = '#f59e0b';
  var negColor = softenColor(rawSentNeg, 0.05);

  var chartTop = pad.top, chartBottom = H-pad.bottom;
  var zone20 = xScale(20), zone40 = xScale(40);

  // 计算位置（先算好，用于防溢出裁剪）
  // 若多个气泡位置重叠（坐标完全相同），加小偏移让它们分开
  var rawPos = topicBubbleData.map(function(d){
    return {cx: xScale(d.value[0]), cy: yScale(d.value[1])};
  });
  var bubblePos = topicBubbleData.map(function(d,i){
    var r  = rScale(d.value[1]);
    var cx = rawPos[i].cx;
    var cy = rawPos[i].cy;
    // 对重叠的气泡做螺旋偏移（避免完全重叠）
    var overlapCount = 0;
    for(var j=0;j<i;j++){
      var dx=rawPos[j].cx-cx, dy=rawPos[j].cy-cy;
      if(Math.sqrt(dx*dx+dy*dy)<4){ overlapCount++; }
    }
    if(overlapCount>0){
      var angle = overlapCount * Math.PI * 0.75;
      var dist  = overlapCount * (r*0.9+6);
      cx += Math.cos(angle)*dist;
      cy += Math.sin(angle)*dist;
    }
    // 防溢出：把 cx/cy 钳制在可见区内，留出气泡半径 + 2px 边距
    cx = Math.max(pad.left+r+2, Math.min(W-pad.right-r-2, cx));
    cy = Math.max(chartTop+r+2, Math.min(chartBottom-r-2, cy));
    return {cx:cx, cy:cy, r:r};
  });

  var ns = 'http://www.w3.org/2000/svg';
  var svgStr = '<svg viewBox="0 0 '+W+' '+H+'" width="100%" height="'+H+'" xmlns="'+ns+'" style="display:block;">';

  // defs
  svgStr += '<defs>';
  // 轻微发光滤镜（stdDeviation 缩小到 5）
  svgStr += '<filter id="bubble-glow" x="-50%" y="-50%" width="200%" height="200%">';
  svgStr += '<feGaussianBlur in="SourceGraphic" stdDeviation="5" result="blur"/>';
  svgStr += '<feColorMatrix in="blur" mode="matrix" values="1 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 1.2 0"/>';
  svgStr += '</filter>';
  // 高光（细腻）
  svgStr += '<radialGradient id="bubble-hl" cx="33%" cy="28%" r="52%">';
  svgStr += '<stop offset="0%" stop-color="#fff" stop-opacity="0.65"/>';
  svgStr += '<stop offset="45%" stop-color="#fff" stop-opacity="0.18"/>';
  svgStr += '<stop offset="100%" stop-color="#fff" stop-opacity="0"/>';
  svgStr += '</radialGradient>';
  // per-bubble 渐变
  topicBubbleData.forEach(function(d,i){
    var negRate = d.value[0];
    var baseCol = negRate>40 ? negColor : (negRate>20 ? midColor : posColor);
    var lightCol = softenColor(baseCol, 0.38);
    var darkCol  = darkenColor(baseCol, 0.15);
    svgStr += '<radialGradient id="bg-'+i+'" cx="35%" cy="30%" r="72%">';
    svgStr += '<stop offset="0%" stop-color="'+lightCol+'"/>';
    svgStr += '<stop offset="60%" stop-color="'+baseCol+'"/>';
    svgStr += '<stop offset="100%" stop-color="'+darkCol+'"/>';
    svgStr += '</radialGradient>';
    // aura（更淡）
    svgStr += '<radialGradient id="aura-'+i+'" cx="50%" cy="50%" r="50%">';
    svgStr += '<stop offset="0%" stop-color="'+baseCol+'" stop-opacity="0.35"/>';
    svgStr += '<stop offset="100%" stop-color="'+baseCol+'" stop-opacity="0"/>';
    svgStr += '</radialGradient>';
  });
  // zone 背景渐变
  svgStr += '<linearGradient id="zone-safe" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="'+posColor+'" stop-opacity="0.03"/><stop offset="100%" stop-color="'+posColor+'" stop-opacity="0.08"/></linearGradient>';
  svgStr += '<linearGradient id="zone-warn" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="'+midColor+'" stop-opacity="0.03"/><stop offset="100%" stop-color="'+midColor+'" stop-opacity="0.08"/></linearGradient>';
  svgStr += '<linearGradient id="zone-danger" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="'+negColor+'" stop-opacity="0.04"/><stop offset="100%" stop-color="'+negColor+'" stop-opacity="0.10"/></linearGradient>';
  svgStr += '</defs>';

  // zone bands
  svgStr += '<rect x="'+pad.left+'" y="'+chartTop+'" width="'+(zone20-pad.left)+'" height="'+(chartBottom-chartTop)+'" fill="url(#zone-safe)" rx="6"/>';
  svgStr += '<rect x="'+zone20+'" y="'+chartTop+'" width="'+(zone40-zone20)+'" height="'+(chartBottom-chartTop)+'" fill="url(#zone-warn)"/>';
  svgStr += '<rect x="'+zone40+'" y="'+chartTop+'" width="'+(W-pad.right-zone40)+'" height="'+(chartBottom-chartTop)+'" fill="url(#zone-danger)" rx="6"/>';

  // zone dividers
  svgStr += '<line x1="'+zone20+'" y1="'+chartTop+'" x2="'+zone20+'" y2="'+chartBottom+'" stroke="'+midColor+'" stroke-width="1" stroke-dasharray="4,4" opacity="0.35"/>';
  svgStr += '<line x1="'+zone40+'" y1="'+chartTop+'" x2="'+zone40+'" y2="'+chartBottom+'" stroke="'+negColor+'" stroke-width="1" stroke-dasharray="4,4" opacity="0.35"/>';

  // grid lines
  for(var gi=1; gi<5; gi++){
    var gy = chartTop + gi*(chartBottom-chartTop)/5;
    svgStr += '<line x1="'+pad.left+'" y1="'+gy+'" x2="'+(W-pad.right)+'" y2="'+gy+'" stroke="#e2e8f0" stroke-width="1" stroke-dasharray="2,4" opacity="0.5"/>';
  }

  // axes
  svgStr += '<line x1="'+pad.left+'" y1="'+chartBottom+'" x2="'+(W-pad.right)+'" y2="'+chartBottom+'" stroke="#94a3b8" stroke-width="1.5"/>';
  svgStr += '<line x1="'+pad.left+'" y1="'+chartTop+'" x2="'+pad.left+'" y2="'+chartBottom+'" stroke="#94a3b8" stroke-width="1.5"/>';

  // x ticks
  for(var xi=0; xi<=5; xi++){
    var xv = xi*maxNeg/5;
    var xp = xScale(xv);
    svgStr += '<line x1="'+xp+'" y1="'+chartBottom+'" x2="'+xp+'" y2="'+(chartBottom+5)+'" stroke="#94a3b8" stroke-width="1"/>';
    svgStr += '<text x="'+xp+'" y="'+(chartBottom+20)+'" text-anchor="middle" font-size="11" fill="#64748b" font-family="Inter,sans-serif">'+xv.toFixed(0)+'%</text>';
  }
  // y ticks
  for(var yi=0; yi<=5; yi++){
    var yv = yi*maxCount/5;
    var yp = yScale(yv);
    svgStr += '<line x1="'+(pad.left-5)+'" y1="'+yp+'" x2="'+pad.left+'" y2="'+yp+'" stroke="#94a3b8" stroke-width="1"/>';
    svgStr += '<text x="'+(pad.left-10)+'" y="'+(yp+4)+'" text-anchor="end" font-size="11" fill="#64748b" font-family="Inter,sans-serif">'+yv.toFixed(0)+'</text>';
  }

  // axis labels
  svgStr += '<text x="'+(W/2)+'" y="'+(H-12)+'" text-anchor="middle" font-size="12" font-weight="600" fill="#475569">负面率 (%)</text>';
  svgStr += '<text x="22" y="'+(H/2)+'" text-anchor="middle" font-size="12" font-weight="600" fill="#475569" transform="rotate(-90, 22, '+(H/2)+')">文章数 (篇)</text>';

  // zone labels
  svgStr += '<text x="'+((pad.left+zone20)/2)+'" y="'+(chartTop-10)+'" text-anchor="middle" font-size="11" font-weight="700" fill="'+posColor+'" letter-spacing="1">● 安全区</text>';
  svgStr += '<text x="'+((zone20+zone40)/2)+'" y="'+(chartTop-10)+'" text-anchor="middle" font-size="11" font-weight="700" fill="'+midColor+'" letter-spacing="1">● 注意区</text>';
  svgStr += '<text x="'+((zone40+W-pad.right)/2)+'" y="'+(chartTop-10)+'" text-anchor="middle" font-size="11" font-weight="700" fill="'+negColor+'" letter-spacing="1">● 警戒区</text>';

  // 气泡：大的先画（不遮挡小的）
  var sortedIdx = topicBubbleData.map(function(_,i){return i;});
  sortedIdx.sort(function(a,b){return bubblePos[b].r - bubblePos[a].r;});

  // aura 层（轻）
  sortedIdx.forEach(function(i){
    var pos = bubblePos[i];
    svgStr += '<circle cx="'+pos.cx+'" cy="'+pos.cy+'" r="'+(pos.r*1.35)+'" fill="url(#aura-'+i+')" filter="url(#bubble-glow)" pointer-events="none"/>';
  });
  // 底部阴影（更淡、更扁）
  sortedIdx.forEach(function(i){
    var pos = bubblePos[i];
    svgStr += '<ellipse cx="'+pos.cx+'" cy="'+(pos.cy+pos.r*0.78)+'" rx="'+(pos.r*0.55)+'" ry="'+(pos.r*0.10)+'" fill="rgba(15,23,42,0.08)" pointer-events="none"/>';
  });
  // 气泡主体：描边改为 1px 半透明
  sortedIdx.forEach(function(i){
    var pos = bubblePos[i];
    svgStr += '<g class="bubble-group" data-idx="'+i+'">';
    svgStr += '<circle class="risk-bubble" cx="'+pos.cx+'" cy="'+pos.cy+'" r="'+pos.r+'" fill="url(#bg-'+i+')" stroke="rgba(255,255,255,0.55)" stroke-width="1" data-idx="'+i+'"/>';
    svgStr += '<circle cx="'+pos.cx+'" cy="'+pos.cy+'" r="'+pos.r+'" fill="url(#bubble-hl)" pointer-events="none"/>';
    svgStr += '</g>';
  });

  // 悬停标签用 HTML 叠加层（absolute div），不写入 SVG，避免占位
  svgStr += '</svg>';
  container.innerHTML = svgStr;

  // 为每个气泡注入 4 个角落标签（话题名、文章数、负面率、风险等级）
  container.style.position = 'relative';
  topicBubbleData.forEach(function(d,i){
    var pos = bubblePos[i];
    var pctX = pos.cx / W * 100;
    var pctY = pos.cy / H * 100;
    // 判断气泡是否偏下方（超过图表 60% 高度），是则标签向上展开而非向下
    var nearBottom = pos.cy > H * 0.62;
    // 判断气泡是否偏右方，是则左侧标签优先
    var nearRight  = pos.cx > W * 0.68;

    var riskLevel = d.value[0]>40 ? '⚠ 警戒' : (d.value[0]>20 ? '△ 注意' : '✓ 安全');
    var riskColor = d.value[0]>40 ? '#fca5a5' : (d.value[0]>20 ? '#fde68a' : '#6ee7b7');

    // 4 块标签内容：[角落方向 (dx,dy), 文字, 强调色]
    // 角落: TL(-1,-1), TR(1,-1), BL(-1,1), BR(1,1)
    // nearBottom 时上下翻转；nearRight 时左右翻转
    var ySign = nearBottom ? -1 : 1;   // 气泡靠下则全部往上
    var xSign = nearRight  ? -1 : 1;   // 气泡靠右则全部往左（BL/TL优先）
    var tags = [
      {dx:-xSign, dy:-ySign, text: escHtml(d.name),               color:'#e2e8f0', weight:'700'},
      {dx: xSign, dy:-ySign, text: d.value[1]+' 篇',              color:'#bae6fd', weight:'500'},
      {dx:-xSign, dy: ySign, text: d.value[0].toFixed(1)+'% 负面', color:'#fca5a5', weight:'500'},
      {dx: xSign, dy: ySign, text: riskLevel,                      color:riskColor, weight:'600'}
    ];
    var spread = pos.r + 22;  // 展开距离（px）

    tags.forEach(function(tag, ti){
      var el = document.createElement('div');
      el.className = 'risk-corner-label';
      el.setAttribute('data-idx', i);
      el.setAttribute('data-corner', ti);
      // 基础位置在气泡中心，偏移到对应象限后再用 translate 对齐自身
      var txBase = tag.dx < 0 ? '-100%' : '0%';
      var tyBase = tag.dy < 0 ? '-100%' : '0%';
      // 展开时的像素偏移（绝对px，非百分比，写在 style 变量里）
      el.style.cssText =
        'position:absolute;'+
        'left:'+pctX+'%;'+
        'top:'+pctY+'%;'+
        'transform:translate('+txBase+','+tyBase+');'+
        'pointer-events:none;'+
        'opacity:0;'+
        'transition:opacity 200ms ease, left 280ms cubic-bezier(0.34,1.56,0.64,1), top 280ms cubic-bezier(0.34,1.56,0.64,1);'+
        'z-index:20;'+
        'white-space:nowrap;';
      el.innerHTML =
        '<div style="'+
          'background:rgba(15,23,42,0.72);'+
          'backdrop-filter:blur(6px);'+
          '-webkit-backdrop-filter:blur(6px);'+
          'border:1px solid rgba(255,255,255,0.10);'+
          'border-radius:10px;'+
          'padding:3px 9px;'+
          'font-size:11px;'+
          'font-weight:'+tag.weight+';'+
          'color:'+tag.color+';'+
          'box-shadow:0 2px 8px rgba(0,0,0,0.30);'+
        '">'+tag.text+'</div>';
      // 存储展开目标坐标到 dataset
      el.dataset.ox = (pctX + tag.dx * spread / W * 100).toFixed(3);
      el.dataset.oy = (pctY + tag.dy * spread / H * 100).toFixed(3);
      container.appendChild(el);
    });
  });

  // 气泡交互：悬停展开标签，离开收回
  var bubbleEls = container.querySelectorAll('.risk-bubble');
  bubbleEls.forEach(function(el){
    var idx = parseInt(el.getAttribute('data-idx'),10);
    var cornerLabels = container.querySelectorAll('.risk-corner-label[data-idx="'+idx+'"]');
    el.style.cursor = 'pointer';

    el.addEventListener('mouseenter', function(ev){
      // 展开：移动到展开目标坐标并显示
      cornerLabels.forEach(function(lbl){
        lbl.style.left  = lbl.dataset.ox + '%';
        lbl.style.top   = lbl.dataset.oy + '%';
        lbl.style.opacity = '1';
      });
    });
    el.addEventListener('mousemove', function(ev){});
    el.addEventListener('mouseleave', function(){
      // 收回：回到气泡中心坐标（opacity→0，位置归回 pctX/pctY）
      cornerLabels.forEach(function(lbl){
        var pos = bubblePos[idx];
        lbl.style.left    = (pos.cx / W * 100) + '%';
        lbl.style.top     = (pos.cy / H * 100) + '%';
        lbl.style.opacity = '0';
      });
    });
  });
}
renderRiskMatrix();

/* ── Comment Sentiment Pie ── */
(function(){
  if(!commentSentData)return;
  var c = mkChart('chart-comment-sent'); if(!c)return;
  var data=[
    {name:'正面',value:commentSentData.positive,itemStyle:{color:sentColors[0]}},
    {name:'中性',value:commentSentData.neutral,itemStyle:{color:sentColors[1]}},
    {name:'负面',value:commentSentData.negative,itemStyle:{color:sentColors[2]}}
  ];
  var opt = {
    tooltip:{trigger:'item',formatter:'{b}: {c} ({d}%)',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    legend:{orient:'horizontal',bottom:0,textStyle:{fontSize:11,color:'#64748b'}},
    series:[{
      type:'pie',radius:['44%','72%'],
      itemStyle:{borderRadius:10,borderColor:'#fff',borderWidth:2,shadowBlur:8,shadowColor:'rgba(0,0,0,0.06)'},
      label:{formatter:function(p){return p.name+'\n'+p.percent.toFixed(1)+'%';},fontSize:11,color:'#475569'},
      emphasis:{itemStyle:{shadowBlur:18,shadowColor:'rgba(0,0,0,0.25)'},scale:true,scaleSize:5},
      data:data
    }]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Comment Platform Bar ── */
(function(){
  if(!commentPlatData)return;
  var c = mkChart('chart-comment-plat'); if(!c)return;
  var names=[]; var vals=[];
  for(var k in commentPlatData){ names.push(k); vals.push(commentPlatData[k]); }
  var paired=names.map(function(n,i){return{n:n,v:vals[i]};});
  paired.sort(function(a,b){return b.v-a.v;});
  names=paired.map(function(p){return p.n;});
  vals=paired.map(function(p){return p.v;});
  var opt = {
    tooltip:{trigger:'axis',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    grid:{top:10,bottom:10,left:'3%',right:'10%',containLabel:true},
    xAxis:{type:'value',axisLabel:{fontSize:10,color:'#64748b'},axisLine:{show:false},splitLine:{lineStyle:{color:'#f1f5f9'}}},
    yAxis:{type:'category',data:names,axisLabel:{fontSize:11,color:'#475569'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
    series:[{
      type:'bar',
      data:vals.map(function(v,i){
        var base=softColors[i%softColors.length];
        return{value:v,itemStyle:{borderRadius:[0,10,10,0],color:{type:'linear',x:0,y:0,x2:1,y2:0,colorStops:[{offset:0,color:softenColor(base,0.35)},{offset:1,color:base}]}}};
      }),
      label:{show:true,position:'right',fontSize:10,color:'#475569'}
    }]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Comment Trend ── */
(function(){
  if(!commentTrendData||!commentTrendData.length)return;
  var c = mkChart('chart-comment-trend'); if(!c)return;
  var dates=commentTrendData.map(function(d){return d.date;});
  var opt = {
    tooltip:{trigger:'axis',backgroundColor:'rgba(15,23,42,0.95)',borderColor:'transparent',textStyle:{color:'#fff',fontSize:12}},
    legend:{bottom:0,textStyle:{fontSize:10,color:'#64748b'}},
    grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
    xAxis:{type:'category',data:dates,axisLabel:{fontSize:9,rotate:30,color:'#64748b'},axisLine:{lineStyle:{color:'#cbd5e1'}}},
    yAxis:{type:'value',axisLabel:{fontSize:10,color:'#64748b'},axisLine:{show:false},splitLine:{lineStyle:{color:'#f1f5f9'}}},
    series:[
      {name:'正面',type:'line',smooth:true,stack:'ct',areaStyle:{opacity:0.5,color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:sentColors[0]},{offset:1,color:softenColor(sentColors[0],0.6)}]}},data:commentTrendData.map(function(d){return d.positive;}),itemStyle:{color:sentColors[0]},lineStyle:{color:sentColors[0],width:2}},
      {name:'中性',type:'line',smooth:true,stack:'ct',areaStyle:{opacity:0.5,color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:sentColors[1]},{offset:1,color:softenColor(sentColors[1],0.6)}]}},data:commentTrendData.map(function(d){return d.neutral;}),itemStyle:{color:sentColors[1]},lineStyle:{color:sentColors[1],width:2}},
      {name:'负面',type:'line',smooth:true,stack:'ct',areaStyle:{opacity:0.5,color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:sentColors[2]},{offset:1,color:softenColor(sentColors[2],0.6)}]}},data:commentTrendData.map(function(d){return d.negative;}),itemStyle:{color:sentColors[2]},lineStyle:{color:sentColors[2],width:2}}
    ]
  };
  Object.assign(opt,commonAnim);
  c.setOption(opt);
})();

/* ── Comment Topic Opinion Cards ── */
(function(){
  var el=document.getElementById('comment-topic-cards'); if(!el)return;
  if(!commentTopicData||!commentTopicData.length)return;
  commentTopicData.forEach(function(t){
    var card=document.createElement('div'); card.className='topic-card';
    var sentBar='<div style="display:flex;height:6px;border-radius:3px;overflow:hidden;margin:8px 0;box-shadow:inset 0 1px 2px rgba(0,0,0,0.06);">';
    var total=t.sentiment.positive+t.sentiment.neutral+t.sentiment.negative;
    if(total>0){
      sentBar+='<div style="width:'+((t.sentiment.positive/total*100).toFixed(1))+'%;background:linear-gradient(90deg,'+sentColors[0]+','+softenColor(sentColors[0],0.3)+');"></div>';
      sentBar+='<div style="width:'+((t.sentiment.neutral/total*100).toFixed(1))+'%;background:linear-gradient(90deg,'+sentColors[1]+','+softenColor(sentColors[1],0.3)+');"></div>';
      sentBar+='<div style="width:'+((t.sentiment.negative/total*100).toFixed(1))+'%;background:linear-gradient(90deg,'+sentColors[2]+','+softenColor(sentColors[2],0.3)+');"></div>';
    }
    sentBar+='</div>';
    var sentLegend='<div style="display:flex;gap:12px;font-size:11px;color:var(--text-secondary);margin-bottom:10px;">';
    if(total>0){
      sentLegend+='<span style="display:flex;align-items:center;gap:3px;"><span style="width:7px;height:7px;border-radius:50%;background:'+sentColors[0]+';display:inline-block;box-shadow:0 0 0 2px rgba(255,255,255,0.6);"></span>正面 '+t.sentiment.positive+'</span>';
      sentLegend+='<span style="display:flex;align-items:center;gap:3px;"><span style="width:7px;height:7px;border-radius:50%;background:'+sentColors[1]+';display:inline-block;box-shadow:0 0 0 2px rgba(255,255,255,0.6);"></span>中性 '+t.sentiment.neutral+'</span>';
      sentLegend+='<span style="display:flex;align-items:center;gap:3px;"><span style="width:7px;height:7px;border-radius:50%;background:'+sentColors[2]+';display:inline-block;box-shadow:0 0 0 2px rgba(255,255,255,0.6);"></span>负面 '+t.sentiment.negative+'</span>';
    }
    sentLegend+='</div>';
    var opinions='';
    if(t.keyOpinions&&t.keyOpinions.length){
      opinions='<div style="margin:8px 0 0;padding:12px 16px;background:linear-gradient(135deg,rgba(0,0,0,0.025),rgba(0,0,0,0.01));border-radius:12px;border:1px solid rgba(0,0,0,0.04);">';
      opinions+='<div style="font-size:11px;font-weight:600;color:var(--text-secondary);margin-bottom:8px;letter-spacing:0.05em;text-transform:uppercase;display:flex;align-items:center;gap:6px;"><span style="width:3px;height:10px;border-radius:1.5px;background:linear-gradient(180deg,var(--primary),var(--accent));"></span>核心观点</div>';
      opinions+='<ul style="margin:0;padding-left:18px;font-size:12.5px;color:var(--text-primary);line-height:1.85;">';
      t.keyOpinions.forEach(function(o){opinions+='<li style="margin:3px 0;">'+escHtml(o)+'</li>';});
      opinions+='</ul></div>';
    }
    card.innerHTML='<div style="font-weight:600;font-size:14px;color:var(--text-primary);display:flex;align-items:center;gap:8px;letter-spacing:-0.01em;">'+escHtml(t.topic)+
      '<span style="font-size:11px;background:linear-gradient(135deg,var(--primary),var(--accent));color:#fff;border-radius:12px;padding:2px 10px;font-weight:600;box-shadow:0 2px 8px rgba(0,0,0,0.08);">'+t.commentCount+' 条评论</span></div>'+
      sentBar+sentLegend+opinions;
    el.appendChild(card);
  });
})();

/* ── Hot Comments Table ── */
(function(){
  var tbody=document.getElementById('hot-comments-tbody'); if(!tbody)return;
  if(!hotCommentsData||!hotCommentsData.length)return;
  hotCommentsData.forEach(function(c,i){
    var cls=c.sentiment==='positive'?'badge-pos':(c.sentiment==='negative'?'badge-neg':'badge-neu');
    var sentLabel=c.sentiment==='positive'?'正面':(c.sentiment==='negative'?'负面':'中性');
    var tr=document.createElement('tr');
    tr.innerHTML='<td style="color:var(--text-secondary);font-size:12px;font-variant-numeric:tabular-nums;">'+(i+1)+'</td>'+
      '<td style="max-width:400px;word-break:break-all;font-size:13px;line-height:1.6;">'+escHtml(c.content)+'</td>'+
      '<td style="font-size:12px;color:var(--text-secondary);">'+escHtml(c.nickname)+'</td>'+
      '<td style="font-size:12px;color:var(--text-secondary);">'+escHtml(c.platform)+'</td>'+
      '<td style="font-size:12px;font-weight:600;font-variant-numeric:tabular-nums;">'+c.likeCount+'</td>'+
      '<td><span class="badge '+cls+'">'+sentLabel+'</span></td>';
    tbody.appendChild(tr);
  });
})();

/* ── Tag Cloud (spiral placement, cloud-shaped) ── */
(function(){
  var el = document.getElementById('tag-cloud'); if(!el)return;
  if(!tagCloudData||!tagCloudData.length)return;
  el.style.position='relative';
  el.style.height='300px';
  el.style.overflow='hidden';
  el.style.display='block';
  var W=el.clientWidth||700, H=300;
  var maxVal = Math.max.apply(null,tagCloudData.map(function(d){return d.value;}));
  var minVal = Math.min.apply(null,tagCloudData.map(function(d){return d.value;}));
  var tagColors = softColors.length>=5 ? softColors : ['#6366f1','#0ea5e9','#10b981','#f59e0b','#ef4444','#8b5cf6','#ec4899'];
  var sorted = tagCloudData.slice().sort(function(a,b){return b.value-a.value;});
  var placed = [];
  var GAP = 10;
  function collides(x,y,w,h){
    for(var i=0;i<placed.length;i++){
      var p=placed[i];
      if(!(x+w+GAP<p.x||x>p.x+p.w+GAP||y+h+GAP<p.y||y>p.y+p.h+GAP)) return true;
    }
    return false;
  }
  var measurer=document.createElement('span');
  measurer.style.position='absolute';measurer.style.visibility='hidden';measurer.style.whiteSpace='nowrap';
  measurer.style.fontFamily="Inter,'Noto Sans SC','PingFang SC',sans-serif";
  document.body.appendChild(measurer);
  sorted.forEach(function(d,i){
    var ratio = maxVal>minVal ? (d.value-minVal)/(maxVal-minVal) : 0.5;
    var fontSize = Math.round(14 + ratio*28);
    var fontWeight = ratio>0.6?'700':(ratio>0.3?'500':'400');
    var opacity = 0.65 + ratio*0.35;
    var color = tagColors[i%tagColors.length];
    measurer.style.fontSize=fontSize+'px';
    measurer.style.fontWeight=fontWeight;
    measurer.textContent=d.name;
    var tw = measurer.offsetWidth+4;
    var th = fontSize+8;
    var angle=0, radius=0;
    var px, py, found=false;
    for(var tries=0;tries<800;tries++){
      px=W/2+radius*Math.cos(angle)-tw/2;
      py=H/2+radius*Math.sin(angle)*0.62-th/2;
      if(px>=0&&py>=0&&px+tw<=W&&py+th<=H&&!collides(px,py,tw,th)){
        found=true; break;
      }
      angle+=0.35; radius+=0.8;
    }
    if(!found) return;
    placed.push({x:px,y:py,w:tw,h:th});
    var span=document.createElement('span');
    span.textContent=d.name;
    span.style.position='absolute';
    span.style.left=px+'px';
    span.style.top=py+'px';
    span.style.fontSize=fontSize+'px';
    span.style.fontWeight=fontWeight;
    span.style.color=color;
    span.style.opacity=opacity;
    span.style.cursor='default';
    span.style.whiteSpace='nowrap';
    span.style.transition='transform 0.25s cubic-bezier(0.34,1.56,0.64,1),text-shadow 0.25s';
    span.onmouseover=function(){this.style.transform='scale(1.18)';this.style.textShadow='0 4px 12px rgba(0,0,0,0.15)';};
    span.onmouseout=function(){this.style.transform='scale(1)';this.style.textShadow='none';};
    el.appendChild(span);
  });
  document.body.removeChild(measurer);
})();

/* ── Top Articles Table ── */
(function(){
  var tbody = document.getElementById('top-articles-tbody'); if(!tbody)return;
  topArticlesData.forEach(function(a,i){
    var sent = a.sentiment||'';
    var cls  = sent==='正面'?'badge-pos':(sent==='负面'?'badge-neg':'badge-neu');
    var score= typeof a.sentScore==='number'?a.sentScore:0;
    var pct  = Math.min(100, Math.round(score));
    var barColor= sent==='正面'?sentColors[0]:(sent==='负面'?sentColors[2]:sentColors[1]);
    var tr=document.createElement('tr');
    tr.innerHTML='<td style="color:var(--text-secondary);font-size:12px;font-variant-numeric:tabular-nums;">'+(i+1)+'</td>'+
      '<td style="max-width:420px;word-break:break-all;line-height:1.6;">'+escHtml(a.title)+'</td>'+
      '<td style="font-size:12px;color:var(--text-secondary);">'+escHtml(a.platform||'')+'</td>'+
      '<td><span class="badge '+cls+'">'+escHtml(sent)+'</span></td>'+
      '<td><div class="score-bar-wrap"><div class="score-bar-bg"><div class="score-bar-fill" style="width:'+pct+'%;background:linear-gradient(90deg,'+barColor+','+softenColor(barColor,0.3)+');"></div></div><span style="font-size:11px;color:var(--text-secondary);font-variant-numeric:tabular-nums;">'+score.toFixed(2)+'</span></div></td>';
    tbody.appendChild(tr);
  });
})();

/* ── Animated Counters ── */
(function(){
  var els = document.querySelectorAll('.kpi-value[data-target]');
  var duration = 1100;
  els.forEach(function(el){
    var target = parseInt(el.getAttribute('data-target'),10)||0;
    var start = null;
    function step(ts){
      if(!start) start=ts;
      var prog = Math.min((ts-start)/duration,1);
      var ease = 1-Math.pow(1-prog,3);
      el.textContent = Math.round(ease*target).toLocaleString();
      if(prog<1) requestAnimationFrame(step);
      else el.textContent = target.toLocaleString();
    }
    requestAnimationFrame(step);
  });
})();

/* ── Scroll-in Section Reveal ── */
(function(){
  var sections = document.querySelectorAll('.section');
  if(!('IntersectionObserver' in window)){
    sections.forEach(function(s){s.classList.add('visible');});
    return;
  }
  var io = new IntersectionObserver(function(entries){
    entries.forEach(function(e){
      if(e.isIntersecting){
        e.target.classList.add('visible');
        io.unobserve(e.target);
      }
    });
  },{threshold:0.08, rootMargin:'0px 0px -40px 0px'});
  sections.forEach(function(s){io.observe(s);});
})();

/* ── Resize ── */
var resizeTimer;
window.addEventListener('resize',function(){
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(function(){
    allCharts.forEach(function(c){try{c.resize();}catch(e){}});
    renderRiskMatrix();
  },150);
});

})();
</script>
</body>
</html>`
