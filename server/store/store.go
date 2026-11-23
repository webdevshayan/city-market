// Package store wires City Market to Turso (libSQL). A remote libsql:// DSN
// talks to Turso through the pure-Go libsql-client-go driver — no CGO, so
// the same binary builds for Vercel. Tests use local file::memory: DSNs,
// which the libsql driver delegates to any registered sqlite driver: the
// test files import modernc.org/sqlite for that, and the production binary
// never links it.
//
// Schema evolution is additive: fresh databases get the full CREATE TABLE
// shape, existing databases get the newer columns through idempotent
// ALTER TABLE statements that tolerate "duplicate column" errors. No
// migration ever rewrites or drops an existing column, so an upgrade is a
// plain redeploy.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

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
// store instead of an error. When DATABASE_AUTH_TOKEN is set (the platform
// token from `turso db tokens create`) it is appended to the DSN as the
// authToken query parameter — deployment config stays two plain env vars
// (URL + token) instead of one hand-assembled URL.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		return &Store{}, nil
	}
	if tok := os.Getenv("DATABASE_AUTH_TOKEN"); tok != "" && !strings.Contains(dsn, "authToken=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "authToken=" + url.QueryEscape(tok)
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
// (e.g. the orders engine). Callers must check Enabled() first.
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

// schema is idempotent: every table is CREATE TABLE IF NOT EXISTS, already
// carrying every column the marketplace phase introduced.
const schema = `
CREATE TABLE IF NOT EXISTS shops (
	id            TEXT PRIMARY KEY,
	name          TEXT NOT NULL,
	lat           REAL NOT NULL,
	lng           REAL NOT NULL,
	active        INTEGER NOT NULL DEFAULT 1,
	phone         TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL DEFAULT '',
	paid_through  TEXT NOT NULL DEFAULT '',
	payment_note  TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS riders (
	id            TEXT PRIMARY KEY,
	name          TEXT NOT NULL,
	lat           REAL NOT NULL,
	lng           REAL NOT NULL,
	available     INTEGER NOT NULL DEFAULT 1,
	phone         TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS products (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	category    TEXT NOT NULL,
	brand       TEXT NOT NULL,
	size        TEXT NOT NULL,
	price_minor INTEGER NOT NULL,
	image       TEXT NOT NULL,
	shop_id     TEXT NOT NULL,
	stock       INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS orders (
	id                 TEXT PRIMARY KEY,
	status             TEXT NOT NULL,
	shop_id            TEXT NOT NULL,
	rider_id           TEXT NOT NULL DEFAULT '',
	subtotal_minor     INTEGER NOT NULL,
	delivery_fee_minor INTEGER NOT NULL,
	total_minor        INTEGER NOT NULL,
	rider_payout_minor INTEGER NOT NULL,
	lat                REAL NOT NULL,
	lng                REAL NOT NULL,
	created_at         TEXT NOT NULL,
	address_note       TEXT NOT NULL DEFAULT '',
	est_route_m        INTEGER NOT NULL DEFAULT 0,
	claimed_at         TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS order_items (
	order_id        TEXT NOT NULL,
	product_id      TEXT NOT NULL,
	qty             INTEGER NOT NULL,
	unit_price_minor INTEGER NOT NULL,
	PRIMARY KEY (order_id, product_id)
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	role       TEXT NOT NULL,
	subject    TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
`

// migrations adds the marketplace-phase columns to databases created by an
// older deployment. Each ALTER runs once per boot and fails benignly with
// "duplicate column name" once applied — there is no migration-version
// table to keep in sync, and additive changes never rewrite data.
var migrations = []string{
	`ALTER TABLE shops ADD COLUMN phone TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE shops ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE shops ADD COLUMN paid_through TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE shops ADD COLUMN payment_note TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE riders ADD COLUMN phone TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE riders ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE products ADD COLUMN stock INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE orders ADD COLUMN address_note TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE orders ADD COLUMN est_route_m INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE orders ADD COLUMN claimed_at TEXT NOT NULL DEFAULT ''`,
}

// EnsureSchema creates the tables if they are missing and adds newer columns
// to older tables. Duplicate-column failures from the migration list are the
// success signal, not errors.
func (s *Store) EnsureSchema() error {
	if !s.Enabled() {
		return nil
	}
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: schema: %w", err)
	}
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("store: migration %q: %w", m, err)
		}
	}
	return nil
}

// SeedShop is a shop fixture row — also the shape of the admin upsert
// payload, which is why the credential and subscription columns stay out of
// it (only the auth and subscription methods write those).
type SeedShop struct {
	ID, Name string
	Lat, Lng float64
	Active   bool
}

// SeedRider is a rider fixture row (and the admin upsert payload).
type SeedRider struct {
	ID, Name  string
	Lat, Lng  float64
	Available bool
}

// SeedProduct is a catalog fixture row (and the upsert payload); Stock is
// the on-shelf quantity the shop owner manages.
type SeedProduct struct {
	ID, Name, Category, Brand, Size, Image string
	PriceMinor                             int64
	ShopID                                 string
	Stock                                  int64
}

// Johi is the market's home town — the fixtures sit inside realistic
// walking distance of Johi's bazaar (Dadu District, Sindh), per Wikipedia
// 26°41′31″N 67°36′48″E.
var (
	JohiCenter = match.Point{Lat: 26.692061, Lng: 67.613300}
)

// SeedShops returns the launch shop fixtures anchored on Johi. A new
// shop registered from the shop app starts unpaid — the admin marks the
// month paid before it can receive orders.
func SeedShops() []SeedShop {
	return []SeedShop{
		{ID: "s-central", Name: "Madina Halal Store — Main Bazaar Johi", Lat: 26.6931, Lng: 67.6156, Active: true},
		{ID: "s-north", Name: "Al-Karam Grocery & Meat — Johi", Lat: 26.6975, Lng: 67.6180, Active: true},
		{ID: "s-south", Name: "Bismillah Fresh Mart — Dadu Road Johi", Lat: 26.6880, Lng: 67.6110, Active: true},
	}
}

// SeedRiders returns the launch rider fixtures around Johi; one is
// deliberately off duty so the on-duty filter is exercised.
func SeedRiders() []SeedRider {
	return []SeedRider{
		{ID: "r-1", Name: "Ali Raza", Lat: 26.6940, Lng: 67.6160, Available: true},
		{ID: "r-2", Name: "Ghulam Hussain", Lat: 26.6885, Lng: 67.6100, Available: true},
		{ID: "r-3", Name: "Imran Khaskheli", Lat: 26.6962, Lng: 67.6105, Available: true},
		{ID: "r-4", Name: "Murid Chang (off duty)", Lat: 26.6921, Lng: 67.6133, Available: false},
	}
}

// SeedProducts maps the catalog's launch fixtures into DB rows (one source
// of truth for the listing data: catalog.Seed; the stock counts are the
// launch shelves the shop app manages from here on).
func SeedProducts() []SeedProduct {
	src := catalog.Seed()
	out := make([]SeedProduct, len(src))
	for i, p := range src {
		out[i] = SeedProduct{
			ID: p.ID, Name: p.Name, Category: p.Category, Brand: p.Brand,
			Size: p.Size, Image: p.Image, PriceMinor: p.PriceMinor, ShopID: p.ShopID,
			Stock: p.Stock,
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
			`INSERT OR REPLACE INTO products (id, name, category, brand, size, price_minor, image, shop_id, stock)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.Category, p.Brand, p.Size, p.PriceMinor, p.Image, p.ShopID, p.Stock); err != nil {
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

// Shops lists shops as dispatch candidates for the matching engine and the
// admin's subscription view (PaidThrough rides along; the SQL stays one
// query so dispatch never pays for the extra column).
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
		`SELECT id, name, category, brand, size, price_minor, image, shop_id, stock FROM products`)
	if err != nil {
		return nil, fmt.Errorf("store: products: %w", err)
	}
	defer rows.Close()
	out := make([]catalog.Product, 0, 8)
	for rows.Next() {
		var p catalog.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Brand, &p.Size,
			&p.PriceMinor, &p.Image, &p.ShopID, &p.Stock); err != nil {
			return nil, fmt.Errorf("store: scan product: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertShop inserts or updates a shop's public fields — the admin
// console's write path. ON CONFLICT UPDATE (not REPLACE) so an admin edit
// never clobbers the shop's credentials or subscription state.
func (s *Store) UpsertShop(sh SeedShop) error {
	if _, err := s.db.Exec(
		`INSERT INTO shops (id, name, lat, lng, active) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, lat = excluded.lat,
		 lng = excluded.lng, active = excluded.active`,
		sh.ID, sh.Name, sh.Lat, sh.Lng, boolInt(sh.Active)); err != nil {
		return fmt.Errorf("store: upsert shop %s: %w", sh.ID, err)
	}
	return nil
}

// UpsertRider inserts or updates a rider's public fields, preserving
// credentials the way UpsertShop does.
func (s *Store) UpsertRider(rd SeedRider) error {
	if _, err := s.db.Exec(
		`INSERT INTO riders (id, name, lat, lng, available) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, lat = excluded.lat,
		 lng = excluded.lng, available = excluded.available`,
		rd.ID, rd.Name, rd.Lat, rd.Lng, boolInt(rd.Available)); err != nil {
		return fmt.Errorf("store: upsert rider %s: %w", rd.ID, err)
	}
	return nil
}

// UpsertProduct inserts or updates a catalog row. Callers must reload
// the catalog snapshot (catalog.NewDB) for the change to go live.
func (s *Store) UpsertProduct(p SeedProduct) error {
	if _, err := s.db.Exec(
		`INSERT INTO products (id, name, category, brand, size, price_minor, image, shop_id, stock)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, category = excluded.category,
		 brand = excluded.brand, size = excluded.size, price_minor = excluded.price_minor,
		 image = excluded.image, shop_id = excluded.shop_id, stock = excluded.stock`,
		p.ID, p.Name, p.Category, p.Brand, p.Size, p.PriceMinor, p.Image, p.ShopID, p.Stock); err != nil {
		return fmt.Errorf("store: upsert product %s: %w", p.ID, err)
	}
	return nil
}
