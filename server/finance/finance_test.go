package finance

import (
	"errors"
	"testing"
)

// The tier boundaries are contractual: 0–50 → 0%, 51–200 → 5%,
// 201–500 → 7%, 501+ → 9%. Every boundary value is tested.
func TestTierFor(t *testing.T) {
	tests := []struct {
		name    string
		orders  int64
		wantBps int64
	}{
		{"zero orders is the free tier", 0, 0},
		{"boundary 50 stays 0%", 50, 0},
		{"boundary 51 starts 5%", 51, 500},
		{"boundary 200 stays 5%", 200, 500},
		{"boundary 201 starts 7%", 201, 700},
		{"boundary 500 stays 7%", 500, 700},
		{"boundary 501 starts 9%", 501, 900},
		{"deep top tier stays 9%", 10000, 900},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tier, err := TierFor(tc.orders)
			if err != nil {
				t.Fatalf("TierFor(%d) returned error: %v", tc.orders, err)
			}
			if tier.RateBps != tc.wantBps {
				t.Fatalf("TierFor(%d) rate = %d bps, want %d bps", tc.orders, tier.RateBps, tc.wantBps)
			}
		})
	}

	t.Run("negative orders rejected", func(t *testing.T) {
		if _, err := TierFor(-1); !errors.Is(err, ErrNegative) {
			t.Fatalf("want ErrNegative, got %v", err)
		}
	})
}

func TestCommissionMinor(t *testing.T) {
	tests := []struct {
		name       string
		orders     int64
		salesMinor int64
		wantComm   int64
		wantBps    int64
		wantCapped bool
	}{
		{"free tier charges nothing", 50, 999999999, 0, 0, false},
		{"51 orders: 5% of 1000.00 = 50.00", 51, 100000, 5000, 500, false},
		{"200 orders: 5% of 10000.00 = 500.00", 200, 1000000, 50000, 500, false},
		{"201 orders: 7% of 10000.00 = 700.00", 201, 1000000, 70000, 700, false},
		{"500 orders: 7% of 10000.00 = 700.00", 500, 1000000, 70000, 700, false},
		{"501 orders: 9% of 10000.00 = 900.00 (below cap)", 501, 1000000, 90000, 900, false},
		{"9% capped at 7000.00", 1000, 10000000, 700000, 900, true},
		{"floor rounding keeps remainder with merchant", 51, 12345, 617, 500, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comm, bps, capped, err := CommissionMinor(tc.orders, tc.salesMinor)
			if err != nil {
				t.Fatalf("CommissionMinor(%d, %d) returned error: %v", tc.orders, tc.salesMinor, err)
			}
			if comm != tc.wantComm {
				t.Fatalf("commission = %d minor, want %d", comm, tc.wantComm)
			}
			if bps != tc.wantBps {
				t.Fatalf("rate = %d bps, want %d", bps, tc.wantBps)
			}
			if capped != tc.wantCapped {
				t.Fatalf("capped = %v, want %v", capped, tc.wantCapped)
			}
		})
	}

	t.Run("negative sales rejected", func(t *testing.T) {
		if _, _, _, err := CommissionMinor(100, -1); !errors.Is(err, ErrNegative) {
			t.Fatalf("want ErrNegative, got %v", err)
		}
	})
}

func TestRiderPayoutMinor(t *testing.T) {
	tests := []struct {
		name       string
		deliveries int64
		km         int64
		wantMinor  int64
	}{
		{"no work, no pay", 0, 0, 0},
		{"one delivery, base only: 60.00", 1, 0, 6000},
		{"distance only: 15.00", 0, 1, 1500},
		{"ten deliveries over 25 km: 10×60 + 25×15 = 975.00", 10, 25, 97500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payout, _, _, err := RiderPayoutMinor(tc.deliveries, tc.km)
			if err != nil {
				t.Fatalf("RiderPayoutMinor(%d, %d) returned error: %v", tc.deliveries, tc.km, err)
			}
			if payout != tc.wantMinor {
				t.Fatalf("payout = %d minor, want %d", payout, tc.wantMinor)
			}
		})
	}

	t.Run("negative work rejected", func(t *testing.T) {
		if _, _, _, err := RiderPayoutMinor(-1, 0); !errors.Is(err, ErrNegative) {
			t.Fatalf("want ErrNegative, got %v", err)
		}
	})
}

func TestCustomerDeliveryFeeMinor(t *testing.T) {
	if got := CustomerDeliveryFeeMinor(); got != 2500 {
		t.Fatalf("flat checkout fee = %d minor, want 2500", got)
	}
}

func TestSimulateMonth(t *testing.T) {
	// 600 orders, 200000.00 sales → 9% would be 18000.00 → capped at 7000.00.
	// 600 deliveries over 1800 km → 600×60 + 1800×15 = 63000.00.
	// Customer fees: 600 × 25.00 = 15000.00.
	summary, err := SimulateMonth(600, 20000000, 600, 1800)
	if err != nil {
		t.Fatalf("SimulateMonth returned error: %v", err)
	}
	if summary.CommissionMinor != 700000 || !summary.CommissionCapped {
		t.Fatalf("commission = %d minor (capped=%v), want 700000 capped", summary.CommissionMinor, summary.CommissionCapped)
	}
	if summary.RiderPayoutMinor != 6300000 {
		t.Fatalf("rider payout = %d minor, want 6300000", summary.RiderPayoutMinor)
	}
	if summary.CustomerFeesMinor != 1500000 {
		t.Fatalf("customer fees = %d minor, want 1500000", summary.CustomerFeesMinor)
	}
	if summary.CommissionRateBps != 900 {
		t.Fatalf("rate = %d bps, want 900", summary.CommissionRateBps)
	}
}

func TestSimulateMonthRejectsNegative(t *testing.T) {
	if _, err := SimulateMonth(600, 20000000, -1, 0); !errors.Is(err, ErrNegative) {
		t.Fatalf("want ErrNegative, got %v", err)
	}
}
