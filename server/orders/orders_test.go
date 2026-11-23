package orders

import (
	"errors"
	"strings"
	"testing"

	// Registers the "libsql" driver (same one production uses) plus a pure-Go
	// sqlite driver so local file::memory: DSNs work. Both test-only.
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"

	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/match"
	"github.com/webdevshayan/city-market/server/store"
)

// fakeCat serves a fixed product list — the service must only depend on the
// ByID slice of the catalog provider.
type fakeCat struct{ products []catalog.Product }

func (f fakeCat) ByID(id string) (catalog.Product, bool) {
	for _, p := range f.products {
		if p.ID == id {
			return p, true
		}
	}
	return catalog.Product{}, false
}

type fakeDispatch struct{ shops, riders []match.Place }

func (f fakeDispatch) Shops() ([]match.Place, error)  { return f.shops, nil }
func (f fakeDispatch) Riders() ([]match.Place, error) { return f.riders, nil }

// fixture catalog mirrors the launch seeds: prices in minor units, one shop
// per product.
func fixtureCat() fakeCat {
	return fakeCat{products: []catalog.Product{
		{ID: "beef-1kg", PriceMinor: 140000, ShopID: "s-central"},
		{ID: "rice-5kg", PriceMinor: 220000, ShopID: "s-north"},
		{ID: "naan-10pcs", PriceMinor: 35000, ShopID: "s-central"},
	}}
}

func fixtureDispatch() fakeDispatch {
	return fakeDispatch{
		shops: []match.Place{
			{ID: "s-north", Name: "North Halal Store", Pos: match.Point{Lat: 24.9450, Lng: 67.0600}, Active: true},
			{ID: "s-central", Name: "Central Market Halal Store", Pos: match.Point{Lat: 24.8607, Lng: 67.0011}, Active: true},
		},
		riders: []match.Place{
			{ID: "r-1", Name: "Rider 1", Pos: match.Point{Lat: 24.8700, Lng: 67.0100}, Active: true},
			{ID: "r-2", Name: "Rider 2", Pos: match.Point{Lat: 24.8200, Lng: 67.0400}, Active: true},
			{ID: "r-3", Name: "Rider 3 (off duty)", Pos: match.Point{Lat: 24.8607, Lng: 67.0011}, Active: false},
		},
	}
}

func testService(t *testing.T, cat fakeCat, disp fakeDispatch) *Service {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return NewService(cat, disp, st.DB())
}

func TestPlaceHappyPath(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())

	// ~3 km due north of s-central: route km floors to 3.
	origin := match.Point{Lat: 24.8607 + 3.0/111.195*1.001, Lng: 67.0011}
	o, err := s.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 2}},
		Lat:   origin.Lat, Lng: origin.Lng,
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusPlaced {
		t.Fatalf("status = %q, want placed", o.Status)
	}
	if o.ShopID != "s-central" || o.ShopName != "Central Market Halal Store" {
		t.Fatalf("shop = %s/%s, want s-central", o.ShopID, o.ShopName)
	}
	// Riders are claimed from the dispatch board now — placement never
	// assigns one, so the field stays empty until a claim.
	if o.RiderID != "" {
		t.Fatalf("rider = %q, want empty at placement (claim assigns)", o.RiderID)
	}
	if o.SubtotalMinor != 2*140000 {
		t.Fatalf("subtotal = %d, want %d (price snapshot)", o.SubtotalMinor, 2*140000)
	}
	if o.DeliveryFeeMinor != 2500 { // finance.CustomerDeliveryFeeMinor
		t.Fatalf("delivery fee = %d, want 2500", o.DeliveryFeeMinor)
	}
	if o.TotalMinor != o.SubtotalMinor+o.DeliveryFeeMinor {
		t.Fatal("total must be subtotal + delivery fee")
	}
	// 3 km route: 6000 base + 3 × 1500 = 10500.
	if o.RiderPayoutMinor != 10500 {
		t.Fatalf("rider payout = %d, want 10500 (3 km route)", o.RiderPayoutMinor)
	}
	if len(o.Items) != 1 || o.Items[0].UnitPriceMinor != 140000 {
		t.Fatal("order items must snapshot the unit price")
	}

	got, err := s.ByID(o.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != StatusPlaced || got.TotalMinor != o.TotalMinor || len(got.Items) != 1 {
		t.Fatal("reloaded order must match the placed one")
	}
}

func TestPlaceMergesDuplicateLines(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	o, err := s.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 1}, {ProductID: "beef-1kg", Qty: 2}},
		Lat:   24.8610, Lng: 67.0011,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Items) != 1 || o.Items[0].Qty != 3 {
		t.Fatalf("items = %+v, want one merged line of qty 3", o.Items)
	}
}

func TestPlaceMultiShopRejected(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	_, err := s.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 1}, {ProductID: "rice-5kg", Qty: 1}},
		Lat:   24.86, Lng: 67.00,
	})
	if !errors.Is(err, ErrMultiShop) {
		t.Fatalf("want ErrMultiShop, got %v", err)
	}
}

func TestPlaceValidationErrors(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	cases := []struct {
		name string
		in   PlaceInput
		want error
	}{
		{"empty order", PlaceInput{Lat: 24, Lng: 67}, ErrEmptyOrder},
		{"qty zero", PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 0}}, Lat: 24, Lng: 67}, ErrBadQty},
		{"qty over cap", PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 100}}, Lat: 24, Lng: 67}, ErrBadQty},
		{"unknown product", PlaceInput{Items: []Item{{ProductID: "nope", Qty: 1}}, Lat: 24, Lng: 67}, ErrUnknownProduct},
		{"lat out of range", PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 91, Lng: 67}, ErrBadLocation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Place(tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestPlaceNoOpenShop(t *testing.T) {
	d := fixtureDispatch()
	for i := range d.shops {
		d.shops[i].Active = false // nothing open stocks anything
	}
	s := testService(t, fixtureCat(), d)
	_, err := s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00})
	if !errors.Is(err, ErrNoShopOpen) {
		t.Fatalf("want ErrNoShopOpen, got %v", err)
	}
}

// fakeEligibility decides which shops may receive orders this month.
type fakeEligibility struct{ ids []string }

func (f fakeEligibility) EligibleShops() ([]match.Place, error) {
	var out []match.Place
	for _, p := range fixtureDispatch().shops {
		for _, id := range f.ids {
			if p.ID == id {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func TestPlacePaidShopGate(t *testing.T) {
	// No shop is paid → placement is refused even though shelves are open.
	s := testService(t, fixtureCat(), fixtureDispatch()).WithEligibility(fakeEligibility{})
	// Auto mode with an empty eligible list is the plain no-shop refusal —
	// the citizen never named a shop, so the message stays generic.
	_, err := s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00})
	if !errors.Is(err, ErrNoShopOpen) {
		t.Fatalf("want ErrNoShopOpen, got %v", err)
	}

	// The citizen naming an unpaid shop gets the same refusal.
	_, err = s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00, ShopID: "s-central"})
	if !errors.Is(err, ErrShopUnavailable) {
		t.Fatalf("explicit unpaid shop: want ErrShopUnavailable, got %v", err)
	}

	// Once the admin marks the month paid, both paths work.
	s2 := testService(t, fixtureCat(), fixtureDispatch()).WithEligibility(fakeEligibility{ids: []string{"s-central"}})
	o, err := s2.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00})
	if err != nil {
		t.Fatalf("paid auto pick: %v", err)
	}
	if o.ShopID != "s-central" {
		t.Fatalf("shop = %q, want s-central", o.ShopID)
	}
	o2, err := s2.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00, ShopID: "s-central"})
	if err != nil || o2.ShopID != "s-central" {
		t.Fatalf("paid explicit pick: %v (%+v)", err, o2)
	}
}

func TestPlaceExplicitShopMismatch(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	// beef lives at s-central; naming s-north is a mismatch, not a silent
	// reassignment to whatever the customer said.
	_, err := s.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00, ShopID: "s-north"})
	if !errors.Is(err, ErrShopMismatch) {
		t.Fatalf("want ErrShopMismatch, got %v", err)
	}
}

func TestPlaceKeepsAddressNote(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	o, err := s.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00,
		AddressNote: "House near the water tank, green gate, ring the bell twice",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ByID(o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AddressNote != o.AddressNote || got.AddressNote == "" {
		t.Fatalf("address note = %q, want it persisted verbatim", got.AddressNote)
	}
	if _, err := s.Place(PlaceInput{
		Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00,
		AddressNote: strings.Repeat("x", 301)}); !errors.Is(err, ErrBadAddress) {
		t.Fatalf("over-long note: want ErrBadAddress, got %v", err)
	}
}

func TestTransitionLifecycle(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	o, err := s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00})
	if err != nil {
		t.Fatal(err)
	}

	steps := []Status{StatusConfirmed, StatusPickedUp, StatusDelivered}
	for _, want := range steps {
		got, err := s.Transition(o.ID, want)
		if err != nil {
			t.Fatalf("→ %s: %v", want, err)
		}
		if got.Status != want {
			t.Fatalf("status = %q, want %q", got.Status, want)
		}
	}

	// Terminal: nothing may follow delivered.
	if _, err := s.Transition(o.ID, StatusCancelled); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition after delivered, got %v", err)
	}

	// Skip steps are rejected: a fresh order cannot jump to picked_up.
	o2, err := s.Place(PlaceInput{Items: []Item{{ProductID: "naan-10pcs", Qty: 2}}, Lat: 24.86, Lng: 67.00})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transition(o2.ID, StatusPickedUp); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("placed → picked_up must be rejected, got %v", err)
	}

	// Cancellation is legal only before pickup.
	o3, err := s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transition(o3.ID, StatusCancelled); err != nil {
		t.Fatalf("placed → cancelled must be legal, got %v", err)
	}

	if _, err := s.ByID("ord-doesnotexist"); !errors.Is(err, ErrUnknownOrder) {
		t.Fatalf("want ErrUnknownOrder, got %v", err)
	}
	if _, err := s.Transition(o3.ID, "nonsense"); !errors.Is(err, ErrBadStatus) {
		t.Fatalf("want ErrBadStatus, got %v", err)
	}
}

func TestForShopAndForRider(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	for i := 0; i < 3; i++ {
		if _, err := s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00}); err != nil {
			t.Fatal(err)
		}
	}
	shopOrders, err := s.ForShop("s-central")
	if err != nil {
		t.Fatal(err)
	}
	if len(shopOrders) != 3 {
		t.Fatalf("shop orders = %d, want 3", len(shopOrders))
	}
	// No rider is assigned at placement — the board claim does that — so
	// the rider's list stays empty until the dispatch phase claims jobs.
	riderOrders, err := s.ForRider("r-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(riderOrders) != 0 {
		t.Fatalf("rider orders = %d, want 0 before any claim", len(riderOrders))
	}
	if empty, _ := s.ForShop("s-north"); len(empty) != 0 {
		t.Fatal("other shops must see no orders")
	}
}

// Names are joined from the live shops/riders tables: an admin rename is
// reflected on the next read without rewriting orders.
func TestNamesJoinFromLiveTables(t *testing.T) {
	s := testService(t, fixtureCat(), fixtureDispatch())
	o, err := s.Place(PlaceInput{Items: []Item{{ProductID: "beef-1kg", Qty: 1}}, Lat: 24.86, Lng: 67.00})
	if err != nil {
		t.Fatal(err)
	}
	// Seed the shop row (fixtures are not loaded in this test DB), then
	// rename it and read the order back.
	if _, err := s.db.Exec(
		`INSERT OR REPLACE INTO shops (id, name, lat, lng, active) VALUES (?, ?, 24.8607, 67.0011, 1)`,
		o.ShopID, "Central Market Halal Store"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE shops SET name = 'Renamed Store' WHERE id = ?`, o.ShopID); err != nil {
		t.Fatal(err)
	}
	got, err := s.ByID(o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ShopName != "Renamed Store" {
		t.Fatalf("shop name = %q, want the live renamed value", got.ShopName)
	}
}
