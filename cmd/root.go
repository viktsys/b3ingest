package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCMD = &cobra.Command{
	Use:   "b3ingest",
	Short: "B3 Trading Data Ingestion and Analysis Tool",
	Long: `A CLI application for ingesting and analyzing B3 trading data.
This tool can process CSV files containing B3 trading data and provide
aggregated statistics through a REST API.`,
}

func Execute() {
	err := rootCMD.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// TODO: Add subcommands and flags here
}
