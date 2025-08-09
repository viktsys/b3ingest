package ingest

import (
	"strings"
	"testing"
	"time"
)

func TestParseTradeRecord(t *testing.T) {
	processor := NewProcessor()

	record := TradeRecord{
		DataNegocio:         "2024-01-15",
		CodigoInstrumento:   "PETR4",
		PrecoNegocio:        "25,50",
		QuantidadeNegociada: "1000",
		HoraFechamento:      "103000000",
	}

	trade, err := processor.parseTradeRecord(record)
	if err != nil {
		t.Fatalf("Failed to parse trade record: %v", err)
	}

	expectedDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !trade.DataNegocio.Equal(expectedDate) {
		t.Errorf("Expected date %v, got %v", expectedDate, trade.DataNegocio)
	}

	if trade.CodigoInstrumento != "PETR4" {
		t.Errorf("Expected ticker PETR4, got %s", trade.CodigoInstrumento)
	}

	if trade.PrecoNegocio != 25.50 {
		t.Errorf("Expected price 25.50, got %f", trade.PrecoNegocio)
	}

	if trade.QuantidadeNegociada != 1000 {
		t.Errorf("Expected quantity 1000, got %d", trade.QuantidadeNegociada)
	}
}

func TestParseInvalidDate(t *testing.T) {
	processor := NewProcessor()

	record := TradeRecord{
		DataNegocio:         "invalid-date",
		CodigoInstrumento:   "PETR4",
		PrecoNegocio:        "25,50",
		QuantidadeNegociada: "1000",
		HoraFechamento:      "103000000",
	}

	_, err := processor.parseTradeRecord(record)
	if err == nil {
		t.Error("Expected error for invalid date, got nil")
	}

	if !strings.Contains(err.Error(), "invalid date format") {
		t.Errorf("Expected 'invalid date format' error, got %v", err)
	}
}

func TestParseInvalidPrice(t *testing.T) {
	processor := NewProcessor()

	record := TradeRecord{
		DataNegocio:         "2024-01-15",
		CodigoInstrumento:   "PETR4",
		PrecoNegocio:        "invalid-price",
		QuantidadeNegociada: "1000",
		HoraFechamento:      "103000000",
	}

	_, err := processor.parseTradeRecord(record)
	if err == nil {
		t.Error("Expected error for invalid price, got nil")
	}
}

func TestParseInvalidQuantity(t *testing.T) {
	processor := NewProcessor()

	record := TradeRecord{
		DataNegocio:         "2024-01-15",
		CodigoInstrumento:   "PETR4",
		PrecoNegocio:        "25,50",
		QuantidadeNegociada: "invalid-quantity",
		HoraFechamento:      "103000000",
	}

	_, err := processor.parseTradeRecord(record)
	if err == nil {
		t.Error("Expected error for invalid quantity, got nil")
	}
}
