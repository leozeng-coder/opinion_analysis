package report

// htmlTemplate is the Go html/template source for the HTML analysis report.
const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>舆情分析报告 — 采集任务 #{{.CrawlerRunID}}</title>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
<style>
:root{
  --primary:     {{.Theme.Primary}};
  --secondary:   {{.Theme.Secondary}};
  --accent:      {{.Theme.Accent}};
  --card-bg:     {{.Theme.CardBg}};
  --text-primary:   {{.Theme.TextPrimary}};
  --text-secondary: {{.Theme.TextSecondary}};
  --sent-pos:    {{.Theme.SentPos}};
  --sent-neu:    {{.Theme.SentNeu}};
  --sent-neg:    {{.Theme.SentNeg}};
  --radius:      {{.Theme.BorderRadius}};
  --shadow:      {{.Theme.Shadow | css}};
  --page-bg:     {{.Theme.PageBackground | css}};
}
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0;}
body{font-family:'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;background:var(--page-bg);color:var(--text-primary);line-height:1.6;}
.page{max-width:1280px;margin:0 auto;padding:0 24px 48px;}
/* ── Hero ── */
.hero{background:{{.Theme.HeaderGradient | css}};border-radius:var(--radius);padding:40px 48px 36px;margin-bottom:32px;position:relative;overflow:hidden;color:#fff;}
.hero::before{content:'';position:absolute;top:-80px;right:-80px;width:320px;height:320px;border-radius:50%;background:rgba(255,255,255,0.12);pointer-events:none;}
.hero::after{content:'';position:absolute;bottom:-60px;left:40%;width:220px;height:220px;border-radius:50%;background:rgba(255,255,255,0.08);pointer-events:none;}
.hero-overlay{position:absolute;inset:0;background:rgba(255,255,255,0.06);pointer-events:none;}
.hero-inner{position:relative;z-index:1;}
.hero-title{font-size:28px;font-weight:700;display:flex;align-items:center;gap:12px;margin-bottom:10px;}
.hero-title .icon{font-size:32px;}
.theme-badge{display:inline-flex;align-items:center;gap:6px;background:rgba(255,255,255,0.20);border-radius:20px;padding:3px 14px;font-size:13px;font-weight:500;margin-bottom:14px;}
.hero-meta{display:flex;flex-wrap:wrap;gap:8px 24px;font-size:13px;opacity:0.88;}
.hero-meta span{display:flex;align-items:center;gap:5px;}
/* ── KPI ── */
.kpi-grid{display:grid;grid-template-columns:repeat(5,1fr);gap:16px;margin-bottom:28px;}
.kpi{background:rgba(255,255,255,0.82);backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);border-radius:var(--radius);padding:20px 18px 16px;box-shadow:0 2px 16px rgba(0,0,0,0.06),0 1px 4px rgba(0,0,0,0.04);border-top:3px solid var(--primary);border:1px solid rgba(255,255,255,0.9);border-top:3px solid var(--primary);transition:transform 0.2s,box-shadow 0.2s;cursor:default;}
.kpi:hover{transform:translateY(-4px);box-shadow:0 8px 28px rgba(0,0,0,0.10);}
.kpi-icon{font-size:22px;margin-bottom:6px;}
.kpi-label{font-size:12px;color:var(--text-secondary);margin-bottom:4px;}
.kpi-value{font-size:30px;font-weight:700;color:var(--primary);line-height:1.1;}
.kpi-sub{font-size:11px;color:var(--text-secondary);margin-top:4px;}
/* ── Risk banner ── */
.risk-banner{background:linear-gradient(90deg,#fff3cd,#fef9e7);border:1px solid #fbbf24;border-left:4px solid #f59e0b;border-radius:var(--radius);padding:14px 20px;margin-bottom:28px;display:flex;align-items:center;gap:12px;font-size:14px;}
.pulse-dot{width:10px;height:10px;border-radius:50%;background:#ef4444;flex-shrink:0;animation:pulse 1.4s infinite;}
@keyframes pulse{0%,100%{box-shadow:0 0 0 0 rgba(239,68,68,0.5);}50%{box-shadow:0 0 0 7px rgba(239,68,68,0);}}
/* ── Sections ── */
.section{margin-bottom:36px;}
.section-header{display:flex;align-items:center;gap:10px;margin-bottom:16px;}
.section-icon{font-size:20px;}
.section-title{font-size:17px;font-weight:600;color:var(--text-primary);}
.section-line{flex:1;height:1px;background:linear-gradient(90deg,var(--primary) 0%,transparent 100%);border-radius:2px;opacity:0.25;margin-left:4px;}
/* ── Grid layouts ── */
.grid-2{display:grid;grid-template-columns:1fr 1fr;gap:20px;}
.grid-1{display:grid;grid-template-columns:1fr;gap:20px;}
.chart-grid{margin-bottom:16px;}
.topic-grid{display:grid;grid-template-columns:1fr 1fr;gap:16px;}
.grid-3{display:grid;grid-template-columns:1fr 1fr 1fr;gap:20px;}
/* ── Chart box ── */
.chart-box{background:rgba(255,255,255,0.95);border-radius:var(--radius);padding:20px;box-shadow:0 2px 12px rgba(0,0,0,0.05),0 1px 3px rgba(0,0,0,0.03);border:1px solid rgba(255,255,255,0.85);}
.chart-box h4{font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:12px;text-transform:uppercase;letter-spacing:0.05em;opacity:0.75;}
/* ── Glass variant (deepens glass for all cards when active) ── */
.card-glass .chart-box{background:rgba(255,255,255,0.75);backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);}
.card-glass .kpi{background:rgba(255,255,255,0.72);}
.card-glass .topic-card{background:rgba(255,255,255,0.72);}
.card-glass .conclusion-box{background:rgba(255,255,255,0.70);}
/* ── Accent-bar variant ── */
.accent-bar .topic-card{border-left:4px solid var(--primary);padding-left:16px;}
/* ── Platform heatmap ── */
.heat-grid{display:grid;grid-template-columns:90px repeat(3,1fr);gap:6px;}
.heat-cell{border-radius:8px;padding:12px 6px;text-align:center;}
.heat-header{font-size:12px;font-weight:600;color:var(--text-secondary);display:flex;align-items:center;justify-content:center;padding:6px 0;}
.heat-label{font-size:12px;font-weight:500;color:var(--text-primary);display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.03);border-radius:8px;padding:6px 4px;}
.heat-val{font-size:18px;font-weight:700;display:block;}
.heat-pct{font-size:11px;opacity:0.80;}
/* ── Topic cards ── */
.topic-card-grid{display:grid;grid-template-columns:1fr 1fr;gap:18px;}
.topic-card{background:rgba(255,255,255,0.82);backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);border-radius:var(--radius);padding:18px 20px;box-shadow:0 2px 14px rgba(0,0,0,0.05),0 1px 4px rgba(0,0,0,0.04);border:1px solid rgba(255,255,255,0.9);}
.topic-name{font-size:15px;font-weight:600;margin-bottom:4px;display:flex;align-items:center;gap:8px;flex-wrap:wrap;}
.topic-count-badge{font-size:11px;background:var(--primary);color:#fff;border-radius:12px;padding:1px 9px;font-weight:500;}
.sent-bar{height:8px;border-radius:4px;display:flex;overflow:hidden;margin:8px 0;}
.sent-bar-pos{background:var(--sent-pos);}
.sent-bar-neu{background:var(--sent-neu);}
.sent-bar-neg{background:var(--sent-neg);}
.sent-legend{display:flex;gap:12px;font-size:12px;color:var(--text-secondary);margin-bottom:8px;flex-wrap:wrap;}
.sent-legend span{display:flex;align-items:center;gap:4px;}
.sent-dot{width:8px;height:8px;border-radius:50%;display:inline-block;}
.topic-summary{font-size:13px;color:var(--text-secondary);line-height:1.55;}
/* ── Tag cloud ── */
.tag-cloud-wrap{position:relative;min-height:280px;padding:12px 4px;}
.tag-chip{display:inline-block;padding:2px 12px;border-radius:20px;background:rgba(0,0,0,0.05);cursor:default;transition:transform 0.15s,background 0.15s;}
.tag-chip:hover{transform:scale(1.12);background:rgba(0,0,0,0.10);}
/* ── Table ── */
.report-table{width:100%;border-collapse:collapse;font-size:13px;}
.report-table th{background:rgba(0,0,0,0.04);font-weight:600;color:var(--text-secondary);padding:10px 12px;text-align:left;border-bottom:2px solid rgba(0,0,0,0.07);}
.report-table td{padding:9px 12px;border-bottom:1px solid rgba(0,0,0,0.05);vertical-align:middle;}
.report-table tr:last-child td{border-bottom:none;}
.report-table tr:hover td{background:rgba(0,0,0,0.02);}
.badge{display:inline-block;padding:2px 9px;border-radius:10px;font-size:11px;font-weight:600;}
.badge-pos{background:#d1fae5;color:#065f46;}
.badge-neu{background:#e2e8f0;color:#475569;}
.badge-neg{background:#fee2e2;color:#991b1b;}
.score-bar-wrap{display:flex;align-items:center;gap:6px;}
.score-bar-bg{width:70px;height:6px;background:#f1f5f9;border-radius:3px;overflow:hidden;}
.score-bar-fill{height:100%;border-radius:3px;}
/* ── Conclusion ── */
.conclusion-box{background:rgba(255,255,255,0.82);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);border-radius:var(--radius);padding:28px 32px;box-shadow:0 2px 16px rgba(0,0,0,0.05),0 1px 4px rgba(0,0,0,0.04);border:1px solid rgba(255,255,255,0.9);position:relative;}
.conclusion-quote{position:absolute;top:16px;left:20px;font-size:60px;color:var(--primary);opacity:0.12;font-family:Georgia,serif;line-height:1;}
.conclusion-text{font-size:14px;line-height:1.85;color:var(--text-primary);position:relative;z-index:1;white-space:pre-wrap;}
/* ── Subtitle ── */
.chart-subtitle{font-size:12px;color:var(--text-secondary);margin-bottom:10px;margin-top:-6px;}
/* ── Footer ── */
.footer{text-align:center;padding:24px 0 8px;border-top:1px solid rgba(0,0,0,0.08);color:var(--text-secondary);font-size:12px;margin-top:16px;}
/* ── Responsive ── */
@media(max-width:960px){.grid-2,.topic-card-grid,.topic-grid{grid-template-columns:1fr;}.grid-3{grid-template-columns:1fr 1fr;}}
@media(max-width:900px){.kpi-grid{grid-template-columns:repeat(3,1fr);}}
@media(max-width:600px){.kpi-grid{grid-template-columns:1fr 1fr;}.grid-3{grid-template-columns:1fr;}.hero{padding:28px 24px 24px;}.hero-title{font-size:21px;}}
</style>
</head>
<body>
<div class="page{{if .Theme.Variant.CardGlass}} card-glass{{end}}{{if .Theme.Variant.AccentBar}} accent-bar{{end}}">

<!-- ══ Hero ══ -->
<div class="hero">
  <div class="hero-overlay"></div>
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
  <div class="kpi" style="border-top-color:var(--sent-pos);">
    <div class="kpi-icon">{{.Theme.Icons.Positive}}</div>
    <div class="kpi-label">正面情感</div>
    <div class="kpi-value" style="color:var(--sent-pos);" data-target="{{.SentimentPos}}">0</div>
    <div class="kpi-sub">占比 {{printf "%.1f" .SentPosRate}}%</div>
  </div>
  <div class="kpi" style="border-top-color:var(--sent-neu);">
    <div class="kpi-icon">{{.Theme.Icons.Neutral}}</div>
    <div class="kpi-label">中性情感</div>
    <div class="kpi-value" style="color:var(--sent-neu);" data-target="{{.SentimentNeu}}">0</div>
    <div class="kpi-sub">占比 {{printf "%.1f" .SentNeuRate}}%</div>
  </div>
  <div class="kpi" style="border-top-color:var(--sent-neg);">
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
    <span class="section-title">舆情概览</span>
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
    <span class="section-title">情感多维拆解</span>
    <div class="section-line"></div>
  </div>
  <div class="grid-2" style="margin-bottom:20px;">
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
    <span class="section-title">话题热度 Top 10</span>
    <div class="section-line"></div>
  </div>
  <div class="chart-box">
    <div id="chart-tags" class="chart-div" style="height:320px;"></div>
  </div>
</div>

<!-- ══ Section: 话题风险矩阵 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Topics}}</span>
    <span class="section-title">话题风险矩阵</span>
    <div class="section-line"></div>
  </div>
  <div class="chart-box">
    <p class="chart-subtitle">X轴：负面率，Y轴：文章量，气泡越大风险越高</p>
    <div id="chart-risk" class="chart-div" style="height:340px;"></div>
  </div>
</div>

<!-- ══ Section: 话题深度分析 ══ -->
<div class="section">
  <div class="section-header">
    <span class="section-icon">{{.Theme.Icons.Topics}}</span>
    <span class="section-title">话题深度分析</span>
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
    <span class="section-title">热词标签云</span>
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
    <span class="section-title">评论深度分析</span>
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
  <div class="chart-box" style="margin-top:16px;">
    <h4>话题评论观点</h4>
    <div id="comment-topic-cards" class="topic-grid"></div>
  </div>
  <div class="chart-box" style="margin-top:16px;">
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
    <span class="section-title">高影响力内容 Top 10</span>
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
    <span class="section-title">综合分析结论</span>
    <div class="section-line"></div>
  </div>
  <div class="conclusion-box">
    <div class="conclusion-quote">"</div>
    <p class="conclusion-text">{{.Conclusion}}</p>
  </div>
</div>

<!-- ══ Footer ══ -->
<div class="footer">
  本报告由舆情监控平台自动生成 · 主题「{{.Theme.Name}}」· 数据有效期 7 天
</div>

</div><!-- .page -->

<script>
(function(){
'use strict';
var variant      = {{.ChartVariantJSON}};
var chartColors  = {{.ChartColorsJSON}};
var sentColors   = [softenColor('{{.Theme.SentPos}}',0.15),softenColor('{{.Theme.SentNeu}}',0.10),softenColor('{{.Theme.SentNeg}}',0.15)];
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

// 将颜色轻微淡化（混入25%白色），使图表色更淡雅
function softenColor(hex,amt){
  var r=parseInt(hex.slice(1,3),16),g=parseInt(hex.slice(3,5),16),b=parseInt(hex.slice(5,7),16);
  r=Math.round(r+(255-r)*amt); g=Math.round(g+(255-g)*amt); b=Math.round(b+(255-b)*amt);
  return '#'+[r,g,b].map(function(v){return v.toString(16).padStart(2,'0');}).join('');
}
var softColors = chartColors.map(function(c){ return /^#[0-9a-fA-F]{6}$/.test(c)?softenColor(c,0.25):c; });

function mkChart(id){ var el=document.getElementById(id); if(!el)return null; var c=echarts.init(el,null,{renderer:'canvas'}); allCharts.push(c); return c; }

/* ── Platform Distribution ── */
(function(){
  var c = mkChart('chart-platform'); if(!c)return;
  var pie = variant.pieStyle;
  var radiusCfg, roseType;
  if(pie==='donut'){    radiusCfg=['42%','70%']; roseType=undefined; }
  else if(pie==='nightingale'){ radiusCfg=['15%','70%']; roseType='radius'; }
  else {                radiusCfg=['20%','68%']; roseType='area'; }
  var seriesCfg = {
    type:'pie', radius:radiusCfg,
    itemStyle:{borderRadius:8,borderColor:'#fff',borderWidth:2},
    emphasis:{itemStyle:{shadowBlur:10,shadowOffsetX:0,shadowColor:'rgba(0,0,0,0.3)'}},
    data:platformData.map(function(d,i){return{name:d.name,value:d.value,itemStyle:{color:softColors[i%softColors.length]}};})
  };
  if(roseType) seriesCfg.roseType = roseType;
  c.setOption({tooltip:{trigger:'item'},legend:{orient:'horizontal',bottom:0,textStyle:{fontSize:11}},series:[seriesCfg]});
})();

/* ── Sentiment Structure ── */
(function(){
  var c = mkChart('chart-sentiment'); if(!c)return;
  c.setOption({
    tooltip:{trigger:'item',formatter:'{b}: {c} ({d}%)'},
    legend:{orient:'horizontal',bottom:0,textStyle:{fontSize:11}},
    series:[{
      type:'pie', radius:['48%','74%'],
      itemStyle:{borderRadius:8,borderColor:'#fff',borderWidth:2},
      label:{formatter:function(p){return p.name+'\n'+p.percent.toFixed(1)+'%';},fontSize:12},
      emphasis:{label:{show:true,fontSize:14,fontWeight:'bold'},itemStyle:{shadowBlur:14,shadowColor:'rgba(0,0,0,0.25)'}},
      data:sentimentData.map(function(d,i){return{name:d.name,value:d.value,itemStyle:{color:sentColors[i%3]}};})
    }]
  });
})();

/* ── Platform Heatmap (CSS) ── */
(function(){
  var el = document.getElementById('plat-heat-grid'); if(!el)return;
  if(!platformSentData||!platformSentData.platforms||!platformSentData.series)return;
  var platforms = platformSentData.platforms;
  var series = platformSentData.series; // [{name:'正面',data:[]},{name:'中性',data:[]},{name:'负面',data:[]}]
  var posPalette=['#d1fae5','#4ade80','#059669'];
  var neuPalette=['#f1f5f9','#94a3b8','#475569'];
  var negPalette=['#fee2e2','#fca5a5','#ef4444'];
  var palettes=[posPalette,neuPalette,negPalette];
  var darkText='#1e293b'; var lightText='#fff';
  var grid = document.createElement('div'); grid.className='heat-grid';
  // header row
  var corner=document.createElement('div'); corner.className='heat-header'; grid.appendChild(corner);
  ['正面','中性','负面'].forEach(function(h){
    var d=document.createElement('div'); d.className='heat-header'; d.textContent=h; grid.appendChild(d);
  });
  // data rows
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
  c.setOption({
    tooltip:{trigger:'axis',axisPointer:{type:'shadow'}},
    legend:{bottom:0,textStyle:{fontSize:11}},
    grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
    xAxis:{type:'value',axisLabel:{fontSize:11}},
    yAxis:{type:'category',data:topicSentData.topics,axisLabel:{fontSize:11,width:80,overflow:'truncate'}},
    series:topicSentData.series.map(function(s,i){return{
      name:s.name,type:'bar',stack:'total',data:s.data,itemStyle:{color:sentColors[i%3]},
      label:{show:false}
    };})
  });
})();

/* ── Score Bucket ── */
(function(){
  var c = mkChart('chart-score'); if(!c)return;
  var gradColors=['#ef4444','#f97316','#eab308','#84cc16','#22c55e'];
  c.setOption({
    tooltip:{trigger:'axis'},
    grid:{top:10,bottom:30,left:'3%',right:'3%',containLabel:true},
    xAxis:{type:'category',data:scoreBucketData.map(function(d){return d.name;}),axisLabel:{fontSize:10}},
    yAxis:{type:'value',axisLabel:{fontSize:10}},
    series:[{
      type:'bar',data:scoreBucketData.map(function(d,i){return{value:d.value,itemStyle:{color:gradColors[i%gradColors.length],borderRadius:[8,8,0,0]}};}),
      label:{show:true,position:'top',fontSize:10}
    }]
  });
})();

/* ── Daily Trend ── */
(function(){
  var c = mkChart('chart-trend'); if(!c)return;
  var dates = dailyTrendData.map(function(d){return d.date;});
  if(variant.trendStyle==='area'){
    c.setOption({
      tooltip:{trigger:'axis'},
      legend:{bottom:0,textStyle:{fontSize:10}},
      grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
      xAxis:{type:'category',data:dates,axisLabel:{fontSize:9,rotate:30}},
      yAxis:{type:'value',axisLabel:{fontSize:10}},
      series:[
        {name:'正面',type:'line',smooth:true,stack:'trend',areaStyle:{opacity:0.35},data:dailyTrendData.map(function(d){return d.positive;}),itemStyle:{color:sentColors[0]},lineStyle:{color:sentColors[0]}},
        {name:'中性',type:'line',smooth:true,stack:'trend',areaStyle:{opacity:0.35},data:dailyTrendData.map(function(d){return d.neutral;}),itemStyle:{color:sentColors[1]},lineStyle:{color:sentColors[1]}},
        {name:'负面',type:'line',smooth:true,stack:'trend',areaStyle:{opacity:0.35},data:dailyTrendData.map(function(d){return d.negative;}),itemStyle:{color:sentColors[2]},lineStyle:{color:sentColors[2]}}
      ]
    });
  } else {
    c.setOption({
      tooltip:{trigger:'axis'},
      legend:{bottom:0,textStyle:{fontSize:10}},
      grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
      xAxis:{type:'category',data:dates,axisLabel:{fontSize:9,rotate:30}},
      yAxis:{type:'value',axisLabel:{fontSize:10}},
      series:[
        {name:'总量',type:'bar',data:dailyTrendData.map(function(d){return d.total;}),itemStyle:{color:'#94a3b8',borderRadius:[6,6,0,0]},barMaxWidth:20},
        {name:'正面',type:'line',smooth:true,data:dailyTrendData.map(function(d){return d.positive;}),itemStyle:{color:sentColors[0]},lineStyle:{color:sentColors[0]}},
        {name:'负面',type:'line',smooth:true,data:dailyTrendData.map(function(d){return d.negative;}),itemStyle:{color:sentColors[2]},lineStyle:{color:sentColors[2]}}
      ]
    });
  }
})();

/* ── Radar ── */
(function(){
  var c = mkChart('chart-radar'); if(!c)return;
  if(!radarData||!radarData.length)return;
  var maxVal = Math.max.apply(null,radarData.map(function(d){return d.value;}))||1;
  c.setOption({
    tooltip:{},
    radar:{indicator:radarData.map(function(d){return{name:d.name,max:Math.ceil(maxVal*1.2)};}),radius:'65%',axisName:{fontSize:10}},
    series:[{type:'radar',data:[{value:radarData.map(function(d){return d.value;}),name:'情感指数',itemStyle:{color:softColors[0]},areaStyle:{opacity:0.25}}]}]
  });
})();

/* ── Tags Bar ── */
(function(){
  var c = mkChart('chart-tags'); if(!c)return;
  var names=topTagsData.map(function(d){return d.name;});
  var vals=topTagsData.map(function(d){return d.value;});
  c.setOption({
    tooltip:{trigger:'axis'},
    grid:{top:10,bottom:10,left:'3%',right:'8%',containLabel:true},
    xAxis:{type:'value',axisLabel:{fontSize:10}},
    yAxis:{type:'category',data:names,axisLabel:{fontSize:11}},
    series:[{
      type:'bar',data:vals.map(function(v,i){return{value:v,itemStyle:{color:softColors[i%softColors.length],borderRadius:[0,8,8,0]}};}),
      label:{show:true,position:'right',fontSize:10}
    }]
  });
})();

/* ── Topic Bubble / Risk Matrix ── */
(function(){
  var c = mkChart('chart-risk'); if(!c)return;
  if(!topicBubbleData||!topicBubbleData.length){
    c.setOption({graphic:[{type:'text',left:'center',top:'middle',style:{text:'暂无话题数据',fontSize:14,fill:'#94a3b8'}}]});
    return;
  }
  // topicBubbleData: [{name, value:[negRate, count]}]
  var scatterData = topicBubbleData.map(function(d){
    var negRate = d.value[0];
    var count   = d.value[1];
    var color = negRate>40 ? sentColors[2] : (negRate>20 ? '#f59e0b' : sentColors[0]);
    return {name:d.name, value:[negRate, count], itemStyle:{color:color, opacity:0.8}};
  });
  c.setOption({
    tooltip:{formatter:function(p){return p.data.name+'<br/>负面率: '+p.data.value[0].toFixed(1)+'%<br/>文章数: '+p.data.value[1];}},
    grid:{top:30,bottom:60,left:'3%',right:'5%',containLabel:true},
    xAxis:{type:'value',name:'负面率(%)',nameLocation:'end',nameTextStyle:{fontSize:11},axisLabel:{formatter:'{value}%',fontSize:10},min:0},
    yAxis:{type:'value',name:'文章数',nameLocation:'end',nameTextStyle:{fontSize:11},axisLabel:{fontSize:10}},
    series:[{
      type:'scatter',
      symbolSize:function(d){return Math.max(24,Math.min(64,d[1]*4));},
      data:scatterData,
      label:{show:true,formatter:function(p){return p.data.name;},position:'top',fontSize:10},
      markLine:{silent:true,lineStyle:{type:'dashed',color:'#94a3b8'},data:[
        {xAxis:20,name:'注意',label:{formatter:'注意 20%',position:'end',fontSize:10,color:'#f59e0b'}},
        {xAxis:40,name:'警戒',label:{formatter:'警戒 40%',position:'end',fontSize:10,color:sentColors[2]}}
      ]}
    }]
  });
})();

/* ── Comment Sentiment Pie ── */
(function(){
  if(!commentSentData)return;
  var c = mkChart('chart-comment-sent'); if(!c)return;
  var data=[
    {name:'正面',value:commentSentData.positive,itemStyle:{color:sentColors[0]}},
    {name:'中性',value:commentSentData.neutral,itemStyle:{color:sentColors[1]}},
    {name:'负面',value:commentSentData.negative,itemStyle:{color:sentColors[2]}}
  ];
  c.setOption({
    tooltip:{trigger:'item',formatter:'{b}: {c} ({d}%)'},
    legend:{orient:'horizontal',bottom:0,textStyle:{fontSize:11}},
    series:[{
      type:'pie',radius:['42%','70%'],
      itemStyle:{borderRadius:8,borderColor:'#fff',borderWidth:2},
      label:{formatter:function(p){return p.name+'\n'+p.percent.toFixed(1)+'%';},fontSize:11},
      data:data
    }]
  });
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
  c.setOption({
    tooltip:{trigger:'axis'},
    grid:{top:10,bottom:10,left:'3%',right:'8%',containLabel:true},
    xAxis:{type:'value',axisLabel:{fontSize:10}},
    yAxis:{type:'category',data:names,axisLabel:{fontSize:11}},
    series:[{
      type:'bar',data:vals.map(function(v,i){return{value:v,itemStyle:{color:softColors[i%softColors.length],borderRadius:[0,8,8,0]}};}),
      label:{show:true,position:'right',fontSize:10}
    }]
  });
})();

/* ── Comment Trend ── */
(function(){
  if(!commentTrendData||!commentTrendData.length)return;
  var c = mkChart('chart-comment-trend'); if(!c)return;
  var dates=commentTrendData.map(function(d){return d.date;});
  c.setOption({
    tooltip:{trigger:'axis'},
    legend:{bottom:0,textStyle:{fontSize:10}},
    grid:{top:10,bottom:50,left:'3%',right:'3%',containLabel:true},
    xAxis:{type:'category',data:dates,axisLabel:{fontSize:9,rotate:30}},
    yAxis:{type:'value',axisLabel:{fontSize:10}},
    series:[
      {name:'正面',type:'line',smooth:true,stack:'ct',areaStyle:{opacity:0.35},data:commentTrendData.map(function(d){return d.positive;}),itemStyle:{color:sentColors[0]},lineStyle:{color:sentColors[0]}},
      {name:'中性',type:'line',smooth:true,stack:'ct',areaStyle:{opacity:0.35},data:commentTrendData.map(function(d){return d.neutral;}),itemStyle:{color:sentColors[1]},lineStyle:{color:sentColors[1]}},
      {name:'负面',type:'line',smooth:true,stack:'ct',areaStyle:{opacity:0.35},data:commentTrendData.map(function(d){return d.negative;}),itemStyle:{color:sentColors[2]},lineStyle:{color:sentColors[2]}}
    ]
  });
})();

/* ── Comment Topic Opinion Cards ── */
(function(){
  var el=document.getElementById('comment-topic-cards'); if(!el)return;
  if(!commentTopicData||!commentTopicData.length)return;
  function escHtml(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
  commentTopicData.forEach(function(t){
    var card=document.createElement('div'); card.className='topic-card';
    var sentBar='<div style="display:flex;height:6px;border-radius:3px;overflow:hidden;margin:8px 0;">';
    var total=t.sentiment.positive+t.sentiment.neutral+t.sentiment.negative;
    if(total>0){
      sentBar+='<div style="width:'+((t.sentiment.positive/total*100).toFixed(1))+'%;background:'+sentColors[0]+';"></div>';
      sentBar+='<div style="width:'+((t.sentiment.neutral/total*100).toFixed(1))+'%;background:'+sentColors[1]+';"></div>';
      sentBar+='<div style="width:'+((t.sentiment.negative/total*100).toFixed(1))+'%;background:'+sentColors[2]+';"></div>';
    }
    sentBar+='</div>';
    var sentLegend='<div style="display:flex;gap:12px;font-size:11px;color:var(--text-secondary);margin-bottom:8px;">';
    if(total>0){
      sentLegend+='<span style="display:flex;align-items:center;gap:3px;"><span style="width:7px;height:7px;border-radius:50%;background:'+sentColors[0]+';display:inline-block;"></span>正面 '+t.sentiment.positive+'</span>';
      sentLegend+='<span style="display:flex;align-items:center;gap:3px;"><span style="width:7px;height:7px;border-radius:50%;background:'+sentColors[1]+';display:inline-block;"></span>中性 '+t.sentiment.neutral+'</span>';
      sentLegend+='<span style="display:flex;align-items:center;gap:3px;"><span style="width:7px;height:7px;border-radius:50%;background:'+sentColors[2]+';display:inline-block;"></span>负面 '+t.sentiment.negative+'</span>';
    }
    sentLegend+='</div>';
    var opinions='';
    if(t.keyOpinions&&t.keyOpinions.length){
      opinions='<div style="margin:8px 0 0;padding:10px 14px;background:rgba(0,0,0,0.025);border-radius:8px;">';
      opinions+='<div style="font-size:11px;font-weight:600;color:var(--text-secondary);margin-bottom:6px;">核心观点</div>';
      opinions+='<ul style="margin:0;padding-left:16px;font-size:12px;color:var(--text-primary);line-height:1.8;">';
      t.keyOpinions.forEach(function(o){opinions+='<li style="margin:2px 0;">'+escHtml(o)+'</li>';});
      opinions+='</ul></div>';
    }
    card.innerHTML='<div style="font-weight:600;font-size:14px;color:var(--text-primary);display:flex;align-items:center;gap:8px;">'+escHtml(t.topic)+
      '<span style="font-size:11px;background:var(--primary);color:#fff;border-radius:12px;padding:1px 9px;font-weight:500;">'+t.commentCount+' 条评论</span></div>'+
      sentBar+sentLegend+opinions;
    el.appendChild(card);
  });
})();

/* ── Hot Comments Table ── */
(function(){
  var tbody=document.getElementById('hot-comments-tbody'); if(!tbody)return;
  if(!hotCommentsData||!hotCommentsData.length)return;
  function escHtml(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');}
  hotCommentsData.forEach(function(c,i){
    var cls=c.sentiment==='positive'?'badge-pos':(c.sentiment==='negative'?'badge-neg':'badge-neu');
    var sentLabel=c.sentiment==='positive'?'正面':(c.sentiment==='negative'?'负面':'中性');
    var tr=document.createElement('tr');
    tr.innerHTML='<td style="color:var(--text-secondary);font-size:12px;">'+(i+1)+'</td>'+
      '<td style="max-width:400px;word-break:break-all;font-size:13px;">'+escHtml(c.content)+'</td>'+
      '<td style="font-size:12px;">'+escHtml(c.nickname)+'</td>'+
      '<td style="font-size:12px;">'+escHtml(c.platform)+'</td>'+
      '<td style="font-size:12px;font-weight:600;">'+c.likeCount+'</td>'+
      '<td><span class="badge '+cls+'">'+sentLabel+'</span></td>';
    tbody.appendChild(tr);
  });
})();

/* ── Tag Cloud (spiral placement, cloud-shaped) ── */
(function(){
  var el = document.getElementById('tag-cloud'); if(!el)return;
  if(!tagCloudData||!tagCloudData.length)return;
  el.style.position='relative';
  el.style.height='280px';
  el.style.overflow='hidden';
  el.style.display='block';
  var W=el.clientWidth||700, H=280;
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
  // Hidden element for measuring text width
  var measurer=document.createElement('span');
  measurer.style.position='absolute';measurer.style.visibility='hidden';measurer.style.whiteSpace='nowrap';
  measurer.style.fontFamily='PingFang SC,Hiragino Sans GB,Microsoft YaHei,sans-serif';
  document.body.appendChild(measurer);
  sorted.forEach(function(d,i){
    var ratio = maxVal>minVal ? (d.value-minVal)/(maxVal-minVal) : 0.5;
    var fontSize = Math.round(14 + ratio*26);
    var fontWeight = ratio>0.6?'700':'500';
    var opacity = 0.6 + ratio*0.4;
    var color = tagColors[i%tagColors.length];
    // Measure real text width
    measurer.style.fontSize=fontSize+'px';
    measurer.style.fontWeight=fontWeight;
    measurer.textContent=d.name;
    var tw = measurer.offsetWidth+4;
    var th = fontSize+8;
    // Spiral search from center
    var angle=0, radius=0;
    var px, py, found=false;
    for(var tries=0;tries<800;tries++){
      px=W/2+radius*Math.cos(angle)-tw/2;
      py=H/2+radius*Math.sin(angle)*0.65-th/2;
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
    span.style.transition='transform 0.15s';
    span.onmouseover=function(){this.style.transform='scale(1.12)';};
    span.onmouseout=function(){this.style.transform='scale(1)';};
    el.appendChild(span);
  });
  document.body.removeChild(measurer);
})();

/* ── Top Articles Table ── */
(function(){
  var tbody = document.getElementById('top-articles-tbody'); if(!tbody)return;
  function escHtml(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');}
  topArticlesData.forEach(function(a,i){
    var sent = a.sentiment||'';
    var cls  = sent==='正面'?'badge-pos':(sent==='负面'?'badge-neg':'badge-neu');
    var score= typeof a.sentScore==='number'?a.sentScore:0;
    var pct  = Math.min(100, Math.round(score));
    var barColor= sent==='正面'?sentColors[0]:(sent==='负面'?sentColors[2]:sentColors[1]);
    var tr=document.createElement('tr');
    tr.innerHTML='<td style="color:var(--text-secondary);font-size:12px;">'+(i+1)+'</td>'+
      '<td style="max-width:420px;word-break:break-all;">'+escHtml(a.title)+'</td>'+
      '<td style="font-size:12px;">'+escHtml(a.platform||'')+'</td>'+
      '<td><span class="badge '+cls+'">'+escHtml(sent)+'</span></td>'+
      '<td><div class="score-bar-wrap"><div class="score-bar-bg"><div class="score-bar-fill" style="width:'+pct+'%;background:'+barColor+';"></div></div><span style="font-size:11px;color:var(--text-secondary);">'+score.toFixed(2)+'</span></div></td>';
    tbody.appendChild(tr);
  });
})();

/* ── Animated Counters ── */
(function(){
  var els = document.querySelectorAll('.kpi-value[data-target]');
  var duration = 800;
  els.forEach(function(el){
    var target = parseInt(el.getAttribute('data-target'),10)||0;
    var start = null;
    function step(ts){
      if(!start) start=ts;
      var prog = Math.min((ts-start)/duration,1);
      var ease = 1-Math.pow(1-prog,3);
      el.textContent = Math.round(ease*target);
      if(prog<1) requestAnimationFrame(step);
      else el.textContent = target;
    }
    requestAnimationFrame(step);
  });
})();

/* ── Resize ── */
window.addEventListener('resize',function(){allCharts.forEach(function(c){try{c.resize();}catch(e){}});});

})();
</script>
</body>
</html>`
