package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/viktsys/b3ingest/database"
	"github.com/viktsys/b3ingest/ingest"
)

var ingestCMD = &cobra.Command{
	Use:   "ingest [data-directory]",
	Short: "Ingest CSV data from the specified directory with parallel processing",
	Long:  `Process and ingest B3 trading data from CSV files in the specified directory using optimized parallel goroutines for faster processing.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dataDir := args[0]

		log.Println("Initializing database...")
		if err := database.InitDB(); err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}

		processor := ingest.NewProcessor()

		log.Printf("Starting optimized parallel ingestion from directory: %s", dataDir)

		// Use the optimized processor for better performance
		if err := processor.ProcessDirectoryOptimized(dataDir); err != nil {
			log.Fatalf("Failed to process data: %v", err)
		}

		fmt.Println("Data ingestion completed successfully with parallel processing!")
	},
}
