package orders

import (
	"fmt"
	"testing"
	"time"

	"github.com/webdevshayan/city-market/server/finance"
)

// deliverOne places a one-line order from a fixed customer point and walks
// it through confirm → pick-up → deliver, returning the final order.
func deliverOne(t *testing.T, svc *Service, productID string, qty int64, lat, lng float64) Order {
	t.Helper()
	o, err := svc.Place(PlaceInput{
		Items: []Item{{ProductID: productID, Qty: qty}},
		Lat:   lat, Lng: lng,
	})
	if err != nil {
		t.Fatalf("place %s: %v", productID, err)
	}
	for _, to := range []Status{StatusConfirmed, StatusPickedUp, StatusDelivered} {
		if o, err = svc.Transition(o.ID, to); err != nil {
			t.Fatalf("transition %s → %s: %v", o.ID, to, err)
		}
	}
	return o
}

func TestStatsSnapshot(t *testing.T) {
	svc := testService(t, fixtureCat(), fixtureDispatch())

	// Two delivered orders (s-central beef, s-north rice), one left placed
	// (GMV yes, payout no) and one cancelled (excluded from money entirely).
	// Riders claim jobs from the dispatch board now, so the test assigns
	// both delivered orders to r-1 directly — the payout ledger shape the
	// view reads is unchanged.
	o1 := deliverOne(t, svc, "beef-1kg", 2, 24.8610, 67.0011)
	o2 := deliverOne(t, svc, "rice-5kg", 1, 24.8610, 67.0011)
	for _, id := range []string{o1.ID, o2.ID} {
		if _, err := svc.db.Exec(`UPDATE orders SET rider_id = 'r-1' WHERE id = ?`, id); err != nil {
			t.Fatalf("assign rider: %v", err)
		}
	}
	o3, err := svc.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.8610, Lng: 67.0011,
	})
	if err != nil {
		t.Fatalf("place o3: %v", err)
	}
	o4, err := svc.Place(PlaceInput{
		Items: []Item{{ProductID: "naan-10pcs", Qty: 1}}, Lat: 24.8610, Lng: 67.0011,
	})
	if err != nil {
		t.Fatalf("place o4: %v", err)
	}
	if _, err := svc.Transition(o4.ID, StatusCancelled); err != nil {
		t.Fatalf("cancel o4: %v", err)
	}

	// 60 delivered month orders for s-north, assigned to r-2, inserted
	// directly: enough volume to push s-north past the 50-order tier so the
	// platform-revenue path is exercised with a nonzero Ujrah rate.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 60; i++ {
		if _, err := svc.db.Exec(`INSERT INTO orders (id, status, shop_id, rider_id, subtotal_minor,
			delivery_fee_minor, total_minor, rider_payout_minor, lat, lng, created_at)
			VALUES (?, 'delivered', 's-north', 'r-2', 100000, 0, 100000, 6000, 24.9, 67.06, ?)`,
			fmt.Sprintf("ord-vol-%02d", i), now); err != nil {
			t.Fatalf("volume insert: %v", err)
		}
	}

	snap, err := svc.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}

	// ——— order counts ———
	if snap.OrdersTotal != 64 {
		t.Errorf("orders_total = %d, want 64", snap.OrdersTotal)
	}
	if snap.OrdersByStatus["delivered"] != 62 || snap.OrdersByStatus["placed"] != 1 ||
		snap.OrdersByStatus["cancelled"] != 1 {
		t.Errorf("orders_by_status = %v", snap.OrdersByStatus)
	}

	// ——— money: the finance engine and the placed orders are the source of
	// truth for every expected value, never a hand-computed constant ———
	wantGMV := o1.TotalMinor + o2.TotalMinor + o3.TotalMinor + 60*100000
	if snap.GMVMinor != wantGMV {
		t.Errorf("gmv_minor = %d, want %d (cancelled excluded, placed included)", snap.GMVMinor, wantGMV)
	}
	if snap.GMVMonthMinor != wantGMV {
		t.Errorf("gmv_month_minor = %d, want %d (every order was created this month)", snap.GMVMonthMinor, wantGMV)
	}
	wantPayouts := o1.RiderPayoutMinor + o2.RiderPayoutMinor + 60*6000
	if snap.RiderPayoutsMinor != wantPayouts {
		t.Errorf("rider_payouts_minor = %d, want %d (delivered only — placed o3 excluded)",
			snap.RiderPayoutsMinor, wantPayouts)
	}

	// ——— shops: all listed, both active, both selling ———
	if snap.ShopsTotal != 2 || snap.ShopsActive != 2 || snap.ShopsSelling != 2 {
		t.Errorf("shops = total %d active %d selling %d, want 2/2/2",
			snap.ShopsTotal, snap.ShopsActive, snap.ShopsSelling)
	}
	var central, north *ShopSettlement
	for i := range snap.Shops {
		switch snap.Shops[i].ID {
		case "s-central":
			central = &snap.Shops[i]
		case "s-north":
			north = &snap.Shops[i]
		}
	}
	if central == nil || north == nil {
		t.Fatalf("shop settlements missing: %+v", snap.Shops)
	}
	if central.Orders != 1 || central.SalesMinor != o1.SubtotalMinor {
		t.Errorf("s-central settlement = %+v, want 1 delivered order / sales %d", central, o1.SubtotalMinor)
	}
	ujrahCentral, _, _, err := finance.CommissionMinor(central.Orders, central.SalesMinor)
	if err != nil {
		t.Fatalf("commission: %v", err)
	}
	if central.UjrahMinor != ujrahCentral || central.NetMinor != central.SalesMinor-ujrahCentral {
		t.Errorf("s-central ujrah/net = %d/%d, want the engine's %d/%d",
			central.UjrahMinor, central.NetMinor, ujrahCentral, central.SalesMinor-ujrahCentral)
	}
	if north.Orders != 61 || north.SalesMinor != o2.SubtotalMinor+60*100000 {
		t.Errorf("s-north settlement = %+v, want 61 delivered orders / sales %d",
			north, o2.SubtotalMinor+60*100000)
	}
	ujrahNorth, _, _, err := finance.CommissionMinor(north.Orders, north.SalesMinor)
	if err != nil {
		t.Fatalf("commission: %v", err)
	}
	if ujrahNorth == 0 {
		t.Fatalf("test wants a nonzero Ujrah tier — the schedule changed, update the fixture")
	}
	if north.UjrahMinor != ujrahNorth || north.NetMinor != north.SalesMinor-ujrahNorth {
		t.Errorf("s-north ujrah/net = %d/%d, want the engine's %d/%d",
			north.UjrahMinor, north.NetMinor, ujrahNorth, north.SalesMinor-ujrahNorth)
	}
	if snap.PlatformUjrahMinor != ujrahCentral+ujrahNorth {
		t.Errorf("platform_ujrah_minor = %d, want %d", snap.PlatformUjrahMinor, ujrahCentral+ujrahNorth)
	}

	// ——— riders: who is available, who is paid, who earns what ———
	if snap.RidersTotal != 3 || snap.RidersAvailable != 2 || snap.RidersPaid != 2 {
		t.Errorf("riders = total %d available %d paid %d, want 3/2/2",
			snap.RidersTotal, snap.RidersAvailable, snap.RidersPaid)
	}
	byID := map[string]RiderEarnings{}
	for _, re := range snap.Riders {
		byID[re.ID] = re
	}
	if byID["r-1"].Deliveries != 2 || byID["r-1"].PayoutMinor != o1.RiderPayoutMinor+o2.RiderPayoutMinor {
		t.Errorf("r-1 = %+v, want 2 deliveries / payouts %d",
			byID["r-1"], o1.RiderPayoutMinor+o2.RiderPayoutMinor)
	}
	if byID["r-2"].Deliveries != 60 || byID["r-2"].PayoutMinor != 60*6000 {
		t.Errorf("r-2 = %+v, want 60 deliveries / payouts %d", byID["r-2"], 60*6000)
	}
	if byID["r-3"].Deliveries != 0 || byID["r-3"].PayoutMinor != 0 {
		t.Errorf("r-3 = %+v, want an idle rider with no earnings", byID["r-3"])
	}
}
