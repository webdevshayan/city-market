// Package orders implements City Market's order lifecycle: placement by
// citizens — which assigns the nearest active shop stocking every item and
// the nearest available rider to that shop, prices the order with the
// finance engine and persists everything — plus status advancement by shops
// and riders, and lookup for all the apps.
//
// Fee model, fixed by the product specification (see package finance):
//   - the customer pays subtotal + one flat delivery fee per order;
//   - shop commission (Ujrah) is a monthly per-shop settlement over sales,
//     not a per-order charge — an order only records the sale;
//   - this order's rider payout is the per-delivery base plus the
//     shop→customer route distance.
package orders

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/finance"
	"github.com/webdevshayan/city-market/server/match"
)

// Status is an order lifecycle state.
type Status string

const (
	StatusPlaced    Status = "placed"
	StatusConfirmed Status = "confirmed"
	StatusPickedUp  Status = "picked_up"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
)

// transitions maps each status to the statuses it may legally move to.
// Shops confirm (or cancel while fresh); riders pick up confirmed orders
// and deliver picked-up ones; delivered and cancelled are terminal.
var transitions = map[Status][]Status{
	StatusPlaced:    {StatusConfirmed, StatusCancelled},
	StatusConfirmed: {StatusPickedUp, StatusCancelled},
	StatusPickedUp:  {StatusDelivered},
	StatusDelivered: {},
	StatusCancelled: {},
}

// CanTransition reports whether a status change is legal.
func CanTransition(from, to Status) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Sentinel errors — handlers map them to HTTP status codes.
var (
	ErrEmptyOrder        = errors.New("orders: order must contain at least one item")
	ErrBadQty            = errors.New("orders: item quantities must be between 1 and 99")
	ErrUnknownProduct    = errors.New("orders: unknown product")
	ErrMultiShop         = errors.New("orders: items span multiple shops — split orders are not supported yet")
	ErrNoShopOpen        = errors.New("orders: no open shop stocks these items")
	ErrNoRider           = errors.New("orders: no rider available right now")
	ErrBadLocation       = errors.New("orders: lat must be in [-90, 90] and lng in [-180, 180]")
	ErrOutOfStock        = errors.New("orders: not enough stock on the shelf")
	ErrUnknownOrder      = errors.New("orders: unknown order")
	ErrInvalidTransition = errors.New("orders: status transition not allowed")
	ErrBadStatus         = errors.New("orders: unknown status")
)

// maxQty bounds one line's quantity; combined with catalog price bounds it
// keeps the subtotal product far below int64 overflow.
const maxQty = 99

// Item is one order line: the product, the quantity and the price snapshot
// at placement time (catalog prices may change later; the order does not).
type Item struct {
	ProductID      string `json:"product_id"`
	Qty            int64  `json:"qty"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
}

// Order is a persisted order with its fee breakdown and assignment.
type Order struct {
	ID               string    `json:"id"`
	Status           Status    `json:"status"`
	ShopID           string    `json:"shop_id"`
	ShopName         string    `json:"shop_name"`
	RiderID          string    `json:"rider_id"`
	RiderName        string    `json:"rider_name"`
	Items            []Item    `json:"items"`
	SubtotalMinor    int64     `json:"subtotal_minor"`
	DeliveryFeeMinor int64     `json:"delivery_fee_minor"`
	TotalMinor       int64     `json:"total_minor"`
	RiderPayoutMinor int64     `json:"rider_payout_minor"`
	Lat              float64   `json:"lat"`
	Lng              float64   `json:"lng"`
	CreatedAt        time.Time `json:"created_at"`
}

// PlaceInput is the citizen's order request.
type PlaceInput struct {
	Items []Item
	Lat   float64
	Lng   float64
}

// Catalog is the slice of the catalog provider the service needs.
type Catalog interface {
	ByID(id string) (catalog.Product, bool)
}

// Dispatch is the slice of the places source the service needs.
type Dispatch interface {
	Shops() ([]match.Place, error)
	Riders() ([]match.Place, error)
}

// StockSource is the live-shelf check placement consults before writing an
// order: a short shelf is a 409 for the citizen, not a bad surprise for the
// shop at accept time. Optional — a Service without one accepts placement
// against the catalog alone (dev fixtures).
type StockSource interface {
	HasStock(productID string, qty int64) (bool, error)
}

// Service composes catalog, dispatch and persistence. Orders are durable by
// nature, so persistence requires a database: wire a Service only when the
// store is enabled.
type Service struct {
	cat      Catalog
	dispatch Dispatch
	db       *sql.DB
	stock    StockSource
}

// NewService wires the order engine.
func NewService(cat Catalog, dispatch Dispatch, db *sql.DB) *Service {
	return &Service{cat: cat, dispatch: dispatch, db: db}
}

// WithStock attaches the live-shelf check.
func (s *Service) WithStock(src StockSource) *Service {
	s.stock = src
	return s
}

// Place validates and prices the order, assigns the fulfilment shop (the
// nearest open one stocking every item) and the rider (the nearest
// available one to that shop), and persists everything in one transaction.
func (s *Service) Place(in PlaceInput) (Order, error) {
	if len(in.Items) == 0 {
		return Order{}, ErrEmptyOrder
	}
	if in.Lat < -90 || in.Lat > 90 || in.Lng < -180 || in.Lng > 180 {
		return Order{}, ErrBadLocation
	}

	// Merge duplicate product lines, then snapshot prices from the catalog.
	qtys := map[string]int64{}
	for _, it := range in.Items {
		if it.Qty < 1 || it.Qty > maxQty {
			return Order{}, ErrBadQty
		}
		qtys[it.ProductID] += it.Qty
	}
	ids := make([]string, 0, len(qtys))
	for id := range qtys {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic item order regardless of input order

	items := make([]Item, 0, len(ids))
	shopID := ""
	var subtotal int64
	for _, pid := range ids {
		p, ok := s.cat.ByID(pid)
		if !ok {
			return Order{}, fmt.Errorf("%w: %s", ErrUnknownProduct, pid)
		}
		if shopID == "" {
			shopID = p.ShopID
		} else if p.ShopID != shopID {
			return Order{}, ErrMultiShop
		}
		items = append(items, Item{ProductID: pid, Qty: qtys[pid], UnitPriceMinor: p.PriceMinor})
		subtotal += p.PriceMinor * qtys[pid]
	}

	// The shelf check runs before anything is written: the citizen learns
	// about a short shelf immediately, and the shop's accept step (which
	// decrements atomically) never sees an impossible order.
	if s.stock != nil {
		for _, it := range items {
			ok, err := s.stock.HasStock(it.ProductID, it.Qty)
			if err != nil {
				return Order{}, fmt.Errorf("orders: stock: %w", err)
			}
			if !ok {
				return Order{}, fmt.Errorf("%w: %s", ErrOutOfStock, it.ProductID)
			}
		}
	}

	shops, err := s.dispatch.Shops()
	if err != nil {
		return Order{}, fmt.Errorf("orders: dispatch source: %w", err)
	}
	var candidates []match.Place
	for _, sh := range shops {
		if sh.ID == shopID && sh.Active {
			candidates = append(candidates, sh)
		}
	}
	origin := match.Point{Lat: in.Lat, Lng: in.Lng}
	shop, _, err := match.Nearest(origin, candidates)
	if err != nil {
		return Order{}, ErrNoShopOpen
	}

	riders, err := s.dispatch.Riders()
	if err != nil {
		return Order{}, fmt.Errorf("orders: dispatch source: %w", err)
	}
	rider, _, err := match.Nearest(shop.Pos, riders)
	if err != nil {
		return Order{}, ErrNoRider
	}

	km := int64(match.HaversineMeters(shop.Pos, origin) / 1000)
	payout, _, _, err := finance.RiderPayoutMinor(1, km)
	if err != nil {
		return Order{}, fmt.Errorf("orders: payout: %w", err)
	}
	deliveryFee := finance.CustomerDeliveryFeeMinor()

	id, err := newID()
	if err != nil {
		return Order{}, fmt.Errorf("orders: id: %w", err)
	}
	o := Order{
		ID:               id,
		Status:           StatusPlaced,
		ShopID:           shop.ID,
		ShopName:         shop.Name,
		RiderID:          rider.ID,
		RiderName:        rider.Name,
		Items:            items,
		SubtotalMinor:    subtotal,
		DeliveryFeeMinor: deliveryFee,
		TotalMinor:       subtotal + deliveryFee,
		RiderPayoutMinor: payout,
		Lat:              in.Lat,
		Lng:              in.Lng,
		CreatedAt:        time.Now().UTC(),
	}
	if err := s.insert(o); err != nil {
		return Order{}, fmt.Errorf("orders: persist: %w", err)
	}
	return o, nil
}

// ByID loads one order by its ID.
func (s *Service) ByID(id string) (Order, error) {
	return s.loadOne(`SELECT `+orderCols+orderFrom+` WHERE o.id = ?`, id)
}

// ForShop lists a shop's orders, newest first.
func (s *Service) ForShop(shopID string) ([]Order, error) {
	return s.loadMany(`SELECT `+orderCols+orderFrom+` WHERE o.shop_id = ? ORDER BY o.created_at DESC`, shopID)
}

// ForRider lists a rider's assigned orders, newest first.
func (s *Service) ForRider(riderID string) ([]Order, error) {
	return s.loadMany(`SELECT `+orderCols+orderFrom+` WHERE o.rider_id = ? ORDER BY o.created_at DESC`, riderID)
}

// Transition moves an order to the next legal status (optimistic update on
// the observed status, so a stale concurrent advance fails instead of
// skipping a step).
func (s *Service) Transition(id string, to Status) (Order, error) {
	if _, ok := transitions[to]; !ok {
		return Order{}, ErrBadStatus
	}
	o, err := s.ByID(id)
	if err != nil {
		return Order{}, err
	}
	if !CanTransition(o.Status, to) {
		return Order{}, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, o.Status, to)
	}
	res, err := s.db.Exec(`UPDATE orders SET status = ? WHERE id = ? AND status = ?`,
		string(to), id, string(o.Status))
	if err != nil {
		return Order{}, fmt.Errorf("orders: update: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Order{}, ErrUnknownOrder
	}
	o.Status = to
	return o, nil
}

func (s *Service) insert(o Order) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit
	if _, err = tx.Exec(
		`INSERT INTO orders (id, status, shop_id, rider_id, subtotal_minor, delivery_fee_minor,
		 total_minor, rider_payout_minor, lat, lng, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, string(o.Status), o.ShopID, o.RiderID, o.SubtotalMinor, o.DeliveryFeeMinor,
		o.TotalMinor, o.RiderPayoutMinor, o.Lat, o.Lng, o.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	for _, it := range o.Items {
		if _, err = tx.Exec(
			`INSERT INTO order_items (order_id, product_id, qty, unit_price_minor) VALUES (?, ?, ?, ?)`,
			o.ID, it.ProductID, it.Qty, it.UnitPriceMinor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const (
	orderCols = `o.id, o.status, o.shop_id, COALESCE(sh.name, o.shop_id), o.rider_id,
		COALESCE(rd.name, o.rider_id), o.subtotal_minor, o.delivery_fee_minor, o.total_minor,
		o.rider_payout_minor, o.lat, o.lng, o.created_at`
	// Names come from the live shops/riders tables so admin renames are
	// reflected without rewriting orders.
	orderFrom = ` FROM orders o
		LEFT JOIN shops sh ON sh.id = o.shop_id
		LEFT JOIN riders rd ON rd.id = o.rider_id`
)

func scanOrder(row interface{ Scan(...any) error }) (Order, error) {
	var o Order
	var status, createdAt string
	if err := row.Scan(&o.ID, &status, &o.ShopID, &o.ShopName, &o.RiderID, &o.RiderName,
		&o.SubtotalMinor, &o.DeliveryFeeMinor, &o.TotalMinor, &o.RiderPayoutMinor,
		&o.Lat, &o.Lng, &createdAt); err != nil {
		return Order{}, err
	}
	o.Status = Status(status)
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Order{}, fmt.Errorf("orders: parse created_at: %w", err)
	}
	o.CreatedAt = t
	return o, nil
}

func (s *Service) loadOne(query string, args ...any) (Order, error) {
	row := s.db.QueryRow(query, args...)
	o, err := scanOrder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrUnknownOrder
	}
	if err != nil {
		return Order{}, fmt.Errorf("orders: load: %w", err)
	}
	items, err := s.items(o.ID)
	if err != nil {
		return Order{}, err
	}
	o.Items = items
	return o, nil
}

func (s *Service) loadMany(query string, args ...any) ([]Order, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("orders: load: %w", err)
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("orders: scan: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orders: load: %w", err)
	}
	for i := range out {
		items, err := s.items(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

func (s *Service) items(orderID string) ([]Item, error) {
	rows, err := s.db.Query(
		`SELECT product_id, qty, unit_price_minor FROM order_items WHERE order_id = ? ORDER BY product_id`,
		orderID)
	if err != nil {
		return nil, fmt.Errorf("orders: load items: %w", err)
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ProductID, &it.Qty, &it.UnitPriceMinor); err != nil {
			return nil, fmt.Errorf("orders: scan items: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func newID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ord-" + hex.EncodeToString(b), nil
}
