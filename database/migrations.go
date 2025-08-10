package database

import (
	"fmt"

	"gorm.io/gorm"
)

// OptimizeIndexes cria índices otimizados para melhorar performance de consultas
func OptimizeIndexes(db *gorm.DB) error {
	// Drop índices antigos se existirem
	if err := db.Exec("DROP INDEX IF EXISTS idx_data_ticker").Error; err != nil {
		fmt.Printf("Warning: Could not drop old index idx_data_ticker: %v\n", err)
	}

	if err := db.Exec("DROP INDEX IF EXISTS idx_daily_ticker").Error; err != nil {
		fmt.Printf("Warning: Could not drop old index idx_daily_ticker: %v\n", err)
	}

	// Criar índices otimizados para trades
	// Índice composto otimizado: ticker primeiro, depois data (mais seletivo)
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_trades_ticker_date_optimized 
		ON trades (codigo_instrumento, data_negocio DESC)
	`).Error; err != nil {
		return fmt.Errorf("failed to create optimized trades index: %w", err)
	}

	// Índice para preços (usado em MAX queries)
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_trades_ticker_price 
		ON trades (codigo_instrumento, preco_negocio DESC) 
		WHERE preco_negocio IS NOT NULL
	`).Error; err != nil {
		return fmt.Errorf("failed to create trades price index: %w", err)
	}

	// Criar índices otimizados para daily_aggregates
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_daily_ticker_date_optimized 
		ON daily_aggregates (codigo_instrumento, data_negocio DESC)
	`).Error; err != nil {
		return fmt.Errorf("failed to create optimized daily aggregates index: %w", err)
	}

	// Índice para volumes (usado em MAX queries)
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_daily_ticker_volume 
		ON daily_aggregates (codigo_instrumento, volume_total DESC) 
		WHERE volume_total IS NOT NULL
	`).Error; err != nil {
		return fmt.Errorf("failed to create daily aggregates volume index: %w", err)
	}

	fmt.Println("Database indexes optimized successfully")
	return nil
}
