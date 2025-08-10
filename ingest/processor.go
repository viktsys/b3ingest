package ingest

import (
	"context"
	"database/sql"
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
	"unsafe"

	"github.com/viktsys/b3ingest/database"
	"github.com/viktsys/b3ingest/models"
	"gorm.io/gorm"
)

const (
	// Optimized default values for high-performance processing
	DefaultBatchSize   = 10000 // Increased batch size
	DefaultWorkerCount = 64    // Reduced to prevent connection pool exhaustion
	DefaultFileWorkers = 16    // Reduced for better resource management
	DefaultBufferSize  = 512   // Increased buffer

	// SQL statement constants
	InsertTradeSQL = `INSERT INTO trades 
		(data_negocio, codigo_instrumento, preco_negocio, quantidade_negociada, hora_fechamento, created_at) 
		VALUES `
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

// OptimizedTradeRecord uses byte slices for better memory performance
type OptimizedTradeRecord struct {
	DataNegocio         []byte
	CodigoInstrumento   []byte
	PrecoNegocio        []byte
	QuantidadeNegociada []byte
	HoraFechamento      []byte
}

type TradeRecord struct {
	DataNegocio         string
	CodigoInstrumento   string
	PrecoNegocio        string
	QuantidadeNegociada string
	HoraFechamento      string
}

type Processor struct {
	db                *gorm.DB
	rawDB             *sql.DB
	processedRows     int64
	processedFiles    int64
	insertStmt        *sql.Stmt
	dateParseCache    sync.Map
	stringBuilderPool sync.Pool
}

func NewProcessor() *Processor {
	rawDB, err := database.DB.DB()
	if err != nil {
		log.Fatalf("Failed to get raw database connection: %v", err)
	}

	processor := &Processor{
		db:    database.DB,
		rawDB: rawDB,
		stringBuilderPool: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}

	return processor
}

// ProcessDirectoryOptimized provides enhanced parallel processing for large datasets
func (p *Processor) ProcessDirectoryOptimized(dataDir string) error {
	startTime := time.Now()

	files, err := filepath.Glob(filepath.Join(dataDir, "*.csv"))
	if err != nil {
		return fmt.Errorf("failed to find data files: %w", err)
	}
	txtFiles, err := filepath.Glob(filepath.Join(dataDir, "*.txt"))
	if err != nil {
		return fmt.Errorf("failed to find data files: %w", err)
	}
	files = append(files, txtFiles...)

	if len(files) == 0 {
		return fmt.Errorf("no data files (.csv or .txt) found in directory: %s", dataDir)
	}

	log.Printf("Found %d data files (.csv/.txt) to process with optimized parallel processing", len(files))

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
		return fmt.Errorf("failed to find data files: %w", err)
	}
	txtFiles, err := filepath.Glob(filepath.Join(dataDir, "*.txt"))
	if err != nil {
		return fmt.Errorf("failed to find data files: %w", err)
	}
	files = append(files, txtFiles...)

	if len(files) == 0 {
		return fmt.Errorf("no data files (.csv or .txt) found in directory: %s", dataDir)
	}

	log.Printf("Found %d data files (.csv/.txt) to process", len(files))

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

	// Use optimized processing with byte slices
	bufferSize := getBufferSize()
	workerCount := getWorkerCount()
	recordChan := make(chan []OptimizedTradeRecord, bufferSize)
	errorChan := make(chan error, workerCount)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go p.optimizedWorker(ctx, recordChan, errorChan, &wg)
	}

	// Read and parse CSV in batches with optimized parsing
	go func() {
		defer close(recordChan)

		reader := csv.NewReader(file)
		reader.Comma = ';'
		reader.ReuseRecord = true
		reader.LazyQuotes = true // Handle malformed quotes better

		var batch []OptimizedTradeRecord
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

			// Parse record with validation
			if len(record) < 9 {
				continue
			}

			// Use byte slices to avoid string allocations
			tradeRecord := OptimizedTradeRecord{
				DataNegocio:         []byte(strings.TrimSpace(record[8])),
				CodigoInstrumento:   []byte(strings.TrimSpace(record[1])),
				PrecoNegocio:        []byte(strings.TrimSpace(record[3])),
				QuantidadeNegociada: []byte(strings.TrimSpace(record[4])),
				HoraFechamento:      []byte(strings.TrimSpace(record[5])),
			}

			batch = append(batch, tradeRecord)

			batchSize := getBatchSize()
			if len(batch) >= batchSize {
				// Create a copy of the batch to send to workers
				batchCopy := make([]OptimizedTradeRecord, len(batch))
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
			batchCopy := make([]OptimizedTradeRecord, len(batch))
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

// optimizedWorker processes batches using raw SQL for maximum performance
func (p *Processor) optimizedWorker(ctx context.Context, recordChan <-chan []OptimizedTradeRecord, errorChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case batch, ok := <-recordChan:
			if !ok {
				return
			}
			if err := p.processBatchOptimized(batch); err != nil {
				errorChan <- err
				return
			}
		case <-ctx.Done():
			return
		}
	}
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

// processBatchOptimized uses raw SQL and bulk insert for maximum performance
func (p *Processor) processBatchOptimized(records []OptimizedTradeRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Build bulk insert query
	builder := p.stringBuilderPool.Get().(*strings.Builder)
	builder.Reset()
	defer p.stringBuilderPool.Put(builder)

	builder.WriteString(InsertTradeSQL)

	values := make([]interface{}, 0, len(records)*6)
	validRecords := 0

	now := time.Now()

	for _, record := range records {
		// Parse data with caching
		dataNegocio, err := p.parseDate(record.DataNegocio)
		if err != nil {
			continue // Skip invalid records
		}

		// Parse price using unsafe conversion for performance
		precoStr := p.bytesToString(record.PrecoNegocio)
		precoStr = strings.Replace(precoStr, ",", ".", -1)
		preco, err := strconv.ParseFloat(precoStr, 64)
		if err != nil {
			continue
		}

		// Parse quantity
		quantidadeStr := p.bytesToString(record.QuantidadeNegociada)
		quantidadeStr = strings.Replace(quantidadeStr, ",", ".", -1)
		quantidadeFloat, err := strconv.ParseFloat(quantidadeStr, 64)
		if err != nil {
			continue
		}
		quantidade := uint64(quantidadeFloat)

		if validRecords > 0 {
			builder.WriteString(", ")
		}

		// PostgreSQL uses $1, $2, etc. for placeholders
		paramBase := validRecords * 6
		builder.WriteString(fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)",
			paramBase+1, paramBase+2, paramBase+3, paramBase+4, paramBase+5, paramBase+6))

		values = append(values,
			dataNegocio,
			p.bytesToString(record.CodigoInstrumento),
			preco,
			quantidade,
			p.bytesToString(record.HoraFechamento),
			now,
		)
		validRecords++
	}

	if len(values) == 0 {
		return nil
	}

	// Execute bulk insert
	query := builder.String()

	// Begin transaction for better performance
	tx, err := p.rawDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to execute bulk insert: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Track processed rows
	atomic.AddInt64(&p.processedRows, int64(validRecords))
	return nil
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

// bytesToString converts byte slice to string without memory allocation
func (p *Processor) bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// parseDate parses date with caching for better performance
func (p *Processor) parseDate(dateBytes []byte) (time.Time, error) {
	dateStr := p.bytesToString(dateBytes)

	// Check cache first
	if cached, ok := p.dateParseCache.Load(dateStr); ok {
		return cached.(time.Time), nil
	}

	// Parse and cache
	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, err
	}

	p.dateParseCache.Store(dateStr, parsed)
	return parsed, nil
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
	// Use UPSERT (ON CONFLICT) for better performance instead of DELETE + INSERT
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
		ON CONFLICT (data_negocio, codigo_instrumento) 
		DO UPDATE SET 
			volume_total = EXCLUDED.volume_total,
			preco_maximo = EXCLUDED.preco_maximo,
			created_at = EXCLUDED.created_at
		ORDER BY data_negocio, codigo_instrumento
	`

	// Execute aggregation with timeout context and using raw SQL for better performance
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Use raw SQL instead of GORM for better performance
	_, err := p.rawDB.ExecContext(ctx, query)
	return err
}
