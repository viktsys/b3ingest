package ingest

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/viktsys/b3ingest/database"
	"github.com/viktsys/b3ingest/models"
	"gorm.io/gorm"
)

const (
	// Default values - can be overridden by environment variables
	DefaultBatchSize   = 2000
	DefaultWorkerCount = 128
	DefaultFileWorkers = 32
	DefaultBufferSize  = 256
)

// getEnvInt returns environment variable as int or default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// Configuration for parallel processing
func getBatchSize() int {
	return getEnvInt("BATCH_SIZE", DefaultBatchSize)
}

func getWorkerCount() int {
	return getEnvInt("WORKER_COUNT", DefaultWorkerCount)
}

func getFileWorkers() int {
	return getEnvInt("FILE_WORKERS", DefaultFileWorkers)
}

func getBufferSize() int {
	return getEnvInt("BUFFER_SIZE", DefaultBufferSize)
}

type TradeRecord struct {
	DataNegocio         string
	CodigoInstrumento   string
	PrecoNegocio        string
	QuantidadeNegociada string
	HoraFechamento      string
}

type Processor struct {
	db             *gorm.DB
	processedRows  int64
	processedFiles int64
}

func NewProcessor() *Processor {
	return &Processor{
		db: database.DB,
	}
}

// ProcessDirectoryOptimized provides enhanced parallel processing for large datasets
func (p *Processor) ProcessDirectoryOptimized(dataDir string) error {
	startTime := time.Now()

	files, err := filepath.Glob(filepath.Join(dataDir, "*.csv"))
	if err != nil {
		return fmt.Errorf("failed to find CSV files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no CSV files found in directory: %s", dataDir)
	}

	log.Printf("Found %d CSV files to process with optimized parallel processing", len(files))

	fileWorkers := getFileWorkers()
	workerCount := getWorkerCount()
	log.Printf("Using %d file workers and %d batch workers per file", fileWorkers, workerCount)

	// Create a semaphore to limit concurrent file processing
	semaphore := make(chan struct{}, fileWorkers)
	var wg sync.WaitGroup
	errorChan := make(chan error, len(files))

	// Process all files concurrently
	for _, file := range files {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			fileStart := time.Now()
			log.Printf("Processing file: %s", filename)
			if err := p.ProcessFile(filename); err != nil {
				log.Printf("Error processing file %s: %v", filename, err)
				errorChan <- err
				return
			}

			atomic.AddInt64(&p.processedFiles, 1)
			duration := time.Since(fileStart)
			log.Printf("Successfully processed file: %s (took %v)", filename, duration)
		}(file)
	}

	// Wait for all processing to complete
	wg.Wait()
	close(errorChan)

	// Collect any errors
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	processingDuration := time.Since(startTime)

	if len(errors) > 0 {
		log.Printf("Encountered %d errors during processing, but continuing with aggregation", len(errors))
	}

	log.Printf("File processing completed in %v. Processed %d files, %d total rows",
		processingDuration, atomic.LoadInt64(&p.processedFiles), atomic.LoadInt64(&p.processedRows))

	log.Println("Starting parallel aggregation of daily data...")
	aggStart := time.Now()
	if err := p.AggregateDaily(); err != nil {
		return fmt.Errorf("failed to aggregate daily data: %w", err)
	}
	aggDuration := time.Since(aggStart)
	log.Printf("Daily aggregation completed successfully in %v", aggDuration)

	totalDuration := time.Since(startTime)
	log.Printf("Total processing time: %v (avg: %v per file)",
		totalDuration, totalDuration/time.Duration(len(files)))

	return nil
}

func (p *Processor) ProcessDirectory(dataDir string) error {
	files, err := filepath.Glob(filepath.Join(dataDir, "*.csv"))
	if err != nil {
		return fmt.Errorf("failed to find CSV files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no CSV files found in directory: %s", dataDir)
	}

	log.Printf("Found %d CSV files to process", len(files))

	// Process files in parallel using goroutines
	fileChan := make(chan string, len(files))
	fileWorkers := getFileWorkers()
	errorChan := make(chan error, fileWorkers)

	// Send all files to channel
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// Start file processing workers
	var wg sync.WaitGroup
	for i := 0; i < fileWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for file := range fileChan {
				log.Printf("Worker %d processing file: %s", workerID, file)
				if err := p.ProcessFile(file); err != nil {
					log.Printf("Worker %d error processing file %s: %v", workerID, file, err)
					select {
					case errorChan <- err:
					default:
						// Channel full, continue processing other files
					}
					continue
				}
				log.Printf("Worker %d successfully processed file: %s", workerID, file)
			}
		}(i)
	}

	// Wait for all file processing to complete
	wg.Wait()
	close(errorChan)

	// Check if there were any errors
	var hasErrors bool
	for err := range errorChan {
		if err != nil {
			log.Printf("File processing error: %v", err)
			hasErrors = true
		}
	}

	if hasErrors {
		log.Println("Some files had processing errors, but continuing with aggregation...")
	}

	log.Println("Starting aggregation of daily data...")
	if err := p.AggregateDaily(); err != nil {
		return fmt.Errorf("failed to aggregate daily data: %w", err)
	}
	log.Println("Daily aggregation completed")

	return nil
}

func (p *Processor) ProcessFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create channels for worker communication with larger buffers
	bufferSize := getBufferSize()
	workerCount := getWorkerCount()
	recordChan := make(chan []TradeRecord, bufferSize)
	errorChan := make(chan error, workerCount)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go p.worker(ctx, recordChan, errorChan, &wg)
	}

	// Read and parse CSV in batches
	go func() {
		defer close(recordChan)

		reader := csv.NewReader(file)
		reader.Comma = ';'        // Assumindo que o CSV usa ponto e vírgula como separador
		reader.ReuseRecord = true // Optimize memory usage

		var batch []TradeRecord
		lineNum := 0
		batchCount := 0

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Error reading CSV line %d: %v", lineNum, err)
				continue
			}

			lineNum++

			// Skip header
			if lineNum == 1 {
				continue
			}

			// Parse record (baseado na estrutura do CSV)
			if len(record) < 9 {
				continue // Skip invalid records silently for performance
			}

			tradeRecord := TradeRecord{
				DataNegocio:         strings.TrimSpace(record[8]),
				CodigoInstrumento:   strings.TrimSpace(record[1]),
				PrecoNegocio:        strings.TrimSpace(record[3]),
				QuantidadeNegociada: strings.TrimSpace(record[4]),
				HoraFechamento:      strings.TrimSpace(record[5]),
			}

			batch = append(batch, tradeRecord)

			batchSize := getBatchSize()
			if len(batch) >= batchSize {
				// Create a copy of the batch to send to workers
				batchCopy := make([]TradeRecord, len(batch))
				copy(batchCopy, batch)

				select {
				case recordChan <- batchCopy:
					batchCount++
					batch = batch[:0] // Reset batch, keep capacity
				case <-ctx.Done():
					return
				}
			}
		}

		// Send remaining records
		if len(batch) > 0 {
			batchCopy := make([]TradeRecord, len(batch))
			copy(batchCopy, batch)

			select {
			case recordChan <- batchCopy:
				batchCount++
			case <-ctx.Done():
				return
			}
		}

		log.Printf("Sent %d batches for processing from file: %s", batchCount, filepath.Base(filename))
	}()

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(errorChan)
	}()

	// Check for errors
	for err := range errorChan {
		if err != nil {
			cancel()
			return fmt.Errorf("worker error: %w", err)
		}
	}

	return nil
}

func (p *Processor) worker(ctx context.Context, recordChan <-chan []TradeRecord, errorChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case batch, ok := <-recordChan:
			if !ok {
				return
			}
			if err := p.processBatch(batch); err != nil {
				errorChan <- err
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *Processor) processBatch(records []TradeRecord) error {
	if len(records) == 0 {
		return nil
	}

	trades := make([]models.Trade, 0, len(records))

	for _, record := range records {
		trade, err := p.parseTradeRecord(record)
		if err != nil {
			// Skip invalid records for performance - log only in debug mode
			continue
		}
		trades = append(trades, trade)
	}

	if len(trades) == 0 {
		return nil
	}

	// Track processed rows
	atomic.AddInt64(&p.processedRows, int64(len(trades)))

	// Use optimized bulk insert with larger batch size
	return p.db.Transaction(func(tx *gorm.DB) error {
		// Insert all trades in a single operation for better performance
		return tx.CreateInBatches(trades, len(trades)).Error
	})
}

func (p *Processor) parseTradeRecord(record TradeRecord) (models.Trade, error) {
	var trade models.Trade

	// Parse data
	dataNegocio, err := time.Parse("2006-01-02", record.DataNegocio)
	if err != nil {
		return trade, fmt.Errorf("invalid date format: %w", err)
	}

	// Parse price
	precoStr := strings.Replace(record.PrecoNegocio, ",", ".", -1)
	preco, err := strconv.ParseFloat(precoStr, 64)
	if err != nil {
		return trade, fmt.Errorf("invalid price format: %w", err)
	}

	// Parse quantity - primeiro remove vírgulas e converte para float, depois para uint64
	quantidadeStr := strings.Replace(record.QuantidadeNegociada, ",", ".", -1)
	quantidadeFloat, err := strconv.ParseFloat(quantidadeStr, 64)
	if err != nil {
		return trade, fmt.Errorf("invalid quantity format: %w", err)
	}
	quantidade := uint64(quantidadeFloat)

	trade.DataNegocio = dataNegocio
	trade.CodigoInstrumento = record.CodigoInstrumento
	trade.PrecoNegocio = preco
	trade.QuantidadeNegociada = quantidade
	trade.HoraFechamento = record.HoraFechamento
	trade.CreatedAt = time.Now()

	return trade, nil
}

func (p *Processor) AggregateDaily() error {
	// Clear existing aggregates in parallel with data preparation
	clearDone := make(chan error, 1)
	go func() {
		clearDone <- p.db.Exec("DELETE FROM daily_aggregates").Error
	}()

	// Wait for clear to complete
	if err := <-clearDone; err != nil {
		return fmt.Errorf("failed to clear daily aggregates: %w", err)
	}

	// Use optimized aggregation query with better performance
	query := `
		INSERT INTO daily_aggregates (data_negocio, codigo_instrumento, volume_total, preco_maximo, created_at)
		SELECT 
			data_negocio,
			codigo_instrumento,
			SUM(quantidade_negociada) as volume_total,
			MAX(preco_negocio) as preco_maximo,
			NOW() as created_at
		FROM trades
		GROUP BY data_negocio, codigo_instrumento
		ORDER BY data_negocio, codigo_instrumento
	`

	// Execute aggregation with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return p.db.WithContext(ctx).Exec(query).Error
}
