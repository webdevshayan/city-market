// Package store wires City Market to Turso (libSQL). A remote libsql:// DSN
// talks to Turso through the pure-Go libsql-client-go driver — no CGO, so
// the same binary builds for Vercel. Tests use local file::memory: DSNs,
// which the libsql driver delegates to any registered sqlite driver: the
// test files import modernc.org/sqlite for that, and the production binary
// never links it.
package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // registers the "libsql" driver

	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/match"
)

// ErrDisabled is returned by readers on a store without a database.
var ErrDisabled = errors.New("store: database not wired")

// Store wraps the database handle. A disabled store (empty DSN) is valid and
// reports Enabled() == false so callers can fall back to seed fixtures —
// DATABASE_URL is optional by design.
type Store struct {
	db *sql.DB
}

// Open connects to a Turso/libSQL database. An empty DSN returns a disabled
// store instead of an error.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		return &Store{}, nil
	}
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	return &Store{db: db}, nil
}

// Enabled reports whether a real database is wired.
func (s *Store) Enabled() bool { return s != nil && s.db != nil }

// DB exposes the raw handle for consumers that define their own queries
// (e.g. the cart store). Callers must check Enabled() first.
func (s *Store) DB() *sql.DB {
	if !s.Enabled() {
		return nil
	}
	return s.db
}

// Close closes the database handle.
func (s *Store) Close() error {
	if !s.Enabled() {
		return nil
	}
	return s.db.Close()
}

// schema is idempotent: every table is CREATE TABLE IF NOT EXISTS.
const schema = `
CREATE TABLE IF NOT EXISTS shops (
	id     TEXT PRIMARY KEY,
	name   TEXT NOT NULL,
	lat    REAL NOT NULL,
	lng    REAL NOT NULL,
	active INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS riders (
	id        TEXT PRIMARY KEY,
	name      TEXT NOT NULL,
	lat       REAL NOT NULL,
	lng       REAL NOT NULL,
	available INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS products (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	category    TEXT NOT NULL,
	brand       TEXT NOT NULL,
	size        TEXT NOT NULL,
	price_minor INTEGER NOT NULL,
	image       TEXT NOT NULL,
	shop_id     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cart_items (
	session_id TEXT NOT NULL,
	product_id TEXT NOT NULL,
	qty        INTEGER NOT NULL,
	PRIMARY KEY (session_id, product_id)
);
CREATE TABLE IF NOT EXISTS orders (
	id                TEXT PRIMARY KEY,
	status            TEXT NOT NULL,
	shop_id           TEXT NOT NULL,
	rider_id          TEXT NOT NULL,
	subtotal_minor    INTEGER NOT NULL,
	delivery_fee_minor INTEGER NOT NULL,
	total_minor       INTEGER NOT NULL,
	rider_payout_minor INTEGER NOT NULL,
	lat               REAL NOT NULL,
	lng               REAL NOT NULL,
	created_at        TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS order_items (
	order_id        TEXT NOT NULL,
	product_id      TEXT NOT NULL,
	qty             INTEGER NOT NULL,
	unit_price_minor INTEGER NOT NULL,
	PRIMARY KEY (order_id, product_id)
);
`

// EnsureSchema creates the tables if they are missing.
func (s *Store) EnsureSchema() error {
	if !s.Enabled() {
		return nil
	}
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: schema: %w", err)
	}
	return nil
}

// SeedShop/SeedRider are plain fixture rows, deliberately free of further
// domain types so wiring stays one-way (main composes everything).
type SeedShop struct {
	ID, Name string
	Lat, Lng float64
	Active   bool
}

type SeedRider struct {
	ID, Name  string
	Lat, Lng  float64
	Available bool
}

type SeedProduct struct {
	ID, Name, Category, Brand, Size, Image string
	PriceMinor                             int64
	ShopID                                 string
}

// SeedShops returns the launch shop fixtures — synthetic demo coordinates,
// labeled as such, until the admin console manages real shops.
func SeedShops() []SeedShop {
	return []SeedShop{
		{ID: "s-north", Name: "North Halal Store", Lat: 24.9450, Lng: 67.0600, Active: true},
		{ID: "s-central", Name: "Central Market Halal Store", Lat: 24.8607, Lng: 67.0011, Active: true},
		{ID: "s-south", Name: "South Halal Store", Lat: 24.8138, Lng: 67.0300, Active: true},
	}
}

// SeedRiders returns the launch rider fixtures; one is deliberately off duty
// so the dispatch skip-inactive path is exercised.
func SeedRiders() []SeedRider {
	return []SeedRider{
		{ID: "r-1", Name: "Rider 1", Lat: 24.8700, Lng: 67.0100, Available: true},
		{ID: "r-2", Name: "Rider 2", Lat: 24.8200, Lng: 67.0400, Available: true},
		{ID: "r-3", Name: "Rider 3", Lat: 24.9000, Lng: 67.0700, Available: true},
		{ID: "r-4", Name: "Rider 4 (off duty)", Lat: 24.8607, Lng: 67.0011, Available: false},
	}
}

// SeedProducts maps the catalog's launch fixtures into DB rows (one source
// of truth: catalog.Seed).
func SeedProducts() []SeedProduct {
	src := catalog.Seed()
	out := make([]SeedProduct, len(src))
	for i, p := range src {
		out[i] = SeedProduct{
			ID: p.ID, Name: p.Name, Category: p.Category, Brand: p.Brand,
			Size: p.Size, Image: p.Image, PriceMinor: p.PriceMinor, ShopID: p.ShopID,
		}
	}
	return out
}

// SeedIfEmpty inserts the launch fixtures when the products table is empty,
// so a fresh Turso database becomes usable on first boot. Safe on every
// start; already-populated tables are never rewritten.
func (s *Store) SeedIfEmpty(shops []SeedShop, riders []SeedRider, products []SeedProduct) error {
	if !s.Enabled() {
		return nil
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&n); err != nil {
		return fmt.Errorf("store: count products: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, sh := range shops {
		if _, err := s.db.Exec(
			`INSERT OR REPLACE INTO shops (id, name, lat, lng, active) VALUES (?, ?, ?, ?, ?)`,
			sh.ID, sh.Name, sh.Lat, sh.Lng, boolInt(sh.Active)); err != nil {
			return fmt.Errorf("store: seed shop %s: %w", sh.ID, err)
		}
	}
	for _, rd := range riders {
		if _, err := s.db.Exec(
			`INSERT OR REPLACE INTO riders (id, name, lat, lng, available) VALUES (?, ?, ?, ?, ?)`,
			rd.ID, rd.Name, rd.Lat, rd.Lng, boolInt(rd.Available)); err != nil {
			return fmt.Errorf("store: seed rider %s: %w", rd.ID, err)
		}
	}
	for _, p := range products {
		if _, err := s.db.Exec(
			`INSERT OR REPLACE INTO products (id, name, category, brand, size, price_minor, image, shop_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.Category, p.Brand, p.Size, p.PriceMinor, p.Image, p.ShopID); err != nil {
			return fmt.Errorf("store: seed product %s: %w", p.ID, err)
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Shops lists shops as dispatch candidates for the matching engine.
func (s *Store) Shops() ([]match.Place, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	rows, err := s.db.Query(`SELECT id, name, lat, lng, active FROM shops`)
	if err != nil {
		return nil, fmt.Errorf("store: shops: %w", err)
	}
	defer rows.Close()
	var out []match.Place
	for rows.Next() {
		var p match.Place
		var lat, lng float64
		var active int
		if err := rows.Scan(&p.ID, &p.Name, &lat, &lng, &active); err != nil {
			return nil, fmt.Errorf("store: scan shop: %w", err)
		}
		p.Pos = match.Point{Lat: lat, Lng: lng}
		p.Active = active == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// Riders lists riders as dispatch candidates (available → Active).
func (s *Store) Riders() ([]match.Place, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	rows, err := s.db.Query(`SELECT id, name, lat, lng, available FROM riders`)
	if err != nil {
		return nil, fmt.Errorf("store: riders: %w", err)
	}
	defer rows.Close()
	var out []match.Place
	for rows.Next() {
		var p match.Place
		var lat, lng float64
		var avail int
		if err := rows.Scan(&p.ID, &p.Name, &lat, &lng, &avail); err != nil {
			return nil, fmt.Errorf("store: scan rider: %w", err)
		}
		p.Pos = match.Point{Lat: lat, Lng: lng}
		p.Active = avail == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// Products lists every catalog row in insertion order — the DB-backed
// catalog provider reads through this.
func (s *Store) Products() ([]catalog.Product, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	rows, err := s.db.Query(
		`SELECT id, name, category, brand, size, price_minor, image, shop_id FROM products`)
	if err != nil {
		return nil, fmt.Errorf("store: products: %w", err)
	}
	defer rows.Close()
	out := make([]catalog.Product, 0, 8)
	for rows.Next() {
		var p catalog.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Brand, &p.Size,
			&p.PriceMinor, &p.Image, &p.ShopID); err != nil {
			return nil, fmt.Errorf("store: scan product: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertShop inserts or replaces one shop row (the admin console's write
// path). The Seed* structs double as the upsert payloads — same shape.
func (s *Store) UpsertShop(sh SeedShop) error {
	if _, err := s.db.Exec(
		`INSERT OR REPLACE INTO shops (id, name, lat, lng, active) VALUES (?, ?, ?, ?, ?)`,
		sh.ID, sh.Name, sh.Lat, sh.Lng, boolInt(sh.Active)); err != nil {
		return fmt.Errorf("store: upsert shop %s: %w", sh.ID, err)
	}
	return nil
}

// UpsertRider inserts or replaces one rider row.
func (s *Store) UpsertRider(rd SeedRider) error {
	if _, err := s.db.Exec(
		`INSERT OR REPLACE INTO riders (id, name, lat, lng, available) VALUES (?, ?, ?, ?, ?)`,
		rd.ID, rd.Name, rd.Lat, rd.Lng, boolInt(rd.Available)); err != nil {
		return fmt.Errorf("store: upsert rider %s: %w", rd.ID, err)
	}
	return nil
}

// UpsertProduct inserts or replaces one catalog row. Callers must reload
// the catalog snapshot (catalog.NewDB) for the change to go live.
func (s *Store) UpsertProduct(p SeedProduct) error {
	if _, err := s.db.Exec(
		`INSERT OR REPLACE INTO products (id, name, category, brand, size, price_minor, image, shop_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Category, p.Brand, p.Size, p.PriceMinor, p.Image, p.ShopID); err != nil {
		return fmt.Errorf("store: upsert product %s: %w", p.ID, err)
	}
	return nil
}
