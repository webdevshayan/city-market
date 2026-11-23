// Admin-facing settlement statistics over the order ledger: revenue (GMV
// and the platform's Ujrah), rider payouts, and who is working and earning.
// Every money number either comes straight from the orders ledger or is
// recomputed by the FIXED Ujrah schedule in package finance — nothing here
// invents math, and no commission is ever stored.
package orders

import (
	"fmt"
	"time"

	"github.com/webdevshayan/city-market/server/finance"
)

// ShopSettlement is one shop's current-month ledger: the delivered sales the
// Ujrah schedule settles over, the platform's commission on them, and what
// the shop nets.
type ShopSettlement struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Orders     int64  `json:"orders"`
	SalesMinor int64  `json:"sales_minor"`
	UjrahMinor int64  `json:"ujrah_minor"`
	NetMinor   int64  `json:"net_minor"`
}

// RiderEarnings is one rider's all-time payout ledger. A rider's payout is
// earned at delivery, so only delivered orders appear here.
type RiderEarnings struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Deliveries  int64  `json:"deliveries"`
	PayoutMinor int64  `json:"payout_minor"`
}

// DailySeries is the 14-day history the admin console renders as charts:
// one bucket per UTC day, oldest first, including empty days so the
// timeline never skips.
type DailySeries struct {
	Days        []string `json:"days"`
	Orders      []int64  `json:"orders"`
	GMVMinor    []int64  `json:"gmv_minor"`
	PayoutMinor []int64  `json:"payout_minor"`
}

// StatsSnapshot is GET /v1/admin/stats — everything the admin console's
// stats view renders, computed from the live order ledger and dispatch
// tables in one call.
type StatsSnapshot struct {
	ShopsTotal         int64            `json:"shops_total"`
	ShopsActive        int64            `json:"shops_active"`
	ShopsSelling       int64            `json:"shops_selling"` // ≥1 delivered order, all time
	RidersTotal        int64            `json:"riders_total"`
	RidersAvailable    int64            `json:"riders_available"`
	RidersPaid         int64            `json:"riders_paid"` // ≥1 delivered order, all time
	OrdersTotal        int64            `json:"orders_total"`
	OrdersByStatus     map[string]int64 `json:"orders_by_status"`
	GMVMinor           int64            `json:"gmv_minor"` // all non-cancelled orders, all time
	GMVMonthMinor      int64            `json:"gmv_month_minor"`
	RiderPayoutsMinor  int64            `json:"rider_payouts_minor"` // delivered orders only — the payout ledger
	PlatformUjrahMinor int64            `json:"platform_ujrah_minor"`
	Shops              []ShopSettlement `json:"shops"`
	Riders             []RiderEarnings  `json:"riders"`
	Series             DailySeries      `json:"series"`
}

// Stats computes the snapshot. GMV and payouts come straight from the
// ledger; each shop's Ujrah is recomputed by the finance engine over the
// current month's delivered sales — the monthly settlement the fee model
// specifies — so platform revenue always matches the FIXED schedule. A
// settlement on an impossible ledger (a negative or out-of-bounds aggregate
// can only come from direct database tampering) settles at zero rather
// than failing the whole admin view.
func (s *Service) Stats() (StatsSnapshot, error) {
	snap := StatsSnapshot{OrdersByStatus: map[string]int64{}}

	if err := s.ledgerAggregates(&snap); err != nil {
		return StatsSnapshot{}, err
	}
	if err := s.dailySeries(&snap); err != nil {
		return StatsSnapshot{}, err
	}

	// This month's GMV — the current settlement window the Ujrah schedule
	// is computed over.
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(total_minor),0) FROM orders
		WHERE status <> ? AND created_at >= ?`,
		string(StatusCancelled), monthStart.Format(time.RFC3339Nano)).Scan(&snap.GMVMonthMinor); err != nil {
		return StatsSnapshot{}, fmt.Errorf("orders: stats: %w", err)
	}

	shops, err := s.dispatch.Shops()
	if err != nil {
		return StatsSnapshot{}, fmt.Errorf("orders: stats: %w", err)
	}
	sales, err := s.shopMonthSales()
	if err != nil {
		return StatsSnapshot{}, err
	}
	snap.ShopsTotal = int64(len(shops))
	snap.Shops = make([]ShopSettlement, 0, len(shops))
	for _, sh := range shops {
		month := sales[sh.ID]
		if sh.Active {
			snap.ShopsActive++
		}
		ujrah, _, _, err := finance.CommissionMinor(month.orders, month.salesMinor)
		if err != nil {
			ujrah = 0
		}
		snap.Shops = append(snap.Shops, ShopSettlement{
			ID:         sh.ID,
			Name:       sh.Name,
			Orders:     month.orders,
			SalesMinor: month.salesMinor,
			UjrahMinor: ujrah,
			NetMinor:   month.salesMinor - ujrah,
		})
		snap.PlatformUjrahMinor += ujrah
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(DISTINCT shop_id) FROM orders WHERE status = ?`,
		string(StatusDelivered)).Scan(&snap.ShopsSelling); err != nil {
		return StatsSnapshot{}, fmt.Errorf("orders: stats: %w", err)
	}

	riders, err := s.dispatch.Riders()
	if err != nil {
		return StatsSnapshot{}, fmt.Errorf("orders: stats: %w", err)
	}
	paid, err := s.riderPayoutRows()
	if err != nil {
		return StatsSnapshot{}, err
	}
	snap.RidersTotal = int64(len(riders))
	snap.Riders = make([]RiderEarnings, 0, len(riders))
	for _, rd := range riders {
		row := paid[rd.ID]
		if row.deliveries > 0 {
			snap.RidersPaid++
		}
		if rd.Active {
			snap.RidersAvailable++
		}
		snap.Riders = append(snap.Riders, RiderEarnings{
			ID:          rd.ID,
			Name:        rd.Name,
			Deliveries:  row.deliveries,
			PayoutMinor: row.payoutMinor,
		})
	}
	return snap, nil
}

// dailySeries buckets the last 14 UTC days of the ledger into the chart
// series: orders per day, GMV per day (cancelled excluded), and the
// rider payout ledger per day (delivered only). Bucketing happens in Go
// so the SQL stays plain timestamps — no database-specific date helpers.
func (s *Service) dailySeries(snap *StatsSnapshot) error {
	const days = 14
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -(days - 1))

	rows, err := s.db.Query(`SELECT created_at, status, total_minor, rider_payout_minor
		FROM orders WHERE created_at >= ?`, start.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("orders: stats: %w", err)
	}
	defer rows.Close()

	index := map[string]int{}
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		snap.Series.Days = append(snap.Series.Days, day)
		index[day] = i
	}
	snap.Series.Orders = make([]int64, days)
	snap.Series.GMVMinor = make([]int64, days)
	snap.Series.PayoutMinor = make([]int64, days)
	for rows.Next() {
		var createdAt, status string
		var total, payout int64
		if err := rows.Scan(&createdAt, &status, &total, &payout); err != nil {
			return fmt.Errorf("orders: stats: %w", err)
		}
		day := createdAt[:10] // RFC3339 starts with YYYY-MM-DD
		i, ok := index[day]
		if !ok {
			continue
		}
		snap.Series.Orders[i]++
		if status != string(StatusCancelled) {
			snap.Series.GMVMinor[i] += total
		}
		if status == string(StatusDelivered) {
			snap.Series.PayoutMinor[i] += payout
		}
	}
	return rows.Err()
}

// ledgerAggregates fills the order-level counters: totals and money sums
// grouped by status in one pass over the ledger.
func (s *Service) ledgerAggregates(snap *StatsSnapshot) error {
	rows, err := s.db.Query(`SELECT status, COUNT(*), COALESCE(SUM(total_minor),0),
		COALESCE(SUM(rider_payout_minor),0) FROM orders GROUP BY status`)
	if err != nil {
		return fmt.Errorf("orders: stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n, gmv, payouts int64
		if err := rows.Scan(&status, &n, &gmv, &payouts); err != nil {
			return fmt.Errorf("orders: stats: %w", err)
		}
		snap.OrdersByStatus[status] = n
		snap.OrdersTotal += n
		if status != string(StatusCancelled) {
			snap.GMVMinor += gmv
		}
		if status == string(StatusDelivered) {
			snap.RiderPayoutsMinor += payouts
		}
	}
	return rows.Err()
}

// shopMonthSales maps each shop to its current-month delivered order count
// and subtotal — the settlement basis of the Ujrah schedule.
func (s *Service) shopMonthSales() (map[string]struct {
	orders     int64
	salesMinor int64
}, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := s.db.Query(`SELECT shop_id, COUNT(*), COALESCE(SUM(subtotal_minor),0)
		FROM orders WHERE status = ? AND created_at >= ?
		GROUP BY shop_id ORDER BY SUM(subtotal_minor) DESC`,
		string(StatusDelivered), monthStart.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("orders: stats: %w", err)
	}
	defer rows.Close()
	out := map[string]struct {
		orders     int64
		salesMinor int64
	}{}
	for rows.Next() {
		var id string
		var n, sales int64
		if err := rows.Scan(&id, &n, &sales); err != nil {
			return nil, fmt.Errorf("orders: stats: %w", err)
		}
		out[id] = struct {
			orders     int64
			salesMinor int64
		}{orders: n, salesMinor: sales}
	}
	return out, rows.Err()
}

// riderPayoutRows maps each rider to their all-time delivered order count
// and payout sum — the payout ledger.
func (s *Service) riderPayoutRows() (map[string]struct {
	deliveries  int64
	payoutMinor int64
}, error) {
	rows, err := s.db.Query(`SELECT rider_id, COUNT(*), COALESCE(SUM(rider_payout_minor),0)
		FROM orders WHERE status = ?
		GROUP BY rider_id ORDER BY SUM(rider_payout_minor) DESC`,
		string(StatusDelivered))
	if err != nil {
		return nil, fmt.Errorf("orders: stats: %w", err)
	}
	defer rows.Close()
	out := map[string]struct {
		deliveries  int64
		payoutMinor int64
	}{}
	for rows.Next() {
		var id string
		var n, payouts int64
		if err := rows.Scan(&id, &n, &payouts); err != nil {
			return nil, fmt.Errorf("orders: stats: %w", err)
		}
		out[id] = struct {
			deliveries  int64
			payoutMinor int64
		}{deliveries: n, payoutMinor: payouts}
	}
	return out, rows.Err()
}
