// Package finance implements City Market's Halal-compliant financial engine.
//
// The business rules below are fixed by the product specification and are the
// single source of truth for every settlement amount in the platform:
//
//	Shop commission (Ujrah) — tiered monthly commission on app sales:
//	    0–50 orders    → 0%
//	    51–200 orders  → 5%
//	    201–500 orders → 7%
//	    501+ orders    → 9%, capped at 7000 (major units) per month
//
//	Rider payout — 60 (major units) per completed delivery + 15 per km.
//
//	Customer fee — one flat delivery fee charged at checkout.
//
// Money is represented as int64 minor units (1 major unit = 100 minor units)
// so no floating point ever touches a settlement amount. Commission rounding
// always floors: any fractional remainder is resolved in the merchant's
// favour, so the platform never overcharges a shop.
package finance

import "errors"

const (
	// MinorPerUnit is how many minor units one major unit contains.
	MinorPerUnit int64 = 100

	// RiderBasePerDeliveryMajor is the fixed base payout per completed delivery.
	RiderBasePerDeliveryMajor int64 = 60

	// RiderPerKmMajor is the payout per kilometre of a delivery route.
	RiderPerKmMajor int64 = 15

	// TopTierCommissionCapMajor is the absolute monthly commission cap for the
	// 9% tier (501+ orders), per the product specification.
	TopTierCommissionCapMajor int64 = 7000

	// DefaultDeliveryFeeMajor is the flat customer checkout fee. The product
	// spec fixes the fee as "flat" but does not state its amount; 25 is the
	// launch default and is configurable without touching the engine.
	DefaultDeliveryFeeMajor int64 = 25
)

// ErrNegative is returned when an input amount or count is negative.
var ErrNegative = errors.New("finance: negative input")

// Tier is one commission bracket of the Ujrah schedule.
type Tier struct {
	MinOrders int64 // inclusive lower bound
	MaxOrders int64 // inclusive upper bound; 0 means unbounded
	RateBps   int64 // rate in basis points (500 = 5%)
}

// Tiers is the Ujrah commission schedule, ordered by MinOrders.
var Tiers = []Tier{
	{MinOrders: 0, MaxOrders: 50, RateBps: 0},
	{MinOrders: 51, MaxOrders: 200, RateBps: 500},
	{MinOrders: 201, MaxOrders: 500, RateBps: 700},
	{MinOrders: 501, MaxOrders: 0, RateBps: 900},
}

// TierFor returns the commission tier that applies to monthlyOrders.
func TierFor(monthlyOrders int64) (Tier, error) {
	if monthlyOrders < 0 {
		return Tier{}, ErrNegative
	}
	for _, t := range Tiers {
		if monthlyOrders >= t.MinOrders && (t.MaxOrders == 0 || monthlyOrders <= t.MaxOrders) {
			return t, nil
		}
	}
	return Tiers[len(Tiers)-1], nil // defensive; loop above always matches
}

// CommissionMinor returns the monthly Ujrah commission for a shop with
// monthlyOrders completed orders and salesMinor total sales, together with
// the applied rate and whether the 7000 cap clamped the result.
func CommissionMinor(monthlyOrders, salesMinor int64) (commissionMinor, rateBps int64, capped bool, err error) {
	if monthlyOrders < 0 || salesMinor < 0 {
		return 0, 0, false, ErrNegative
	}
	t, err := TierFor(monthlyOrders)
	if err != nil {
		return 0, 0, false, err
	}
	// Integer division floors, which keeps any rounding remainder on the
	// merchant's side of the ledger, not the platform's.
	commissionMinor = salesMinor * t.RateBps / 10000
	if t.RateBps == 900 {
		if capMinor := TopTierCommissionCapMajor * MinorPerUnit; commissionMinor > capMinor {
			commissionMinor = capMinor
			capped = true
		}
	}
	return commissionMinor, t.RateBps, capped, nil
}

// RiderPayoutMinor returns a rider's payout in minor units for deliveries
// completed deliveries over km kilometres, split into base and distance
// components: 60 per delivery + 15 per km.
func RiderPayoutMinor(deliveries, km int64) (payoutMinor, baseMinor, distanceMinor int64, err error) {
	if deliveries < 0 || km < 0 {
		return 0, 0, 0, ErrNegative
	}
	baseMinor = deliveries * RiderBasePerDeliveryMajor * MinorPerUnit
	distanceMinor = km * RiderPerKmMajor * MinorPerUnit
	return baseMinor + distanceMinor, baseMinor, distanceMinor, nil
}

// CustomerDeliveryFeeMinor returns the flat checkout delivery fee in minor units.
func CustomerDeliveryFeeMinor() int64 {
	return DefaultDeliveryFeeMajor * MinorPerUnit
}

// MonthSummary is a simulated month of marketplace activity, rolling the
// three business rules up into a single auditable statement.
type MonthSummary struct {
	Orders            int64 `json:"orders"`
	SalesMinor        int64 `json:"sales_minor"`
	CommissionMinor   int64 `json:"commission_minor"`
	CommissionRateBps int64 `json:"commission_rate_bps"`
	CommissionCapped  bool  `json:"commission_capped"`
	Deliveries        int64 `json:"deliveries"`
	Km                int64 `json:"km"`
	RiderPayoutMinor  int64 `json:"rider_payout_minor"`
	CustomerFeesMinor int64 `json:"customer_fees_minor"`
}

// SimulateMonth computes a month of marketplace activity: shop commission,
// total rider payout, and the customer delivery fees collected.
func SimulateMonth(monthlyOrders, salesMinor, deliveries, km int64) (MonthSummary, error) {
	summary := MonthSummary{
		Orders:     monthlyOrders,
		SalesMinor: salesMinor,
		Deliveries: deliveries,
		Km:         km,
	}
	commission, rateBps, capped, err := CommissionMinor(monthlyOrders, salesMinor)
	if err != nil {
		return MonthSummary{}, err
	}
	payout, _, _, err := RiderPayoutMinor(deliveries, km)
	if err != nil {
		return MonthSummary{}, err
	}
	summary.CommissionMinor = commission
	summary.CommissionRateBps = rateBps
	summary.CommissionCapped = capped
	summary.RiderPayoutMinor = payout
	summary.CustomerFeesMinor = deliveries * CustomerDeliveryFeeMinor()
	return summary, nil
}
