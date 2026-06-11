package analytics

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	analyticsService "velox-server/internal/services/analytics"
	"velox-server/internal/store"
)

type DashboardHandler struct {
	dataDir      string
	analyticsSvc analyticsService.AnalyticsService
}

func NewDashboardHandler(dataDir string, svc analyticsService.AnalyticsService) *DashboardHandler {
	return &DashboardHandler{
		dataDir:      dataDir,
		analyticsSvc: svc,
	}
}

func (h *DashboardHandler) DashboardSummary(c *gin.Context) {
	jobCounts := make(map[string]int64)
	if h.analyticsSvc != nil {
		if counts, err := h.analyticsSvc.GetJobCounts(c.Request.Context()); err == nil {
			jobCounts = counts
		}
	}

	var analyticsSummary map[string]any
	if h.analyticsSvc != nil {
		if data, err := h.analyticsSvc.GetAnalyticsTotals("30"); err == nil {
			analyticsSummary = data
		}
	}

	var realtimeData map[string]any
	if h.analyticsSvc != nil {
		if data, err := h.analyticsSvc.GetAnalyticsCache("realtime"); err == nil {
			realtimeData = data
		}
	}

	response := gin.H{
		"ok": true,
		"jobs": gin.H{
			"total":      jobCounts["total"],
			"pending":    jobCounts["pending"],
			"processing": jobCounts["processing"],
			"completed":  jobCounts["completed"],
			"error":      jobCounts["error"],
		},
		"analytics": gin.H{
			"total_views":   0,
			"total_revenue": 0.0,
			"total_videos":  0,
			"channels":      0,
		},
		"realtime": gin.H{
			"views_24h": 0,
			"views_1h":  0,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if analyticsSummary != nil {
		if views, ok := analyticsSummary["views"]; ok {
			response["analytics"].(gin.H)["total_views"] = views
		}
		if revenue, ok := analyticsSummary["revenue"]; ok {
			response["analytics"].(gin.H)["total_revenue"] = revenue
		}
		if videos, ok := analyticsSummary["videos"]; ok {
			response["analytics"].(gin.H)["total_videos"] = videos
		}
		if channels, ok := analyticsSummary["channels"]; ok {
			response["analytics"].(gin.H)["channels"] = channels
		}
	}

	if realtimeData != nil {
		if views24h, ok := realtimeData["total_views_24h"]; ok {
			response["realtime"].(gin.H)["views_24h"] = views24h
		}
		if views1h, ok := realtimeData["total_views_1h"]; ok {
			response["realtime"].(gin.H)["views_1h"] = views1h
		}
	}

	c.JSON(http.StatusOK, response)
}

func (h *DashboardHandler) DashboardFinance(c *gin.Context) {
	period := c.DefaultQuery("period", "30")

	var data map[string]any
	var err error

	if h.analyticsSvc != nil {
		data, err = h.analyticsSvc.GetAnalyticsCache(period)
		if err != nil {
			data, err = h.analyticsSvc.GetAnalyticsCache("30")
		}
	}

	if data == nil {
		data = make(map[string]any)
	}

	totals, _ := data["totals"].(map[string]any)
	channels, _ := data["channels"].([]any)
	dailyStats, _ := data["daily_stats"].([]any)

	totalRevenue := 0.0
	totalViews := 0
	if totals != nil {
		if r, ok := totals["revenue"].(float64); ok {
			totalRevenue = r
		}
		if v, ok := totals["views"].(float64); ok {
			totalViews = int(v)
		}
	}

	type ChannelRevenue struct {
		Name    string  `json:"name"`
		Revenue float64 `json:"revenue"`
		Views   int     `json:"views"`
	}

	channelRevenues := make([]ChannelRevenue, 0)
	if channels != nil {
		for _, ch := range channels {
			chMap, ok := ch.(map[string]any)
			if !ok {
				continue
			}
			channelRevenues = append(channelRevenues, ChannelRevenue{
				Name:    asStr(chMap["name"]),
				Revenue: asFloatFromAny(chMap["revenue"]),
				Views:   int(asFloatFromAny(chMap["views"])),
			})
		}
	}

	sort.Slice(channelRevenues, func(i, j int) bool {
		return channelRevenues[i].Revenue > channelRevenues[j].Revenue
	})

	if len(channelRevenues) > 10 {
		channelRevenues = channelRevenues[:10]
	}

	chartData := make([]gin.H, 0)
	if dailyStats != nil {
		for _, ds := range dailyStats {
			dsMap, ok := ds.(map[string]any)
			if !ok {
				continue
			}
			chartData = append(chartData, gin.H{
				"date":    asStr(dsMap["date"]),
				"views":   int(asFloatFromAny(dsMap["views"])),
				"revenue": asFloatFromAny(dsMap["revenue"]),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"summary": gin.H{
			"total_revenue": totalRevenue,
			"total_views":   totalViews,
			"period":        period,
			"channels":      len(channelRevenues),
		},
		"top_channels": channelRevenues,
		"chart_data":   chartData,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardPerformance(c *gin.Context) {
	limit := 10
	if l := c.Query("limit"); l != "" {
		if parsed := parseIntDef(l, 10); parsed > 0 {
			limit = parsed
		}
	}

	var topVideos []store.VideoStat
	if h.analyticsSvc != nil {
		if videos, err := h.analyticsSvc.GetTopVideos(limit, "30d"); err == nil {
			topVideos = videos
		}
	}

	var topChannels []store.ChannelStat
	if h.analyticsSvc != nil {
		if channels, err := h.analyticsSvc.GetTopChannels(limit, "30d"); err == nil {
			topChannels = channels
		}
	}

	var realtimeViews map[string]any
	if h.analyticsSvc != nil {
		if data, err := h.analyticsSvc.GetAnalyticsCache("realtime"); err == nil {
			realtimeViews = data
		}
	}

	avgViews := 0
	if len(topVideos) > 0 {
		total := 0
		for _, v := range topVideos {
			total += v.Views30d
		}
		avgViews = total / len(topVideos)
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"top_videos": gin.H{
			"items": topVideos,
			"count": len(topVideos),
		},
		"top_channels": gin.H{
			"items": topChannels,
			"count": len(topChannels),
		},
		"metrics": gin.H{
			"avg_views_per_video": avgViews,
			"total_videos":        len(topVideos),
		},
		"realtime":  realtimeViews,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardRealtime(c *gin.Context) {
	var data map[string]any
	if h.analyticsSvc != nil {
		data, _ = h.analyticsSvc.GetAnalyticsCache("realtime")
	}

	if data == nil {
		data = make(map[string]any)
	}

	if _, ok := data["chart_data"]; !ok {
		data["chart_data"] = generateEstimatedChartData()
	}

	if _, ok := data["top_videos"]; !ok {
		data["top_videos"] = []any{}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardChannels(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed := parseIntDef(l, 20); parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	period := c.DefaultQuery("period", "30d")

	var channels []store.ChannelStat
	if h.analyticsSvc != nil {
		channels, _ = h.analyticsSvc.GetTopChannels(limit, period)
	}

	authErrors := 0
	for _, ch := range channels {
		if ch.AuthError {
			authErrors++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"channels": gin.H{
			"items":       channels,
			"count":       len(channels),
			"auth_errors": authErrors,
		},
		"period":    period,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardGroups(c *gin.Context) {
	var data map[string]any
	if h.analyticsSvc != nil {
		data, _ = h.analyticsSvc.GetAnalyticsCache("30")
	}
	if data == nil {
		data = make(map[string]any)
	}

	groupMap := make(map[string]*struct {
		Views   int
		Count   int
		Revenue float64
	})

	channels, _ := data["channels"].([]any)
	for _, ch := range channels {
		chMap, ok := ch.(map[string]any)
		if !ok {
			continue
		}

		name := asStr(chMap["name"])
		group := "Ungrouped"

		if idx := strings.LastIndex(name, " - "); idx > 0 {
			group = strings.TrimSpace(name[idx+3:])
		} else if idx := strings.LastIndex(name, " "); idx > 0 {
			group = strings.TrimSpace(name[idx+1:])
		}

		if _, exists := groupMap[group]; !exists {
			groupMap[group] = &struct {
				Views   int
				Count   int
				Revenue float64
			}{}
		}

		groupMap[group].Views += int(asFloatFromAny(chMap["views"]))
		groupMap[group].Count++
		groupMap[group].Revenue += asFloatFromAny(chMap["revenue"])
	}

	type GroupStat struct {
		GroupName        string  `json:"group_name"`
		TotalViews       int     `json:"total_views"`
		VideoCount       int     `json:"video_count"`
		AvgViewsPerVideo int     `json:"avg_views_per_video"`
		TotalRevenue     float64 `json:"total_revenue"`
		ChannelCount     int     `json:"channel_count"`
	}

	groups := make([]GroupStat, 0, len(groupMap))
	for name, g := range groupMap {
		avg := 0
		if g.Count > 0 {
			avg = g.Views / g.Count
		}
		groups = append(groups, GroupStat{
			GroupName:        name,
			TotalViews:       g.Views,
			VideoCount:       g.Count,
			AvgViewsPerVideo: avg,
			TotalRevenue:     g.Revenue,
			ChannelCount:     g.Count,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].TotalViews > groups[j].TotalViews
	})

	limit := 10
	if l := c.Query("limit"); l != "" {
		if parsed := parseIntDef(l, 10); parsed > 0 {
			limit = parsed
		}
	}
	if len(groups) > limit {
		groups = groups[:limit]
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"groups":    groups,
		"count":     len(groups),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardTimeline(c *gin.Context) {
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed := parseIntDef(d, 30); parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	var dailyStats []store.DailyStat
	if h.analyticsSvc != nil {
		dailyStats, _ = h.analyticsSvc.GetDailyStats(days)
	}

	cumulative := 0
	chartData := make([]gin.H, 0, len(dailyStats))
	for _, ds := range dailyStats {
		cumulative += ds.Views
		chartData = append(chartData, gin.H{
			"date":       ds.Date,
			"views":      ds.Views,
			"revenue":    ds.Revenue,
			"cumulative": cumulative,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":          true,
		"chart_data":  chartData,
		"period_days": days,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardComparison(c *gin.Context) {
	period1 := c.DefaultQuery("period1", "7")
	period2 := c.DefaultQuery("period2", "30")

	var data1, data2 map[string]any
	if h.analyticsSvc != nil {
		data1, _ = h.analyticsSvc.GetAnalyticsTotals(period1)
		data2, _ = h.analyticsSvc.GetAnalyticsTotals(period2)
	}

	extractValue := func(data map[string]any, key string) int {
		if data == nil {
			return 0
		}
		if v, ok := data[key]; ok {
			switch n := v.(type) {
			case int:
				return n
			case int64:
				return int(n)
			case float64:
				return int(n)
			}
		}
		return 0
	}

	views1 := extractValue(data1, "views")
	views2 := extractValue(data2, "views")
	revenue1 := extractFloat(data1, "revenue")
	revenue2 := extractFloat(data2, "revenue")

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"period1": gin.H{
			"days":    period1,
			"views":   views1,
			"revenue": revenue1,
		},
		"period2": gin.H{
			"days":    period2,
			"views":   views2,
			"revenue": revenue2,
		},
		"comparison": gin.H{
			"views_diff":   views1 - views2,
			"revenue_diff": revenue1 - revenue2,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) DashboardExport(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	period := c.DefaultQuery("period", "30")

	var data map[string]any
	if h.analyticsSvc != nil {
		data, _ = h.analyticsSvc.GetAnalyticsCache(period)
	}

	if data == nil {
		data = make(map[string]any)
	}

	switch format {
	case "csv":
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename=analytics_export.csv")

		dailyStats, _ := data["daily_stats"].([]any)
		csv := "date,views,revenue\n"
		for _, ds := range dailyStats {
			dsMap, ok := ds.(map[string]any)
			if !ok {
				continue
			}
			csv += asStr(dsMap["date"]) + "," +
				asStr(dsMap["views"]) + "," +
				asStr(dsMap["revenue"]) + "\n"
		}
		c.String(http.StatusOK, csv)

	default:
		c.Header("Content-Disposition", "attachment; filename=analytics_export.json")
		c.JSON(http.StatusOK, data)
	}
}

func (h *DashboardHandler) DashboardHealth(c *gin.Context) {
	serviceStatus := "ok"
	if h.analyticsSvc == nil {
		serviceStatus = "unavailable"
	}

	dataStatus := "ok"
	if h.dataDir == "" {
		dataStatus = "not_configured"
	} else {
		analyticsPath := filepath.Join(h.dataDir, "analytics")
		if _, err := os.Stat(analyticsPath); os.IsNotExist(err) {
			dataStatus = "missing_analytics_dir"
		}
	}

	lastUpdate := ""
	if h.analyticsSvc != nil {
		if data, err := h.analyticsSvc.GetAnalyticsCache("30"); err == nil {
			if ts, ok := data["cached_at"].(float64); ok {
				lastUpdate = time.Unix(int64(ts), 0).UTC().Format(time.RFC3339)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"health": gin.H{
			"service":     serviceStatus,
			"data_dir":    dataStatus,
			"last_update": lastUpdate,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func RegisterDashboardRoutes(r *gin.Engine, dataDir string, svc analyticsService.AnalyticsService) {
	handler := NewDashboardHandler(dataDir, svc)

	dashboard := r.Group("/api/v1/dashboard")
	{
		dashboard.GET("/summary", handler.DashboardSummary)
		dashboard.GET("/finance", handler.DashboardFinance)
		dashboard.GET("/performance", handler.DashboardPerformance)
		dashboard.GET("/realtime", handler.DashboardRealtime)
		dashboard.GET("/channels", handler.DashboardChannels)
		dashboard.GET("/groups", handler.DashboardGroups)
		dashboard.GET("/timeline", handler.DashboardTimeline)
		dashboard.GET("/comparison", handler.DashboardComparison)
		dashboard.GET("/export", handler.DashboardExport)
		dashboard.GET("/health", handler.DashboardHealth)
	}

	log.Printf("✅ Dashboard routes registered at /api/v1/dashboard/*")
}

func generateEstimatedChartData() []gin.H {
	chart := make([]gin.H, 24)
	now := time.Now()
	for i := 0; i < 24; i++ {
		t := now.Add(-time.Duration(i) * time.Hour)
		chart[i] = gin.H{
			"time":  t.Format("2006-01-02 15:04"),
			"views": 0,
			"type":  "estimated",
		}
	}
	return chart
}
