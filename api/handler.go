package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/viktsys/b3ingest/database"
	"github.com/viktsys/b3ingest/models"
)

type QueryParams struct {
	Ticker     string `form:"ticker" binding:"required"`
	DataInicio string `form:"data_inicio"`
}

func GetTradeStats(c *gin.Context) {
	var params QueryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var startDate time.Time
	var err error

	if params.DataInicio != "" {
		startDate, err = time.Parse("2006-01-02", params.DataInicio)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD"})
			return
		}
	} else {
		// Default to 7 days before yesterday
		startDate = time.Now().AddDate(0, 0, -8)
	}

	stats, err := calculateStats(params.Ticker, startDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func calculateStats(ticker string, startDate time.Time) (*models.TradeStats, error) {
	db := database.DB

	// Estrutura para capturar o resultado da query combinada
	type StatsResult struct {
		MaxPrice       float64
		MaxDailyVolume uint64
	}

	var result StatsResult

	// Query combinada usando subqueries para otimizar performance
	err := db.Raw(`
		SELECT 
			COALESCE((SELECT MAX(preco_negocio) FROM trades 
				WHERE codigo_instrumento = ? AND data_negocio >= ?), 0) as max_price,
			COALESCE((SELECT MAX(volume_total) FROM daily_aggregates 
				WHERE codigo_instrumento = ? AND data_negocio >= ?), 0) as max_daily_volume
	`, ticker, startDate, ticker, startDate).Scan(&result).Error

	if err != nil {
		return nil, err
	}

	return &models.TradeStats{
		Ticker:         ticker,
		MaxRangeValue:  result.MaxPrice,
		MaxDailyVolume: result.MaxDailyVolume,
	}, nil
}

func SetupRoutes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/trades/stats", GetTradeStats)

	return r
}
