/// API client for the City Market Go server. [ApiClient] is the interface
/// the apps depend on (so widget tests substitute fakes); [HttpApiClient]
/// is the real JSON-over-HTTP implementation. Shop and rider surfaces take
/// the bearer token explicitly — screens own their session, the client
/// stays stateless.
library;

import 'dart:async';
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
/// 409 "another rider claimed this job").
class ApiException implements Exception {
  final int statusCode;
  final String message;

  ApiException(this.statusCode, this.message);

  @override
  String toString() => message;
}

/// The outcome of register/login: a bearer token plus the signed-in
/// account. Both roles answer the same shape ("account"), so the login
/// screens share one parser.
class AuthResult {
  final String token;
  final String role;
  final ShopAccount? shop;
  final RiderAccount? rider;

  const AuthResult({required this.token, required this.role, this.shop, this.rider});

  factory AuthResult.fromJson(Map<String, dynamic> j) => AuthResult(
        token: j['token'] as String,
        role: j['role'] as String,
        shop: j['shop'] is Map<String, dynamic>
            ? ShopAccount.fromJson(j['shop'] as Map<String, dynamic>)
            : null,
        rider: j['rider'] is Map<String, dynamic>
            ? RiderAccount.fromJson(j['rider'] as Map<String, dynamic>)
            : null,
      );
}

abstract class ApiClient {
  // ——— citizen ———
  Future<List<Product>> products();
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng,
      {String? shopId, String addressNote = ''});
  Future<Order> order(String id);

  // ——— accounts ———
  Future<AuthResult> registerShop(
      {required String name, required String phone, required String password, required double lat, required double lng});
  Future<AuthResult> registerRider(
      {required String name, required String phone, required String password, required double lat, required double lng});
  Future<AuthResult> login({required String role, required String phone, required String password});
  Future<void> logout(String token);

  // ——— shop owner ———
  Future<ShopAccount> shopMe(String token);
  Future<List<Product>> shopProducts(String token);
  Future<String> shopCreateProduct(String token,
      {required String name, required String category, required String brand, required String size,
      required int priceMinor, required int stock, String image = ''});
  Future<void> shopUpdateProduct(String token,
      {required String id, required String name, required String category, required String brand,
      required String size, required int priceMinor, required int stock, String image = ''});
  Future<List<Order>> shopOrders(String token);
  Future<Order> acceptOrder(String token, String id);
  Future<Order> cancelOrder(String token, String id);

  // ——— rider ———
  Future<RiderAccount> riderMe(String token);
  Future<void> riderSetMe(String token, {required double lat, required double lng, required bool available});
  Future<List<BoardJob>> riderBoard(String token);
  Future<List<Order>> riderJobs(String token);
  Future<Order> claimOrder(String token, String id);
  Future<Order> pickUpOrder(String token, String id);
  Future<Order> deliverOrder(String token, String id);

  // ——— admin console ———
  Future<List<Place>> adminShops(String adminToken);
  Future<List<Place>> adminRiders(String adminToken);
  Future<AdminStats> adminStats(String adminToken);
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
      required int stock,
      required String shopId});
  Future<CommissionView> commission(String adminToken, {required int orders, required int salesMinor});
  Future<void> adminPayShop(String adminToken,
      {required String id, required String paidThrough, String paymentNote = ''});
  Future<List<ShopSubscription>> adminSubscriptions(String adminToken);
}

class HttpApiClient implements ApiClient {
  final String baseUrl;
  final http.Client _http;

  /// Hard ceiling on any request, headers to body. The OS default for an
  /// unreachable host can stall a request for minutes — an app that points
  /// at a dead server must surface a clear error in seconds, not spin.
  static const _timeout = Duration(seconds: 20);

  HttpApiClient(this.baseUrl, {http.Client? client}) : _http = client ?? http.Client();

  // ——— citizen ———

  @override
  Future<List<Product>> products() async {
    final list = await _list('GET', '/v1/products') as List<dynamic>;
    return list.map((e) => Product.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng,
      {String? shopId, String addressNote = ''}) async {
    final body = jsonEncode({
      'items': [
        for (final it in items) {'product_id': it.productId, 'qty': it.qty},
      ],
      'lat': lat,
      'lng': lng,
      if (shopId != null && shopId.isNotEmpty) 'shop_id': shopId,
      'address_note': addressNote,
    });
    return Order.fromJson(await _json('POST', '/v1/orders', body: body) as Map<String, dynamic>);
  }

  @override
  Future<Order> order(String id) async =>
      Order.fromJson(await _json('GET', '/v1/orders/$id') as Map<String, dynamic>);

  // ——— accounts ———

  @override
  Future<AuthResult> registerShop(
      {required String name, required String phone, required String password, required double lat, required double lng}) async {
    return AuthResult.fromJson(await _json('POST', '/v1/auth/shop/register',
        body: jsonEncode({'name': name, 'phone': phone, 'password': password, 'lat': lat, 'lng': lng}))
        as Map<String, dynamic>);
  }

  @override
  Future<AuthResult> registerRider(
      {required String name, required String phone, required String password, required double lat, required double lng}) async {
    return AuthResult.fromJson(await _json('POST', '/v1/auth/rider/register',
        body: jsonEncode({'name': name, 'phone': phone, 'password': password, 'lat': lat, 'lng': lng}))
        as Map<String, dynamic>);
  }

  @override
  Future<AuthResult> login({required String role, required String phone, required String password}) async {
    return AuthResult.fromJson(await _json('POST', '/v1/auth/login',
        body: jsonEncode({'role': role, 'phone': phone, 'password': password}))
        as Map<String, dynamic>);
  }

  @override
  Future<void> logout(String token) async {
    await _json('POST', '/v1/auth/logout', bearer: token);
  }

  // ——— shop owner ———

  @override
  Future<ShopAccount> shopMe(String token) async =>
      ShopAccount.fromJson(await _json('GET', '/v1/shop/me', bearer: token) as Map<String, dynamic>);

  @override
  Future<List<Product>> shopProducts(String token) async {
    final list = await _list('GET', '/v1/shop/products', bearer: token) as List<dynamic>;
    return list.map((e) => Product.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<String> shopCreateProduct(String token,
      {required String name, required String category, required String brand, required String size,
      required int priceMinor, required int stock, String image = ''}) async {
    final j = await _json('POST', '/v1/shop/products',
        bearer: token,
        body: jsonEncode(_productBody(name, category, brand, size, priceMinor, stock, image)))
        as Map<String, dynamic>;
    return j['id'] as String;
  }

  @override
  Future<void> shopUpdateProduct(String token,
      {required String id, required String name, required String category, required String brand,
      required String size, required int priceMinor, required int stock, String image = ''}) async {
    await _json('PUT', '/v1/shop/products/$id',
        bearer: token,
        body: jsonEncode(_productBody(name, category, brand, size, priceMinor, stock, image)));
  }

  Map<String, dynamic> _productBody(String name, String category, String brand, String size,
          int priceMinor, int stock, String image) =>
      {
        'name': name,
        'category': category,
        'brand': brand,
        'size': size,
        'price_minor': priceMinor,
        'stock': stock,
        'image': image,
      };

  @override
  Future<List<Order>> shopOrders(String token) async =>
      _orders('GET', '/v1/shop/orders', bearer: token);

  @override
  Future<Order> acceptOrder(String token, String id) async =>
      Order.fromJson(await _json('POST', '/v1/shop/orders/$id/accept', bearer: token) as Map<String, dynamic>);

  @override
  Future<Order> cancelOrder(String token, String id) async =>
      Order.fromJson(await _json('POST', '/v1/shop/orders/$id/cancel', bearer: token) as Map<String, dynamic>);

  // ——— rider ———

  @override
  Future<RiderAccount> riderMe(String token) async =>
      RiderAccount.fromJson(await _json('GET', '/v1/rider/me', bearer: token) as Map<String, dynamic>);

  @override
  Future<void> riderSetMe(String token, {required double lat, required double lng, required bool available}) async {
    await _json('POST', '/v1/rider/me',
        bearer: token, body: jsonEncode({'lat': lat, 'lng': lng, 'available': available}));
  }

  @override
  Future<List<BoardJob>> riderBoard(String token) async {
    final list = await _list('GET', '/v1/rider/board', bearer: token) as List<dynamic>;
    return list.map((e) => BoardJob.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<List<Order>> riderJobs(String token) async =>
      _orders('GET', '/v1/rider/orders', bearer: token);

  @override
  Future<Order> claimOrder(String token, String id) async =>
      Order.fromJson(await _json('POST', '/v1/rider/orders/$id/claim', bearer: token) as Map<String, dynamic>);

  @override
  Future<Order> pickUpOrder(String token, String id) async =>
      Order.fromJson(await _json('POST', '/v1/rider/orders/$id/pick-up', bearer: token) as Map<String, dynamic>);

  @override
  Future<Order> deliverOrder(String token, String id) async =>
      Order.fromJson(await _json('POST', '/v1/rider/orders/$id/deliver', bearer: token) as Map<String, dynamic>);

  Future<List<Order>> _orders(String method, String path, {String? bearer}) async {
    final list = await _list(method, path, bearer: bearer) as List<dynamic>;
    return list.map((e) => Order.fromJson(e as Map<String, dynamic>)).toList();
  }

  // ——— admin console ———

  @override
  Future<List<Place>> adminShops(String adminToken) async {
    final list = await _list('GET', '/v1/admin/shops', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => Place.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<List<Place>> adminRiders(String adminToken) async {
    final list = await _list('GET', '/v1/admin/riders', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => Place.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<AdminStats> adminStats(String adminToken) async => AdminStats.fromJson(
      await _json('GET', '/v1/admin/stats', adminToken: adminToken) as Map<String, dynamic>);

  @override
  Future<List<Product>> adminProducts(String adminToken) async {
    final list = await _list('GET', '/v1/admin/products', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => Product.fromJson(e as Map<String, dynamic>)).toList();
  }

  @override
  Future<void> adminUpsertShop(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool active}) async {
    await _json('PUT', '/v1/admin/shops/${Uri.encodeQueryComponent(id)}',
        adminToken: adminToken,
        body: jsonEncode({'name': name, 'lat': lat, 'lng': lng, 'active': active}));
  }

  @override
  Future<void> adminUpsertRider(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool available}) async {
    await _json('PUT', '/v1/admin/riders/${Uri.encodeQueryComponent(id)}',
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
      required int stock,
      required String shopId}) async {
    await _json('PUT', '/v1/admin/products/${Uri.encodeQueryComponent(id)}',
        adminToken: adminToken,
        body: jsonEncode({
          'name': name,
          'category': category,
          'brand': brand,
          'size': size,
          'price_minor': priceMinor,
          'stock': stock,
          'shop_id': shopId,
        }));
  }

  @override
  Future<CommissionView> commission(String adminToken, {required int orders, required int salesMinor}) async {
    final j = await _json('GET', '/v1/admin/commission?orders=$orders&sales=$salesMinor',
        adminToken: adminToken) as Map<String, dynamic>;
    return CommissionView.fromJson(j);
  }

  @override
  Future<void> adminPayShop(String adminToken,
      {required String id, required String paidThrough, String paymentNote = ''}) async {
    await _json('PATCH', '/v1/admin/shops/${Uri.encodeQueryComponent(id)}/paid',
        adminToken: adminToken,
        body: jsonEncode({'paid_through': paidThrough, 'payment_note': paymentNote}));
  }

  @override
  Future<List<ShopSubscription>> adminSubscriptions(String adminToken) async {
    final list = await _list('GET', '/v1/admin/subscriptions', adminToken: adminToken) as List<dynamic>;
    return list.map((e) => ShopSubscription.fromJson(e as Map<String, dynamic>)).toList();
  }

  // ——— plumbing ———

  Future<dynamic> _json(String method, String path, {String? body, String? adminToken, String? bearer}) async {
    final resp = await _send(method, path, body: body, adminToken: adminToken, bearer: bearer);
    return jsonDecode(utf8.decode(resp.bodyBytes));
  }

  Future<dynamic> _list(String method, String path, {String? adminToken, String? bearer}) async {
    final resp = await _send(method, path, adminToken: adminToken, bearer: bearer);
    return jsonDecode(utf8.decode(resp.bodyBytes));
  }

  Future<http.Response> _send(String method, String path,
      {String? body, String? adminToken, String? bearer}) async {
    final uri = Uri.parse('$baseUrl$path');
    final headers = <String, String>{'Content-Type': 'application/json'};
    if (adminToken != null) {
      headers['X-Admin-Token'] = adminToken;
    }
    if (bearer != null && bearer.isNotEmpty) {
      headers['Authorization'] = 'Bearer $bearer';
    }
    final req = http.Request(method, uri)..headers.addAll(headers);
    if (body != null) {
      req.body = body;
    }
    final http.Response streamed;
    try {
      final resp = await _http.send(req).timeout(_timeout);
      streamed = await http.Response.fromStream(resp).timeout(_timeout);
    } on TimeoutException {
      throw ApiException(0,
          'The market server did not respond within 20 s — check your internet connection and try again.');
    }
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
