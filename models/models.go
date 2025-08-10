package models

import (
	"time"
)

// Trade representa uma negociação da B3
type Trade struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	DataNegocio         time.Time `gorm:"index:idx_data_ticker" json:"data_negocio"`
	CodigoInstrumento   string    `gorm:"index:idx_data_ticker;size:20" json:"codigo_instrumento"`
	PrecoNegocio        float64   `json:"preco_negocio"`
	QuantidadeNegociada uint64    `json:"quantidade_negociada"`
	HoraFechamento      string    `gorm:"size:10" json:"hora_fechamento"`
	CreatedAt           time.Time `json:"created_at"`
}

// DailyAggregate armazena dados agregados por dia e ticker
type DailyAggregate struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	DataNegocio       time.Time `gorm:"index:idx_daily_ticker;uniqueIndex:uidx_data_ticker" json:"data_negocio"`
	CodigoInstrumento string    `gorm:"index:idx_daily_ticker;size:20;uniqueIndex:uidx_data_ticker" json:"codigo_instrumento"`
	VolumeTotal       uint64    `json:"volume_total"`
	PrecoMaximo       float64   `json:"preco_maximo"`
	CreatedAt         time.Time `json:"created_at"`
}

// TradeStats representa as estatísticas agregadas retornadas pela API
type TradeStats struct {
	Ticker         string  `json:"ticker"`
	MaxRangeValue  float64 `json:"max_range_value"`
	MaxDailyVolume uint64  `json:"max_daily_volume"`
}
