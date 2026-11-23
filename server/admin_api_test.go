package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webdevshayan/city-market/server/orders"
)

// adminDo performs an admin request with the X-Admin-Token header.
func adminDo(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Admin-Token", "test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, raw
}

// currentMonthString is the YYYY-MM the tests pay shops through.
func currentMonthString() string {
	return time.Now().UTC().Format("2006-01")
}

func TestAdminPayShopAndSubscriptions(t *testing.T) {
	srv, _, shopID := shopFlowServer(t)
	t.Setenv("ADMIN_TOKEN", "test-secret")

	// A wrong month format is refused loudly.
	code, m, _ := adminDo(t, srv, "PATCH", "/v1/admin/shops/"+shopID+"/paid",
		`{"paid_through":"november"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad month = %d %v, want 400", code, m)
	}
	// Beyond the two-year window is refused.
	code, _, _ = adminDo(t, srv, "PATCH", "/v1/admin/shops/"+shopID+"/paid",
		`{"paid_through":"2099-12"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("far-future month = %d, want 400", code)
	}
	// A valid month (this one) marks the shop eligible, with the payment
	// note recorded verbatim.
	code, m, _ = adminDo(t, srv, "PATCH", "/v1/admin/shops/"+shopID+"/paid",
		`{"paid_through":"`+currentMonthString()+`","payment_note":"easypaisa trx 9912"}`)
	if code != http.StatusOK || m["eligible"] != true || m["paid_through"] != currentMonthString() {
		t.Fatalf("pay = %d %v, want eligible=true", code, m)
	}
	if m["payment_note"] != "easypaisa trx 9912" {
		t.Fatalf("payment note = %v, want it stored verbatim", m["payment_note"])
	}

	// The subscriptions table lists every shop with its settlement columns.
	code, m, raw := adminDo(t, srv, "GET", "/v1/admin/subscriptions", "")
	if code != http.StatusOK {
		t.Fatalf("subscriptions = %d", code)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) < 4 { // 3 seeded shops + the registered one
		t.Fatalf("subscriptions = %d rows, want the seeded shops and the registered one", len(rows))
	}
	found := false
	for _, row := range rows {
		if row["id"] == shopID {
			found = true
			for _, field := range []string{"paid_through", "payment_note", "eligible", "month_orders", "month_sales_minor", "ujrah_minor", "net_minor"} {
				if _, ok := row[field]; !ok {
					t.Fatalf("subscription row missing %q: %v", field, row)
				}
			}
		}
	}
	if !found {
		t.Fatal("the paid shop must appear in the subscriptions table")
	}
}

func TestAdminStatsSeries(t *testing.T) {
	srv, _, _ := shopFlowServer(t)
	t.Setenv("ADMIN_TOKEN", "test-secret")

	// One placed order → it lands in today's bucket (GMV yes, payout no).
	resp, raw := postOrder(t, srv.URL, `{"items":[{"product_id":"beef-1kg","qty":1}],"lat":26.6931,"lng":67.6156}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("place = %d; body %s", resp.StatusCode, raw)
	}

	code, _, body := adminDo(t, srv, "GET", "/v1/admin/stats", "")
	if code != http.StatusOK {
		t.Fatalf("stats = %d", code)
	}
	var snap orders.StatsSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Series.Days) != 14 {
		t.Fatalf("series days = %d, want a 14-day window", len(snap.Series.Days))
	}
	if len(snap.Series.Orders) != 14 || len(snap.Series.GMVMinor) != 14 || len(snap.Series.PayoutMinor) != 14 {
		t.Fatal("every series must carry one bucket per day")
	}
	var sumOrders int64
	var sumGMV int64
	for i := range snap.Series.Orders {
		sumOrders += snap.Series.Orders[i]
		sumGMV += snap.Series.GMVMinor[i]
	}
	if sumOrders != snap.OrdersTotal {
		t.Fatalf("series order sum = %d, want the ledger total %d", sumOrders, snap.OrdersTotal)
	}
	if sumGMV != snap.GMVMinor {
		t.Fatalf("series GMV sum = %d, want the ledger total %d", sumGMV, snap.GMVMinor)
	}
	// The newest bucket is today and must hold the order just placed.
	if snap.Series.Orders[13] < 1 {
		t.Fatalf("today's bucket = %d orders, want at least the one just placed", snap.Series.Orders[13])
	}
}
