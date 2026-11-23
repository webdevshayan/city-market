/// Test double for [ApiClient]: canned fixtures plus recorded calls so the
/// apps' widget tests run with no server. Import from the apps' test files
/// — never from app code.
library;

import 'package:core/api.dart';
import 'package:core/models.dart';

class FakeApi implements ApiClient {
  FakeApi({
    List<Product>? products,
    Order? placedOrder,
    List<Order>? orders,
    List<Place>? shops,
    List<Place>? riders,
    CommissionView? commission,
    AdminStats? stats,
    ShopAccount? shopAccount,
    RiderAccount? riderAccount,
    List<BoardJob>? board,
    List<ShopSubscription>? subscriptions,
    AuthResult? auth,
  })  : products_ = products ??
            [
              const Product(
                  id: 'beef-1kg',
                  name: 'Halal Beef Boneless',
                  category: 'meat',
                  brand: 'al-safwa',
                  size: '1kg',
                  image: '',
                  shopId: 's-central',
                  priceMinor: 140000,
                  stock: 12),
            ],
        placedOrder = placedOrder ??
            Order(
                id: 'ord-test-1',
                status: statusPlaced,
                shopId: 's-central',
                shopName: 'Madina Halal Store — Main Bazaar Johi',
                riderId: '',
                riderName: '',
                items: const [
                  OrderItem(productId: 'beef-1kg', qty: 2, unitPriceMinor: 140000),
                ],
                subtotalMinor: 280000,
                deliveryFeeMinor: 2500,
                totalMinor: 282500,
                riderPayoutMinor: 6000,
                lat: 26.692061,
                lng: 67.6133,
                createdAt: DateTime.parse('2026-11-20T10:00:00Z'),
                addressNote: 'Near the grain market, blue shutter'),
        orders_ = orders ??
            [
              Order(
                  id: 'ord-shop-1',
                  status: statusPlaced,
                  shopId: 'sh-1',
                  shopName: 'Test Shelf',
                  riderId: '',
                  riderName: '',
                  items: const [
                    OrderItem(productId: 'beef-1kg', qty: 1, unitPriceMinor: 140000),
                  ],
                  subtotalMinor: 140000,
                  deliveryFeeMinor: 2500,
                  totalMinor: 142500,
                  riderPayoutMinor: 6000,
                  lat: 26.692061,
                  lng: 67.6133,
                  createdAt: DateTime.parse('2026-11-20T10:00:00Z'),
                  addressNote: 'Near the grain market, blue shutter'),
            ],
        shops_ = shops ??
            [
              const Place(id: 's-central', name: 'Madina Halal Store', lat: 26.6931, lng: 67.6156, active: true),
            ],
        riders_ = riders ??
            [
              const Place(id: 'r-1', name: 'Ali Raza', lat: 26.694, lng: 67.616, active: true),
            ],
        commission_ = commission ??
            const CommissionView(
                monthlyOrders: 250, salesMinor: 10000000, commissionMinor: 700000, rateBps: 700, capped: false),
        stats_ = stats ??
            const AdminStats(
              shopsTotal: 2,
              shopsActive: 2,
              shopsSelling: 1,
              ridersTotal: 3,
              ridersAvailable: 2,
              ridersPaid: 1,
              ordersTotal: 4,
              ordersByStatus: {'placed': 1, 'delivered': 2, 'cancelled': 1},
              gmvMinor: 465000,
              gmvMonthMinor: 465000,
              riderPayoutsMinor: 25500,
              platformUjrahMinor: 31100,
              shops: [
                ShopSettlement(
                    id: 's-central',
                    name: 'Madina Halal Store',
                    orders: 1,
                    salesMinor: 280000,
                    ujrahMinor: 0,
                    netMinor: 280000),
              ],
              riders: [
                RiderEarnings(id: 'r-1', name: 'Ali Raza', deliveries: 2, payoutMinor: 25500),
                RiderEarnings(id: 'r-2', name: 'Ghulam Hussain', deliveries: 0, payoutMinor: 0),
              ],
              series: DailySeries(
                days: ['2026-11-08', '2026-11-09'],
                orders: [2, 1],
                gmvMinor: [300000, 142500],
                payoutMinor: [12000, 6000],
              ),
            ),
        shopAccount_ = shopAccount ??
            const ShopAccount(
                id: 'sh-1',
                name: 'Test Shelf',
                lat: 26.6931,
                lng: 67.6156,
                active: true,
                phone: '923211234567',
                paidThrough: '2099-12',
                paymentNote: 'cash',
                eligible: true),
        riderAccount_ = riderAccount ??
            const RiderAccount(
                id: 'rd-1',
                name: 'Board Rider',
                lat: 26.694,
                lng: 67.616,
                available: true,
                phone: '923441234567'),
        board_ = board ??
            [
              const BoardJob(
                  orderId: 'ord-shop-1',
                  shopName: 'Test Shelf',
                  shopLat: 26.6931,
                  shopLng: 67.6156,
                  dropLat: 26.7,
                  dropLng: 67.65,
                  addressNote: 'Near the grain market, blue shutter',
                  lines: 1,
                  totalMinor: 142500,
                  pickupKm: 0.9,
                  routeKm: 2.4,
                  etaMinutes: 14,
                  payoutMinor: 96000),
            ],
        subscriptions_ = subscriptions ??
            [
              const ShopSubscription(
                  id: 's-central',
                  name: 'Madina Halal Store',
                  active: true,
                  phone: '923001234567',
                  paidThrough: '',
                  paymentNote: '',
                  eligible: false,
                  monthOrders: 1,
                  monthSalesMinor: 280000,
                  ujrahMinor: 0,
                  netMinor: 280000),
            ],
        auth_ = auth ??
            const AuthResult(
                token: 'token-fixture', role: 'shop', shop: null, rider: null);

  final List<Product> products_;
  final Order placedOrder;
  final List<Order> orders_;
  final List<Place> shops_;
  final List<Place> riders_;
  final CommissionView commission_;
  final AdminStats stats_;
  final ShopAccount shopAccount_;
  RiderAccount riderAccount_;
  final List<BoardJob> board_;
  final List<ShopSubscription> subscriptions_;
  final AuthResult auth_;

  // Recorded calls the widget tests assert against.
  final placeOrderCalls = <List<OrderDraftItem>>[];
  final placeOrderNotes = <String>[];
  final placeOrderShopIds = <String?>[];
  final confirmCalls = <String>[];
  final pickUpCalls = <String>[];
  final deliverCalls = <String>[];
  final acceptCalls = <String>[];
  final cancelCalls = <String>[];
  final claimCalls = <String>[];
  final upsertShopCalls = <Map<String, Object?>>[];
  final upsertRiderCalls = <Map<String, Object?>>[];
  final upsertProductCalls = <Map<String, Object?>>[];
  final payShopCalls = <Map<String, Object?>>[];
  final createProductCalls = <Map<String, Object?>>[];
  final updateProductCalls = <Map<String, Object?>>[];
  final setMeCalls = <Map<String, Object?>>[];

  Order _withStatus(Order o, String status) => Order(
      id: o.id,
      status: status,
      shopId: o.shopId,
      shopName: o.shopName,
      riderId: o.riderId,
      riderName: o.riderName,
      items: o.items,
      subtotalMinor: o.subtotalMinor,
      deliveryFeeMinor: o.deliveryFeeMinor,
      totalMinor: o.totalMinor,
      riderPayoutMinor: o.riderPayoutMinor,
      lat: o.lat,
      lng: o.lng,
      createdAt: o.createdAt,
      addressNote: o.addressNote,
      estRouteMeters: o.estRouteMeters,
      claimedAt: o.claimedAt);

  // ——— citizen ———

  @override
  Future<List<Product>> products() async => products_;

  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng,
      {String? shopId, String addressNote = ''}) async {
    placeOrderCalls.add(items);
    placeOrderNotes.add(addressNote);
    placeOrderShopIds.add(shopId);
    return placedOrder;
  }

  @override
  Future<Order> order(String id) async => placedOrder;

  // ——— accounts ———

  @override
  Future<AuthResult> registerShop(
      {required String name, required String phone, required String password, required double lat, required double lng}) async =>
      auth_;

  @override
  Future<AuthResult> registerRider(
      {required String name, required String phone, required String password, required double lat, required double lng}) async =>
      auth_;

  @override
  Future<AuthResult> login({required String role, required String phone, required String password}) async => auth_;

  @override
  Future<void> logout(String token) async {}

  // ——— shop owner ———

  @override
  Future<ShopAccount> shopMe(String token) async => shopAccount_;

  @override
  Future<List<Product>> shopProducts(String token) async => products_;

  @override
  Future<String> shopCreateProduct(String token,
      {required String name, required String category, required String brand, required String size,
      required int priceMinor, required int stock, String image = ''}) async {
    createProductCalls.add({
      'name': name, 'category': category, 'brand': brand, 'size': size,
      'price_minor': priceMinor, 'stock': stock,
    });
    return 'p-new-1';
  }

  @override
  Future<void> shopUpdateProduct(String token,
      {required String id, required String name, required String category, required String brand,
      required String size, required int priceMinor, required int stock, String image = ''}) async {
    updateProductCalls.add({
      'id': id, 'name': name, 'category': category, 'brand': brand, 'size': size,
      'price_minor': priceMinor, 'stock': stock,
    });
  }

  @override
  Future<List<Order>> shopOrders(String token) async => orders_;

  @override
  Future<Order> acceptOrder(String token, String id) async {
    acceptCalls.add(id);
    _apply(id, statusConfirmed);
    return _withStatus(placedOrder, statusConfirmed);
  }

  @override
  Future<Order> cancelOrder(String token, String id) async {
    cancelCalls.add(id);
    _apply(id, statusCancelled);
    return _withStatus(placedOrder, statusCancelled);
  }

  // ——— rider ———

  @override
  Future<RiderAccount> riderMe(String token) async => riderAccount_;

  @override
  Future<void> riderSetMe(String token, {required double lat, required double lng, required bool available}) async {
    setMeCalls.add({'lat': lat, 'lng': lng, 'available': available});
    // The real backend updates the rider row; the fake must too, or the
    // duty banner keeps showing the stale flag after the reload.
    riderAccount_ = RiderAccount(
        id: riderAccount_.id,
        name: riderAccount_.name,
        lat: lat,
        lng: lng,
        available: available,
        phone: riderAccount_.phone);
  }

  @override
  Future<List<BoardJob>> riderBoard(String token) async => board_;

  @override
  Future<List<Order>> riderJobs(String token) async => orders_;

  @override
  Future<Order> claimOrder(String token, String id) async {
    claimCalls.add(id);
    _apply(id, statusConfirmed);
    return _withStatus(placedOrder, statusConfirmed);
  }

  @override
  Future<Order> pickUpOrder(String token, String id) async {
    pickUpCalls.add(id);
    _apply(id, statusPickedUp);
    return _withStatus(placedOrder, statusPickedUp);
  }

  @override
  Future<Order> deliverOrder(String token, String id) async {
    deliverCalls.add(id);
    _apply(id, statusDelivered);
    return _withStatus(placedOrder, statusDelivered);
  }

  /// Reflects a status change in the canned list, the way the real backend
  /// would — so a screen that reloads after advancing sees the new status.
  void applyStatus(String id, String status) => _apply(id, status);

  void _apply(String id, String status) {
    for (var i = 0; i < orders_.length; i++) {
      if (orders_[i].id == id) {
        orders_[i] = _withStatus(orders_[i], status);
      }
    }
  }

  // ——— admin ———

  @override
  Future<List<Place>> adminShops(String adminToken) async => shops_;

  @override
  Future<List<Place>> adminRiders(String adminToken) async => riders_;

  @override
  Future<AdminStats> adminStats(String adminToken) async => stats_;

  @override
  Future<List<Product>> adminProducts(String adminToken) async => products_;

  @override
  Future<void> adminUpsertShop(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool active}) async {
    upsertShopCalls.add({'id': id, 'name': name, 'lat': lat, 'lng': lng, 'active': active});
  }

  @override
  Future<void> adminUpsertRider(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool available}) async {
    upsertRiderCalls.add({'id': id, 'name': name, 'lat': lat, 'lng': lng, 'available': available});
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
    upsertProductCalls.add({
      'id': id, 'name': name, 'category': category, 'brand': brand, 'size': size,
      'price_minor': priceMinor, 'stock': stock, 'shop_id': shopId,
    });
  }

  @override
  Future<CommissionView> commission(String adminToken, {required int orders, required int salesMinor}) async =>
      commission_;

  @override
  Future<void> adminPayShop(String adminToken,
      {required String id, required String paidThrough, String paymentNote = ''}) async {
    payShopCalls.add({'id': id, 'paid_through': paidThrough, 'payment_note': paymentNote});
  }

  @override
  Future<List<ShopSubscription>> adminSubscriptions(String adminToken) async => subscriptions_;
}
