/// Shared data models for the City Market apps. JSON shapes mirror the Go
/// server exactly: snake_case keys, money as int64 minor units
/// (1 major unit = 100 minor units), and no business logic here — the
/// server (server/finance, server/orders) is the single source of truth.
library;

/// One catalog listing, as served by GET /v1/products — the public catalog
/// the citizen app browses.
class Product {
  final String id;
  final String name;
  final String category;
  final String brand;
  final String size;
  final String image;
  final String shopId;
  final int priceMinor;

  const Product({
    required this.id,
    required this.name,
    required this.category,
    required this.brand,
    required this.size,
    required this.image,
    required this.shopId,
    required this.priceMinor,
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
      );

  /// Price rendered as "major.minor", matching the server's PriceText.
  String get priceText => money(priceMinor);
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
  });

  factory Order.fromJson(Map<String, dynamic> j) => Order(
        id: j['id'] as String,
        status: j['status'] as String,
        shopId: j['shop_id'] as String,
        shopName: j['shop_name'] as String? ?? j['shop_id'] as String,
        riderId: j['rider_id'] as String,
        riderName: j['rider_name'] as String? ?? j['rider_id'] as String,
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
      );
}
