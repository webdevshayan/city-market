// Accounts, credentials, subscriptions, stock and sessions — the storage
// side of the marketplace. Only hashes ever land here: passwords are
// bcrypt digests written by the auth flow, and session rows carry a SHA-256
// of the bearer token, never the token itself.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ShopAccount is a shop row including its auth and subscription state.
// PasswordHash is a bcrypt digest — never a plaintext password.
type ShopAccount struct {
	ID           string
	Name         string
	Lat, Lng     float64
	Active       bool
	Phone        string
	PasswordHash string
	PaidThrough  string // "YYYY-MM"; empty = never paid
	PaymentNote  string // free text from the admin (method/trx reference)
}

// RiderAccount is a rider row including its auth state.
type RiderAccount struct {
	ID           string
	Name         string
	Lat, Lng     float64
	Available    bool
	Phone        string
	PasswordHash string
}

// ErrNotFound is returned by lookups that match no row.
var ErrNotFound = sql.ErrNoRows

// ShopByPhone loads a shop by its login phone (normalized +92…).
func (s *Store) ShopByPhone(phone string) (ShopAccount, error) {
	return s.scanShop(s.db.QueryRow(
		`SELECT id, name, lat, lng, active, phone, password_hash, paid_through, payment_note
		 FROM shops WHERE phone = ?`, phone))
}

// ShopByID loads one shop account by ID.
func (s *Store) ShopByID(id string) (ShopAccount, error) {
	return s.scanShop(s.db.QueryRow(
		`SELECT id, name, lat, lng, active, phone, password_hash, paid_through, payment_note
		 FROM shops WHERE id = ?`, id))
}

func (s *Store) scanShop(row *sql.Row) (ShopAccount, error) {
	var a ShopAccount
	var active int
	err := row.Scan(&a.ID, &a.Name, &a.Lat, &a.Lng, &active, &a.Phone,
		&a.PasswordHash, &a.PaidThrough, &a.PaymentNote)
	if err != nil {
		return ShopAccount{}, err
	}
	a.Active = active == 1
	return a, nil
}

// RiderByPhone loads a rider by its login phone.
func (s *Store) RiderByPhone(phone string) (RiderAccount, error) {
	return s.scanRider(s.db.QueryRow(
		`SELECT id, name, lat, lng, available, phone, password_hash
		 FROM riders WHERE phone = ?`, phone))
}

// RiderByID loads one rider account by ID.
func (s *Store) RiderByID(id string) (RiderAccount, error) {
	return s.scanRider(s.db.QueryRow(
		`SELECT id, name, lat, lng, available, phone, password_hash
		 FROM riders WHERE id = ?`, id))
}

func (s *Store) scanRider(row *sql.Row) (RiderAccount, error) {
	var a RiderAccount
	var avail int
	err := row.Scan(&a.ID, &a.Name, &a.Lat, &a.Lng, &avail, &a.Phone, &a.PasswordHash)
	if err != nil {
		return RiderAccount{}, err
	}
	a.Available = avail == 1
	return a, nil
}

// SetShopCredentials writes a shop's login phone and password hash (the
// registration path, and password changes later).
func (s *Store) SetShopCredentials(id, phone, passwordHash string) error {
	_, err := s.db.Exec(
		`UPDATE shops SET phone = ?, password_hash = ? WHERE id = ?`, phone, passwordHash, id)
	if err != nil {
		return fmt.Errorf("store: shop credentials %s: %w", id, err)
	}
	return nil
}

// SetRiderCredentials writes a rider's login phone and password hash.
func (s *Store) SetRiderCredentials(id, phone, passwordHash string) error {
	_, err := s.db.Exec(
		`UPDATE riders SET phone = ?, password_hash = ? WHERE id = ?`, phone, passwordHash, id)
	if err != nil {
		return fmt.Errorf("store: rider credentials %s: %w", id, err)
	}
	return nil
}

// SetShopPaid marks a shop paid through the given month ("YYYY-MM") and
// records how the payment reached the admin (cash, EasyPaisa, JazzCash —
// free text, nothing is charged automatically).
func (s *Store) SetShopPaid(id, paidThrough, paymentNote string) error {
	_, err := s.db.Exec(
		`UPDATE shops SET paid_through = ?, payment_note = ? WHERE id = ?`, paidThrough, paymentNote, id)
	if err != nil {
		return fmt.Errorf("store: shop paid %s: %w", id, err)
	}
	return nil
}

// SetRiderLocation updates a rider's position (the dispatch board computes
// pickup distances from it) and their on-duty flag.
func (s *Store) SetRiderLocation(id string, lat, lng float64, available bool) error {
	_, err := s.db.Exec(
		`UPDATE riders SET lat = ?, lng = ?, available = ? WHERE id = ?`, lat, lng, boolInt(available), id)
	if err != nil {
		return fmt.Errorf("store: rider location %s: %w", id, err)
	}
	return nil
}

// StockAll returns the live on-shelf quantity of every product, keyed by
// product ID — the catalog overlay the public products endpoint serves.
func (s *Store) StockAll() (map[string]int64, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	rows, err := s.db.Query(`SELECT id, stock FROM products`)
	if err != nil {
		return nil, fmt.Errorf("store: stock: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store: scan stock: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// HasStock reports whether the shop still holds qty units of the product —
// the placement-time check before an order is ever written.
func (s *Store) HasStock(productID string, qty int64) (bool, error) {
	if !s.Enabled() {
		return false, ErrDisabled
	}
	var n int64
	if err := s.db.QueryRow(`SELECT stock FROM products WHERE id = ?`, productID).Scan(&n); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("store: has stock %s: %w", productID, err)
	}
	return n >= qty, nil
}

// ReserveStock decrements the ordered quantities inside one transaction —
// the accept path. Every line must clear its own guard (stock >= qty), so
// a short shelf fails the whole accept atomically and names the product.
func (s *Store) ReserveStock(items map[string]int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: reserve tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit
	for pid, qty := range items {
		res, err := tx.Exec(`UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?`, qty, pid, qty)
		if err != nil {
			return fmt.Errorf("store: reserve %s: %w", pid, err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("store: out of stock: %s", pid)
		}
	}
	return tx.Commit()
}

// ReleaseStock restores the ordered quantities (a confirmed order being
// cancelled must give the shelf its goods back).
func (s *Store) ReleaseStock(items map[string]int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: release tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit
	for pid, qty := range items {
		if _, err := tx.Exec(`UPDATE products SET stock = stock + ? WHERE id = ?`, qty, pid); err != nil {
			return fmt.Errorf("store: release %s: %w", pid, err)
		}
	}
	return tx.Commit()
}

// CreateSession stores a bearer-token session (hashed) with its expiry and
// sweeps expired rows on the way in — sessions clean themselves up.
func (s *Store) CreateSession(tokenHash, role, subject string, ttl time.Duration) error {
	now := time.Now().UTC()
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: session sweep: %w", err)
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, role, subject, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, role, subject, now.Add(ttl).Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: session create: %w", err)
	}
	return nil
}

// SessionSubject resolves a token hash to its role + subject; expired or
// unknown tokens resolve to a miss.
func (s *Store) SessionSubject(tokenHash string) (role, subject string, err error) {
	var exp string
	err = s.db.QueryRow(
		`SELECT role, subject, expires_at FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&role, &subject, &exp)
	if err != nil {
		return "", "", err
	}
	expAt, err := time.Parse(time.RFC3339Nano, exp)
	if err != nil {
		return "", "", fmt.Errorf("store: session expiry: %w", err)
	}
	if time.Now().UTC().After(expAt) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
		return "", "", sql.ErrNoRows
	}
	return role, subject, nil
}

// DeleteSession removes one session (logout).
func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("store: session delete: %w", err)
	}
	return nil
}
