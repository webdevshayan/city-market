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
      'shop_name': 'Central Market Halal Store',
      'rider_id': 'r-1',
      'rider_name': 'Rider 1',
      'items': [
        {'product_id': 'beef-1kg', 'qty': 2, 'unit_price_minor': 140000},
      ],
      'subtotal_minor': 280000,
      'delivery_fee_minor': 2500,
      'total_minor': 282500,
      'rider_payout_minor': 6000,
      'lat': 24.8607,
      'lng': 67.0011,
      'created_at': '2026-11-20T10:00:00Z',
    };

void main() {
  test('placeOrder posts the server shape and parses the 201', () async {
    http.Request? captured;
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      captured = req;
      return http.Response(jsonEncode(orderFixture('ord-1', statusPlaced)), 201);
    }));

    final o = await api.placeOrder([OrderDraftItem('beef-1kg', 2)], 24.8607, 67.0011);

    expect(captured!.method, 'POST');
    expect(captured!.url.path, '/v1/orders');
    final body = jsonDecode(captured!.body) as Map<String, dynamic>;
    expect(body['items'], hasLength(1));
    expect((body['items'] as List).first, containsPair('product_id', 'beef-1kg'));
    expect(body['lat'], 24.8607);
    expect(o.id, 'ord-1');
    expect(o.totalMinor, 282500);
  });

  test('server errors map to ApiException with the real message', () async {
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      return http.Response(
          jsonEncode({'error': 'no rider available right now'}), 409);
    }));

    await expectLater(
      api.placeOrder([OrderDraftItem('beef-1kg', 1)], 24.8, 67.0),
      throwsA(isA<ApiException>()
          .having((e) => e.statusCode, 'statusCode', 409)
          .having((e) => e.message, 'message', 'no rider available right now')),
    );
  });

  test('status transitions hit the right endpoints', () async {
    final paths = <String>[];
    final api = HttpApiClient('https://api.example.test', client: MockClient((req) async {
      paths.add(req.url.path);
      return http.Response(jsonEncode(orderFixture('ord-1', 'x')), 200);
    }));

    await api.confirmOrder('ord-1');
    await api.pickUpOrder('ord-1');
    await api.deliverOrder('ord-1');

    expect(paths, ['/v1/shop/orders/ord-1/confirm', '/v1/rider/orders/ord-1/pick-up', '/v1/rider/orders/ord-1/deliver']);
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
        id: 's-dha', name: 'DHA Store', lat: 24.825, lng: 67.07, active: true);
    await api.adminUpsertProduct('t',
        id: 'gajar-1kg',
        name: 'Gajar ka Halal Mix',
        category: 'grocery',
        brand: 'noor',
        size: '1kg',
        priceMinor: 50000,
        shopId: 's-dha');

    expect(requests[0].method, 'PUT');
    expect(requests[0].url.path, '/v1/admin/shops/s-dha');
    expect(jsonDecode(requests[0].body)['name'], 'DHA Store');
    expect(requests[1].url.path, '/v1/admin/products/gajar-1kg');
    expect(jsonDecode(requests[1].body)['price_minor'], 50000);
  });
}
