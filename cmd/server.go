package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/viktsys/b3ingest/api"
	"github.com/viktsys/b3ingest/database"
)

var serverCMD = &cobra.Command{
	Use:   "server",
	Short: "Start the API server",
	Long:  `Start the HTTP API server to serve trade statistics.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Initializing database...")
		if err := database.InitDB(); err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}

		r := api.SetupRoutes()

		port := ":8080"
		log.Printf("Starting server on port %s", port)
		if err := r.Run(port); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	},
}
