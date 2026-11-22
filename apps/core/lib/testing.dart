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
                  priceMinor: 140000),
            ],
        placedOrder = placedOrder ??
            Order(
                id: 'ord-test-1',
                status: statusPlaced,
                shopId: 's-central',
                shopName: 'Central Market Halal Store',
                riderId: 'r-1',
                riderName: 'Rider 1',
                items: const [
                  OrderItem(productId: 'beef-1kg', qty: 2, unitPriceMinor: 140000),
                ],
                subtotalMinor: 280000,
                deliveryFeeMinor: 2500,
                totalMinor: 282500,
                riderPayoutMinor: 6000,
                lat: 24.8607,
                lng: 67.0011,
                createdAt: DateTime.parse('2026-11-20T10:00:00Z')),
        orders_ = orders ??
            [
              Order(
                  id: 'ord-shop-1',
                  status: statusPlaced,
                  shopId: 's-central',
                  shopName: 'Central Market Halal Store',
                  riderId: 'r-1',
                  riderName: 'Rider 1',
                  items: const [
                    OrderItem(productId: 'beef-1kg', qty: 1, unitPriceMinor: 140000),
                  ],
                  subtotalMinor: 140000,
                  deliveryFeeMinor: 2500,
                  totalMinor: 142500,
                  riderPayoutMinor: 6000,
                  lat: 24.8607,
                  lng: 67.0011,
                  createdAt: DateTime.parse('2026-11-20T10:00:00Z')),
            ],
        shops_ = shops ??
            [
              const Place(id: 's-central', name: 'Central Market Halal Store', lat: 24.8607, lng: 67.0011, active: true),
            ],
        riders_ = riders ??
            [
              const Place(id: 'r-1', name: 'Rider 1', lat: 24.87, lng: 67.01, active: true),
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
                    name: 'Central Market Halal Store',
                    orders: 1,
                    salesMinor: 280000,
                    ujrahMinor: 0,
                    netMinor: 280000),
              ],
              riders: [
                RiderEarnings(id: 'r-1', name: 'Rider 1', deliveries: 2, payoutMinor: 25500),
                RiderEarnings(id: 'r-2', name: 'Rider 2', deliveries: 0, payoutMinor: 0),
              ],
            );

  final List<Product> products_;
  final Order placedOrder;
  final List<Order> orders_;
  final List<Place> shops_;
  final List<Place> riders_;
  final CommissionView commission_;
  final AdminStats stats_;

  // Recorded calls the widget tests assert against.
  final placeOrderCalls = <List<OrderDraftItem>>[];
  final confirmCalls = <String>[];
  final pickUpCalls = <String>[];
  final deliverCalls = <String>[];
  final upsertShopCalls = <Map<String, Object?>>[];
  final upsertRiderCalls = <Map<String, Object?>>[];
  final upsertProductCalls = <Map<String, Object?>>[];

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
      createdAt: o.createdAt);

  @override
  Future<List<Product>> products() async => products_;

  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng) async {
    placeOrderCalls.add(items);
    return placedOrder;
  }

  @override
  Future<Order> order(String id) async => placedOrder;

  @override
  Future<List<Order>> shopOrders(String shopId) async => orders_;

  @override
  Future<List<Order>> riderOrders(String riderId) async => orders_;

  @override
  Future<Order> confirmOrder(String id) async {
    confirmCalls.add(id);
    _apply(id, statusConfirmed);
    return _withStatus(placedOrder, statusConfirmed);
  }

  @override
  Future<Order> pickUpOrder(String id) async {
    pickUpCalls.add(id);
    _apply(id, statusPickedUp);
    return _withStatus(placedOrder, statusPickedUp);
  }

  @override
  Future<Order> deliverOrder(String id) async {
    deliverCalls.add(id);
    _apply(id, statusDelivered);
    return _withStatus(placedOrder, statusDelivered);
  }

  /// Reflects the transition in the canned list, the way the real backend
  /// would — so a screen that reloads after advancing sees the new status.
  void _apply(String id, String status) {
    for (var i = 0; i < orders_.length; i++) {
      if (orders_[i].id == id) {
        orders_[i] = _withStatus(orders_[i], status);
      }
    }
  }

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
      required String shopId}) async {
    upsertProductCalls.add(
        {'id': id, 'name': name, 'category': category, 'brand': brand, 'size': size, 'price_minor': priceMinor, 'shop_id': shopId});
  }

  @override
  Future<CommissionView> commission(String adminToken, {required int orders, required int salesMinor}) async =>
      commission_;
}
