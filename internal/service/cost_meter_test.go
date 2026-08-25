package service

import (
	"context"
	"testing"
)

// TestCostMeter_Basic verifies additions accumulate + Total uses defaults.
func TestCostMeter_Basic(t *testing.T) {
	m := NewCostMeter()
	m.AddDBQuery(false)
	m.AddDBQuery(false)
	m.AddDBQuery(true) // slow
	m.AddExternalAPI()
	m.AddCacheHit()

	// 3 DB queries * 0.0001 = 0.0003
	// 1 slow query * 0.001    = 0.001
	// 1 external API * 0.002  = 0.002
	// 1 cache hit * 0.0       = 0.0
	// Total = 0.0033
	got := m.TotalUSD()
	want := 0.0033
	if !floatNear(got, want, 1e-6) {
		t.Errorf("TotalUSD() = %v, want %v", got, want)
	}
}

// TestCostMeter_NilSafe - nil meter operations don't panic.
func TestCostMeter_NilSafe(t *testing.T) {
	var m *CostMeter
	m.AddDBQuery(false)
	m.AddExternalAPI()
	m.AddCacheHit()
	if got := m.TotalUSD(); got != 0 {
		t.Errorf("nil meter TotalUSD = %v, want 0", got)
	}
}

// TestCostMeter_Context - Meter round-trips through context.
func TestCostMeter_Context(t *testing.T) {
	m := NewCostMeter()
	m.AddExternalAPI()

	ctx := WithCostMeter(context.Background(), m)
	got := CostMeterFromContext(ctx)
	if got != m {
		t.Errorf("CostMeterFromContext returned different meter")
	}

	// Empty context returns nil.
	if CostMeterFromContext(context.Background()) != nil {
		t.Errorf("empty ctx should return nil meter")
	}
	// Nil context safe.
	if CostMeterFromContext(nil) != nil {
		t.Errorf("nil ctx should return nil meter")
	}
}

// TestCostMeter_SetRates - custom rates propagate to TotalUSD.
func TestCostMeter_SetRates(t *testing.T) {
	m := NewCostMeter()
	m.SetRates(0.01, 0.05, 0.0, 0.0)
	m.AddDBQuery(false)
	m.AddDBQuery(false)
	m.AddExternalAPI()

	// 2 DB * 0.01 + 1 external * 0.05 = 0.07
	got := m.TotalUSD()
	want := 0.07
	if !floatNear(got, want, 1e-6) {
		t.Errorf("TotalUSD with custom rates = %v, want %v", got, want)
	}
}

func floatNear(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}
