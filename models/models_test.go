package models

import (
	"testing"
	"time"
)

func TestTradeModel(t *testing.T) {
	trade := Trade{
		DataNegocio:         time.Now(),
		CodigoInstrumento:   "PETR4",
		PrecoNegocio:        25.50,
		QuantidadeNegociada: 1000,
		HoraFechamento:      "103000000",
	}

	if trade.CodigoInstrumento != "PETR4" {
		t.Errorf("Expected ticker PETR4, got %s", trade.CodigoInstrumento)
	}

	if trade.PrecoNegocio != 25.50 {
		t.Errorf("Expected price 25.50, got %f", trade.PrecoNegocio)
	}
}

func TestDailyAggregateModel(t *testing.T) {
	aggregate := DailyAggregate{
		DataNegocio:       time.Now(),
		CodigoInstrumento: "VALE3",
		VolumeTotal:       150000,
		PrecoMaximo:       85.75,
	}

	if aggregate.VolumeTotal != 150000 {
		t.Errorf("Expected volume 150000, got %d", aggregate.VolumeTotal)
	}
}

func TestTradeStats(t *testing.T) {
	stats := TradeStats{
		Ticker:         "PETR4",
		MaxRangeValue:  30.50,
		MaxDailyVolume: 250000,
	}

	if stats.Ticker != "PETR4" {
		t.Errorf("Expected ticker PETR4, got %s", stats.Ticker)
	}
}
