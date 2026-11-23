import 'dart:convert';

import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

// A minimal order fixture mirroring the server's JSON.
Map<String, dynamic> orderFixture(String id, String status) => {
      'id': id,
      'status': status,
      'shop_id': 's-central',
      'shop_name': 'Madina Halal Store, Main Bazaar Johi',
      'rider_id': '',
      'rider_name': '',
      'items': [
        {'product_id': 'beef-1kg', 'qty': 2, 'unit_price_minor': 140000},
      ],
      'subtotal_minor': 280000,
      'delivery_fee_minor': 2500,
      'total_minor': 282500,
      'rider_payout_minor': 6000,
      'lat': 26.692061,
      'lng': 67.6133,
      'created_at': '2026-11-20T10:00:00Z',
      'address_note': 'Near the grain market, blue shutter',
    };

void main() {
  test('placeOrder posts the server shape and parses the 201', () async {
    http.Request? captured;
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      captured = req;
      return http.Response(jsonEncode(orderFixture('ord-1', statusPlaced)), 201);
    }));

    final o = await api.placeOrder([OrderDraftItem('beef-1kg', 2)], 26.692061, 67.6133,
        shopId: 's-central', addressNote: 'Near the grain market');

    expect(captured!.method, 'POST');
    expect(captured!.url.path, '/v1/orders');
    final body = jsonDecode(captured!.body) as Map<String, dynamic>;
    expect(body['items'], hasLength(1));
    expect((body['items'] as List).first, containsPair('product_id', 'beef-1kg'));
    expect(body['lat'], 26.692061);
    expect(body['shop_id'], 's-central');
    expect(body['address_note'], 'Near the grain market');
    expect(o.id, 'ord-1');
    expect(o.totalMinor, 282500);
    expect(o.addressNote, 'Near the grain market, blue shutter');
  });

  test('server errors map to ApiException with the real message', () async {
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      return http.Response(
          jsonEncode({'error': 'orders: not enough stock on the shelf'}), 409);
    }));

    await expectLater(
      api.placeOrder([OrderDraftItem('beef-1kg', 1)], 26.69, 67.61),
      throwsA(isA<ApiException>()
          .having((e) => e.statusCode, 'statusCode', 409)
          .having((e) => e.message, 'message', 'orders: not enough stock on the shelf')),
    );
  });

  test('marketplace steps hit the right bearer-gated endpoints', () async {
    final paths = <String>[];
    final auths = <String?>[];
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      paths.add(req.url.path);
      auths.add(req.headers['Authorization']);
      return http.Response(jsonEncode(orderFixture('ord-1', 'x')), 200);
    }));

    await api.acceptOrder('shop-token', 'ord-1');
    await api.claimOrder('rider-token', 'ord-1');
    await api.pickUpOrder('rider-token', 'ord-1');
    await api.deliverOrder('rider-token', 'ord-1');

    expect(paths, [
      '/v1/shop/orders/ord-1/accept',
      '/v1/rider/orders/ord-1/claim',
      '/v1/rider/orders/ord-1/pick-up',
      '/v1/rider/orders/ord-1/deliver',
    ]);
    expect(auths[0], 'Bearer shop-token');
    expect(auths[1], 'Bearer rider-token');
  });

  test('login posts the role and parses the account', () async {
    http.Request? captured;
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      captured = req;
      return http.Response(
          jsonEncode({
            'token': 'tok-1',
            'role': 'shop',
            'shop': {
              'id': 'sh-1',
              'name': 'Test Shelf',
              'lat': 26.6931,
              'lng': 67.6156,
              'active': true,
              'phone': '923211234567',
              'paid_through': '2026-11',
              'payment_note': 'cash',
              'eligible': true,
            },
          }),
          200);
    }));

    final r = await api.login(role: 'shop', phone: '0300-1234567', password: 'long-enough');

    expect(captured!.url.path, '/v1/auth/login');
    expect(jsonDecode(captured!.body)['role'], 'shop');
    expect(r.token, 'tok-1');
    expect(r.shop!.eligible, isTrue);
    expect(r.shop!.paidThrough, '2026-11');
  });

  test('the rider board parses every estimate field', () async {
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      expect(req.url.path, '/v1/rider/board');
      return http.Response(jsonEncode([
        {
          'order_id': 'ord-1',
          'shop_name': 'Test Shelf',
          'shop_lat': 26.6931,
          'shop_lng': 67.6156,
          'drop_lat': 26.7,
          'drop_lng': 67.65,
          'address_note': 'Near the grain market',
          'lines': 2,
          'total_minor': 142500,
          'pickup_km': 0.9,
          'route_km': 2.4,
          'eta_minutes': 14,
          'payout_minor': 96000,
        }
      ]), 200);
    }));

    final board = await api.riderBoard('rider-token');
    expect(board.single.orderId, 'ord-1');
    expect(board.single.etaMinutes, 14);
    expect(board.single.routeKm, 2.4);
    expect(board.single.payoutText, '960.00');
  });

  test('admin calls carry the token header', () async {
    final headers = <String>[];
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      headers.add(req.headers['X-Admin-Token'] ?? '(missing)');
      return http.Response('{"monthly_orders":250,"sales_minor":10000000,"commission_minor":700000,"commission_rate_bps":700,"commission_capped":false}', 200);
    }));

    final c = await api.commission('sekret', orders: 250, salesMinor: 10000000);

    expect(headers.single, 'sekret');
    expect(c.rateBps, 700);
  });

  test('admin upserts PUT the right paths and bodies', () async {
    final requests = <http.Request>[];
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      requests.add(req);
      return http.Response('{"id":"s-dha","ok":true}', 200);
    }));

    await api.adminUpsertShop('t',
        id: 's-dha', name: 'Dadu Road Mart', lat: 26.73, lng: 67.7769, active: true);
    await api.adminUpsertProduct('t',
        id: 'gajar-1kg',
        name: 'Gajar ka Halal Mix',
        category: 'grocery',
        brand: 'noor',
        size: '1kg',
        priceMinor: 50000,
        stock: 25,
        shopId: 's-dha');

    expect(requests[0].method, 'PUT');
    expect(requests[0].url.path, '/v1/admin/shops/s-dha');
    expect(jsonDecode(requests[0].body)['name'], 'Dadu Road Mart');
    expect(requests[1].url.path, '/v1/admin/products/gajar-1kg');
    expect(jsonDecode(requests[1].body)['stock'], 25);
  });

  test('admin pay-shop PATCHes the subscription and the table parses', () async {
    final requests = <http.Request>[];
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      requests.add(req);
      return http.Response(jsonEncode([
        {
          'id': 's-central',
          'name': 'Madina Halal Store',
          'active': true,
          'phone': '923001234567',
          'paid_through': '2026-11',
          'payment_note': 'easypaisa trx 9912',
          'eligible': true,
          'month_orders': 12,
          'month_sales_minor': 1680000,
          'ujrah_minor': 117600,
          'net_minor': 1562400,
        }
      ]), 200);
    }));

    await api.adminPayShop('t', id: 's-central', paidThrough: '2026-11', paymentNote: 'easypaisa trx 9912');
    final rows = await api.adminSubscriptions('t');

    expect(requests[0].method, 'PATCH');
    expect(requests[0].url.path, '/v1/admin/shops/s-central/paid');
    expect(jsonDecode(requests[0].body)['paid_through'], '2026-11');
    expect(rows.single.eligible, isTrue);
    expect(rows.single.ujrahMinor, 117600);
  });
}
