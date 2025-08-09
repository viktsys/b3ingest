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

	var maxPrice float64
	err := db.Model(&models.Trade{}).
		Select("COALESCE(MAX(preco_negocio), 0)").
		Where("codigo_instrumento = ? AND data_negocio >= ?", ticker, startDate).
		Scan(&maxPrice).Error
	if err != nil {
		return nil, err
	}

	var maxDailyVolume uint64
	err = db.Model(&models.DailyAggregate{}).
		Select("COALESCE(MAX(volume_total), 0)").
		Where("codigo_instrumento = ? AND data_negocio >= ?", ticker, startDate).
		Scan(&maxDailyVolume).Error
	if err != nil {
		return nil, err
	}

	return &models.TradeStats{
		Ticker:         ticker,
		MaxRangeValue:  maxPrice,
		MaxDailyVolume: maxDailyVolume,
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
