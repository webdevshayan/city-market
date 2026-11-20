/// API client for the City Market Go server. [ApiClient] is the interface
/// the apps depend on (so widget tests substitute fakes); [HttpApiClient]
/// is the real JSON-over-HTTP implementation.
library;

import 'dart:convert';

import 'package:core/models.dart';
import 'package:http/http.dart' as http;

/// One line of a citizen's order draft before pricing/assignment.
class OrderDraftItem {
  final String productId;
  final int qty;

  const OrderDraftItem(this.productId, this.qty);
}

/// Raised for any non-success server response, carrying the status code and
/// the server's error message so screens can show real reasons (e.g. the
/// 409 "no rider available right now").
class ApiException implements Exception {
  final int statusCode;
  final String message;

  ApiException(this.statusCode, this.message);

  @override
  String toString() => message;
}

abstract class ApiClient {
  Future<List<Product>> products();

  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng);
  Future<Order> order(String id);
  Future<List<Order>> shopOrders(String shopId);
  Future<List<Order>> riderOrders(String riderId);
  Future<Order> confirmOrder(String id);
  Future<Order> pickUpOrder(String id);
  Future<Order> deliverOrder(String id);

  Future<List<Place>> adminShops(String adminToken);
  Future<List<Place>> adminRiders(String adminToken);
  Future<List<Product>> adminProducts(String adminToken);
  Future<void> adminUpsertShop(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool active});
  Future<void> adminUpsertRider(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool available});
  Future<void> adminUpsertProduct(String adminToken,
      {required String id,
      required String name,
      required String category,
      required String brand,
      required String size,
      required int priceMinor,
      required String shopId});
  Future<CommissionView> commission(String adminToken, {required int orders, required int salesMinor});
}

class HttpApiClient implements ApiClient {
  final String baseUrl;
  final http.Client _http;

  HttpApiClient(this.baseUrl, {http.Client? client}) : _http = client ?? http.Client();

  // ——— citizen ———

  @override
  Future<List<Product>> products() async {
    final list = await _list('GET', '/api/v1/admin/products') as List<dynamic>;
    return list.map((e) => Product.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng) async {
    final body = jsonEncode({
      'items': [
        for (final it in items) {'product_id': it.productId, 'qty': it.qty},
      ],
      'lat': lat,
      'lng': lng,
    });
    return Order.fromJson(await _json('POST', '/api/v1/orders', body: body) as Map<String, dynamic>);
  }

  @override
  Future<Order> order(String id) async =>
      Order.fromJson(await _json('GET', '/api/v1/orders/$id') as Map<String, dynamic>);

  Future<List<Order>> _orders(String path) async {
    final list = await _list('GET', path) as List<dynamic>;
    return list.map((e) => Order.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<List<Order>> shopOrders(String shopId) async =>
      _orders('/api/v1/shop/orders?shop_id=${Uri.encodeQueryComponent(shopId)}');

  @override
  Future<List<Order>> riderOrders(String riderId) async =>
      _orders('/api/v1/rider/orders?rider_id=${Uri.encodeQueryComponent(riderId)}');

  @override
  Future<Order> confirmOrder(String id) async =>
      Order.fromJson(await _json('POST', '/api/v1/shop/orders/$id/confirm') as Map<String, dynamic>);

  @override
  Future<Order> pickUpOrder(String id) async =>
      Order.fromJson(await _json('POST', '/api/v1/rider/orders/$id/pick-up') as Map<String, dynamic>);

  @override
  Future<Order> deliverOrder(String id) async =>
      Order.fromJson(await _json('POST', '/api/v1/rider/orders/$id/deliver') as Map<String, dynamic>);

  // ——— admin console ———

  @override
  Future<List<Place>> adminShops(String adminToken) async {
    final list = await _list('GET', '/api/v1/admin/shops', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => Place.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<List<Place>> adminRiders(String adminToken) async {
    final list = await _list('GET', '/api/v1/admin/riders', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => Place.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<List<Product>> adminProducts(String adminToken) async {
    final list = await _list('GET', '/api/v1/admin/products', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => Product.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<void> adminUpsertShop(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool active}) async {
    await _json('PUT', '/api/v1/admin/shops/${Uri.encodeQueryComponent(id)}',
        adminToken: adminToken,
        body: jsonEncode({'name': name, 'lat': lat, 'lng': lng, 'active': active}));
  }

  @override
  Future<void> adminUpsertRider(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool available}) async {
    await _json('PUT', '/api/v1/admin/riders/${Uri.encodeQueryComponent(id)}',
        adminToken: adminToken,
        body: jsonEncode({'name': name, 'lat': lat, 'lng': lng, 'available': available}));
  }

  @override
  Future<void> adminUpsertProduct(String adminToken,
      {required String id,
      required String name,
      required String category,
      required String brand,
      required String size,
      required int priceMinor,
      required String shopId}) async {
    await _json('PUT', '/api/v1/admin/products/${Uri.encodeQueryComponent(id)}',
        adminToken: adminToken,
        body: jsonEncode({
          'name': name,
          'category': category,
          'brand': brand,
          'size': size,
          'price_minor': priceMinor,
          'shop_id': shopId,
        }));
  }

  @override
  Future<CommissionView> commission(String adminToken, {required int orders, required int salesMinor}) async {
    final j = await _json('GET', '/api/v1/admin/commission?orders=$orders&sales=$salesMinor',
        adminToken: adminToken) as Map<String, dynamic>;
    return CommissionView.fromJson(j);
  }

  // ——— plumbing ———

  Future<dynamic> _json(String method, String path, {String? body, String? adminToken}) async {
    final resp = await _send(method, path, body: body, adminToken: adminToken);
    return jsonDecode(utf8.decode(resp.bodyBytes));
  }

  Future<dynamic> _list(String method, String path, {String? adminToken}) async {
    final resp = await _send(method, path, adminToken: adminToken);
    return jsonDecode(utf8.decode(resp.bodyBytes));
  }

  Future<http.Response> _send(String method, String path, {String? body, String? adminToken}) async {
    final uri = Uri.parse('$baseUrl$path');
    final headers = <String, String>{'Content-Type': 'application/json'};
    if (adminToken != null) {
      headers['X-Admin-Token'] = adminToken;
    }
    final req = http.Request(method, uri)..headers.addAll(headers);
    if (body != null) {
      req.body = body;
    }
    final resp = await _http.send(req);
    final streamed = await http.Response.fromStream(resp);
    if (streamed.statusCode < 200 || streamed.statusCode >= 300) {
      var message = 'HTTP ${streamed.statusCode}';
      try {
        final j = jsonDecode(utf8.decode(streamed.bodyBytes));
        if (j is Map<String, dynamic> && j['error'] is String) {
          message = j['error'] as String;
        }
      } catch (_) {
        // keep the generic message when the body is not the server's error JSON
      }
      throw ApiException(streamed.statusCode, message);
    }
    return streamed;
  }
}
