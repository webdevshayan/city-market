/// Shared data models for the City Market apps. JSON shapes mirror the Go
/// server exactly: snake_case keys, money as int64 minor units
/// (1 major unit = 100 minor units), and no business logic here — the
/// server (server/finance, server/orders) is the single source of truth.
library;

/// One catalog listing, as served by GET /v1/products — the public catalog
/// the citizen app browses. Stock is the shop's live on-shelf quantity
/// (decremented when the shop accepts an order).
class Product {
  final String id;
  final String name;
  final String category;
  final String brand;
  final String size;
  final String image;
  final String shopId;
  final int priceMinor;
  final int stock;

  const Product({
    required this.id,
    required this.name,
    required this.category,
    required this.brand,
    required this.size,
    required this.image,
    required this.shopId,
    required this.priceMinor,
    this.stock = 0,
  });

  factory Product.fromJson(Map<String, dynamic> j) => Product(
        id: j['id'] as String,
        name: j['name'] as String,
        category: j['category'] as String,
        brand: j['brand'] as String,
        size: j['size'] as String,
        image: j['image'] as String? ?? '',
        shopId: j['shop_id'] as String? ?? '',
        priceMinor: (j['price_minor'] as num).toInt(),
        stock: (j['stock'] as num?)?.toInt() ?? 0,
      );

  /// Price rendered as "major.minor", matching the server's PriceText.
  String get priceText => money(priceMinor);

  bool get inStock => stock > 0;
}

/// One order line, with the price snapshot taken at placement time.
class OrderItem {
  final String productId;
  final int qty;
  final int unitPriceMinor;

  const OrderItem({required this.productId, required this.qty, required this.unitPriceMinor});

  factory OrderItem.fromJson(Map<String, dynamic> j) => OrderItem(
        productId: j['product_id'] as String,
        qty: (j['qty'] as num).toInt(),
        unitPriceMinor: (j['unit_price_minor'] as num).toInt(),
      );

  Map<String, dynamic> toJson() =>
      {'product_id': productId, 'qty': qty, 'unit_price_minor': unitPriceMinor};
}

/// Order lifecycle statuses served by the Go engine (server/orders).
const statusPlaced = 'placed';
const statusConfirmed = 'confirmed';
const statusPickedUp = 'picked_up';
const statusDelivered = 'delivered';
const statusCancelled = 'cancelled';

/// A persisted order with its fee breakdown and dispatch assignment.
class Order {
  final String id;
  final String status;
  final String shopId;
  final String shopName;
  final String riderId;
  final String riderName;
  final List<OrderItem> items;
  final int subtotalMinor;
  final int deliveryFeeMinor;
  final int totalMinor;
  final int riderPayoutMinor;
  final double lat;
  final double lng;
  final DateTime createdAt;
  final String addressNote;
  final int estRouteMeters;
  final DateTime? claimedAt;

  const Order({
    required this.id,
    required this.status,
    required this.shopId,
    required this.shopName,
    required this.riderId,
    required this.riderName,
    required this.items,
    required this.subtotalMinor,
    required this.deliveryFeeMinor,
    required this.totalMinor,
    required this.riderPayoutMinor,
    required this.lat,
    required this.lng,
    required this.createdAt,
    this.addressNote = '',
    this.estRouteMeters = 0,
    this.claimedAt,
  });

  factory Order.fromJson(Map<String, dynamic> j) => Order(
        id: j['id'] as String,
        status: j['status'] as String,
        shopId: j['shop_id'] as String,
        shopName: j['shop_name'] as String? ?? j['shop_id'] as String,
        riderId: j['rider_id'] as String? ?? '',
        riderName: j['rider_name'] as String? ?? j['rider_id'] as String? ?? '',
        items: (j['items'] as List<dynamic>? ?? [])
            .map((e) => OrderItem.fromJson(e as Map<String, dynamic>))
            .toList(),
        subtotalMinor: (j['subtotal_minor'] as num).toInt(),
        deliveryFeeMinor: (j['delivery_fee_minor'] as num).toInt(),
        totalMinor: (j['total_minor'] as num).toInt(),
        riderPayoutMinor: (j['rider_payout_minor'] as num).toInt(),
        lat: (j['lat'] as num).toDouble(),
        lng: (j['lng'] as num).toDouble(),
        createdAt: DateTime.parse(j['created_at'] as String),
        addressNote: j['address_note'] as String? ?? '',
        estRouteMeters: (j['est_route_m'] as num?)?.toInt() ?? 0,
        claimedAt: j['claimed_at'] is String && (j['claimed_at'] as String).isNotEmpty
            ? DateTime.parse(j['claimed_at'] as String)
            : null,
      );
}

/// A shop owner's account, as served by registration, login and
/// GET /v1/shop/me. Eligible means open AND paid through the running
/// month — the admin marks the monthly payment manually.
class ShopAccount {
  final String id;
  final String name;
  final double lat;
  final double lng;
  final bool active;
  final String phone;
  final String paidThrough;
  final String paymentNote;
  final bool eligible;

  const ShopAccount({
    required this.id,
    required this.name,
    required this.lat,
    required this.lng,
    required this.active,
    required this.phone,
    required this.paidThrough,
    required this.paymentNote,
    required this.eligible,
  });

  factory ShopAccount.fromJson(Map<String, dynamic> j) => ShopAccount(
        id: j['id'] as String,
        name: j['name'] as String,
        lat: (j['lat'] as num).toDouble(),
        lng: (j['lng'] as num).toDouble(),
        active: j['active'] as bool? ?? false,
        phone: j['phone'] as String? ?? '',
        paidThrough: j['paid_through'] as String? ?? '',
        paymentNote: j['payment_note'] as String? ?? '',
        eligible: j['eligible'] as bool? ?? false,
      );
}

/// A rider's account.
class RiderAccount {
  final String id;
  final String name;
  final double lat;
  final double lng;
  final bool available;
  final String phone;

  const RiderAccount({
    required this.id,
    required this.name,
    required this.lat,
    required this.lng,
    required this.available,
    required this.phone,
  });

  factory RiderAccount.fromJson(Map<String, dynamic> j) => RiderAccount(
        id: j['id'] as String,
        name: j['name'] as String,
        lat: (j['lat'] as num).toDouble(),
        lng: (j['lng'] as num).toDouble(),
        available: j['available'] as bool? ?? false,
        phone: j['phone'] as String? ?? '',
      );
}

/// One open job on the rider dispatch board — the full decision set the
/// server computes before the rider commits: where to pick up, where to
/// deliver, how far and how long, what it pays.
class BoardJob {
  final String orderId;
  final String shopName;
  final double shopLat;
  final double shopLng;
  final double dropLat;
  final double dropLng;
  final String addressNote;
  final int lines;
  final int totalMinor;
  final double pickupKm;
  final double routeKm;
  final int etaMinutes;
  final int payoutMinor;

  const BoardJob({
    required this.orderId,
    required this.shopName,
    required this.shopLat,
    required this.shopLng,
    required this.dropLat,
    required this.dropLng,
    required this.addressNote,
    required this.lines,
    required this.totalMinor,
    required this.pickupKm,
    required this.routeKm,
    required this.etaMinutes,
    required this.payoutMinor,
  });

  factory BoardJob.fromJson(Map<String, dynamic> j) => BoardJob(
        orderId: j['order_id'] as String,
        shopName: j['shop_name'] as String? ?? '',
        shopLat: (j['shop_lat'] as num).toDouble(),
        shopLng: (j['shop_lng'] as num).toDouble(),
        dropLat: (j['drop_lat'] as num).toDouble(),
        dropLng: (j['drop_lng'] as num).toDouble(),
        addressNote: j['address_note'] as String? ?? '',
        lines: (j['lines'] as num?)?.toInt() ?? 0,
        totalMinor: (j['total_minor'] as num?)?.toInt() ?? 0,
        pickupKm: (j['pickup_km'] as num?)?.toDouble() ?? 0,
        routeKm: (j['route_km'] as num?)?.toDouble() ?? 0,
        etaMinutes: (j['eta_minutes'] as num?)?.toInt() ?? 0,
        payoutMinor: (j['payout_minor'] as num?)?.toInt() ?? 0,
      );

  String get payoutText => money(payoutMinor);
}

/// One row of the admin's subscription table: subscription state beside the
/// current-month settlement the fixed Ujrah schedule computes.
class ShopSubscription {
  final String id;
  final String name;
  final bool active;
  final String phone;
  final String paidThrough;
  final String paymentNote;
  final bool eligible;
  final int monthOrders;
  final int monthSalesMinor;
  final int ujrahMinor;
  final int netMinor;

  const ShopSubscription({
    required this.id,
    required this.name,
    required this.active,
    required this.phone,
    required this.paidThrough,
    required this.paymentNote,
    required this.eligible,
    required this.monthOrders,
    required this.monthSalesMinor,
    required this.ujrahMinor,
    required this.netMinor,
  });

  factory ShopSubscription.fromJson(Map<String, dynamic> j) => ShopSubscription(
        id: j['id'] as String,
        name: j['name'] as String,
        active: j['active'] as bool? ?? false,
        phone: j['phone'] as String? ?? '',
        paidThrough: j['paid_through'] as String? ?? '',
        paymentNote: j['payment_note'] as String? ?? '',
        eligible: j['eligible'] as bool? ?? false,
        monthOrders: (j['month_orders'] as num?)?.toInt() ?? 0,
        monthSalesMinor: (j['month_sales_minor'] as num?)?.toInt() ?? 0,
        ujrahMinor: (j['ujrah_minor'] as num?)?.toInt() ?? 0,
        netMinor: (j['net_minor'] as num?)?.toInt() ?? 0,
      );
}

/// The 14-day daily series behind the admin charts (UTC buckets, oldest
/// first, empty days included).
class DailySeries {
  final List<String> days;
  final List<int> orders;
  final List<int> gmvMinor;
  final List<int> payoutMinor;

  const DailySeries({
    required this.days,
    required this.orders,
    required this.gmvMinor,
    required this.payoutMinor,
  });

  factory DailySeries.fromJson(Map<String, dynamic> j) => DailySeries(
        days: (j['days'] as List<dynamic>? ?? []).cast<String>(),
        orders: (j['orders'] as List<dynamic>? ?? []).map((e) => (e as num).toInt()).toList(),
        gmvMinor: (j['gmv_minor'] as List<dynamic>? ?? []).map((e) => (e as num).toInt()).toList(),
        payoutMinor: (j['payout_minor'] as List<dynamic>? ?? []).map((e) => (e as num).toInt()).toList(),
      );
}

/// A dispatch candidate (shop or rider) from the admin lists.
class Place {
  final String id;
  final String name;
  final double lat;
  final double lng;
  final bool active;

  const Place({
    required this.id,
    required this.name,
    required this.lat,
    required this.lng,
    required this.active,
  });

  factory Place.fromJson(Map<String, dynamic> j) => Place(
        id: j['id'] as String,
        name: j['name'] as String,
        lat: (j['lat'] as num).toDouble(),
        lng: (j['lng'] as num).toDouble(),
        active: j['active'] as bool? ?? true,
      );
}

/// The commission settlement view from GET /v1/admin/commission.
class CommissionView {
  final int monthlyOrders;
  final int salesMinor;
  final int commissionMinor;
  final int rateBps;
  final bool capped;

  const CommissionView({
    required this.monthlyOrders,
    required this.salesMinor,
    required this.commissionMinor,
    required this.rateBps,
    required this.capped,
  });

  factory CommissionView.fromJson(Map<String, dynamic> j) => CommissionView(
        monthlyOrders: (j['monthly_orders'] as num).toInt(),
        salesMinor: (j['sales_minor'] as num).toInt(),
        commissionMinor: (j['commission_minor'] as num).toInt(),
        rateBps: (j['commission_rate_bps'] as num).toInt(),
        capped: j['commission_capped'] as bool? ?? false,
      );

  String get rateText => '${(rateBps / 100).toStringAsFixed(1)}%';
}

/// Turns a URL slug into a readable label ("al-safwa" → "Al Safwa") — the
/// Dart twin of the server's catalog.Display, so chips and cards label
/// identically on both sides of the wire.
String displaySlug(String slug) {
  if (slug.isEmpty) {
    return '';
  }
  final words = slug.replaceAll('-', ' ').split(' ');
  for (var i = 0; i < words.length; i++) {
    final w = words[i];
    if (w.isNotEmpty) {
      words[i] = '${w[0].toUpperCase()}${w.substring(1)}';
    }
  }
  return words.join(' ');
}

/// Renders minor units as "major.minor" — the same convention as the
/// server's PriceText, so both sides of the wire display identically.
String money(int minor) =>
    '${minor ~/ 100}.${(minor % 100).toString().padLeft(2, '0')}';

/// One shop's current-month settlement row from GET /v1/admin/stats:
/// the delivered sales the Ujrah schedule settles over, the platform's
/// commission on them, and what the shop nets.
class ShopSettlement {
  final String id;
  final String name;
  final int orders;
  final int salesMinor;
  final int ujrahMinor;
  final int netMinor;

  const ShopSettlement({
    required this.id,
    required this.name,
    required this.orders,
    required this.salesMinor,
    required this.ujrahMinor,
    required this.netMinor,
  });

  factory ShopSettlement.fromJson(Map<String, dynamic> j) => ShopSettlement(
        id: j['id'] as String,
        name: j['name'] as String,
        orders: (j['orders'] as num).toInt(),
        salesMinor: (j['sales_minor'] as num).toInt(),
        ujrahMinor: (j['ujrah_minor'] as num).toInt(),
        netMinor: (j['net_minor'] as num).toInt(),
      );
}

/// One rider's all-time payout row from GET /v1/admin/stats — payouts are
/// earned at delivery, so only delivered orders appear.
class RiderEarnings {
  final String id;
  final String name;
  final int deliveries;
  final int payoutMinor;

  const RiderEarnings({
    required this.id,
    required this.name,
    required this.deliveries,
    required this.payoutMinor,
  });

  factory RiderEarnings.fromJson(Map<String, dynamic> j) => RiderEarnings(
        id: j['id'] as String,
        name: j['name'] as String,
        deliveries: (j['deliveries'] as num).toInt(),
        payoutMinor: (j['payout_minor'] as num).toInt(),
      );
}

/// The admin console's whole stats view: revenue (GMV and the platform's
/// Ujrah), rider payouts, and who is working and earning. Money in minor
/// units, exactly as the server computes it.
class AdminStats {
  final int shopsTotal;
  final int shopsActive;
  final int shopsSelling;
  final int ridersTotal;
  final int ridersAvailable;
  final int ridersPaid;
  final int ordersTotal;
  final Map<String, int> ordersByStatus;
  final int gmvMinor;
  final int gmvMonthMinor;
  final int riderPayoutsMinor;
  final int platformUjrahMinor;
  final List<ShopSettlement> shops;
  final List<RiderEarnings> riders;
  final DailySeries series;

  const AdminStats({
    required this.shopsTotal,
    required this.shopsActive,
    required this.shopsSelling,
    required this.ridersTotal,
    required this.ridersAvailable,
    required this.ridersPaid,
    required this.ordersTotal,
    required this.ordersByStatus,
    required this.gmvMinor,
    required this.gmvMonthMinor,
    required this.riderPayoutsMinor,
    required this.platformUjrahMinor,
    required this.shops,
    required this.riders,
    this.series = const DailySeries(days: [], orders: [], gmvMinor: [], payoutMinor: []),
  });

  factory AdminStats.fromJson(Map<String, dynamic> j) => AdminStats(
        shopsTotal: (j['shops_total'] as num?)?.toInt() ?? 0,
        shopsActive: (j['shops_active'] as num?)?.toInt() ?? 0,
        shopsSelling: (j['shops_selling'] as num?)?.toInt() ?? 0,
        ridersTotal: (j['riders_total'] as num?)?.toInt() ?? 0,
        ridersAvailable: (j['riders_available'] as num?)?.toInt() ?? 0,
        ridersPaid: (j['riders_paid'] as num?)?.toInt() ?? 0,
        ordersTotal: (j['orders_total'] as num?)?.toInt() ?? 0,
        ordersByStatus: (j['orders_by_status'] as Map<String, dynamic>? ?? {})
            .map((k, v) => MapEntry(k, (v as num).toInt())),
        gmvMinor: (j['gmv_minor'] as num?)?.toInt() ?? 0,
        gmvMonthMinor: (j['gmv_month_minor'] as num?)?.toInt() ?? 0,
        riderPayoutsMinor: (j['rider_payouts_minor'] as num?)?.toInt() ?? 0,
        platformUjrahMinor: (j['platform_ujrah_minor'] as num?)?.toInt() ?? 0,
        shops: (j['shops'] as List<dynamic>? ?? [])
            .map((e) => ShopSettlement.fromJson(e as Map<String, dynamic>))
            .toList(),
        riders: (j['riders'] as List<dynamic>? ?? [])
            .map((e) => RiderEarnings.fromJson(e as Map<String, dynamic>))
            .toList(),
        series: j['series'] is Map<String, dynamic>
            ? DailySeries.fromJson(j['series'] as Map<String, dynamic>)
            : const DailySeries(days: [], orders: [], gmvMinor: [], payoutMinor: []),
      );
}
