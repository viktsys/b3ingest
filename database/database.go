package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/viktsys/b3ingest/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "password")
	dbName := getEnv("DB_NAME", "b3ingest")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=America/Sao_Paulo",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Configure connection pool for optimal read performance
	sqlDB.SetMaxOpenConns(25)                  // Reduzido para consultas de leitura
	sqlDB.SetMaxIdleConns(25)                  // Match max open conns
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // Increased lifetime for stability
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Longer idle timeout for read queries

	// Auto migrate the schema
	if err := DB.AutoMigrate(&models.Trade{}, &models.DailyAggregate{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Apply database optimizations
	if err := OptimizeIndexes(DB); err != nil {
		log.Printf("Warning: Failed to optimize indexes: %v", err)
	}

	log.Println("Database connected and migrated successfully")
	return nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
