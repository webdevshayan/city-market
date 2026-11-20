import 'package:core/models.dart';
import 'package:test/test.dart';

void main() {
  test('money renders minor units exactly like the server', () {
    expect(money(140000), '1400.00');
    expect(money(2500), '25.00');
    expect(money(5), '0.05');
    expect(money(0), '0.00');
    expect(money(999999), '9999.99');
  });

  test('Product.fromJson parses the server JSON shape', () {
    final p = Product.fromJson({
      'id': 'beef-1kg',
      'name': 'Halal Beef Boneless',
      'category': 'meat',
      'brand': 'al-safwa',
      'size': '1kg',
      'price_minor': 140000,
      'image': '/static/img/meat.svg',
      'shop_id': 's-central',
    });
    expect(p.shopId, 's-central');
    expect(p.priceText, '1400.00');
  });

  test('Order.fromJson parses the full order including price snapshots', () {
    final o = Order.fromJson({
      'id': 'ord-abc123',
      'status': 'placed',
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
      'created_at': '2026-11-20T10:00:00.123456Z',
    });
    expect(o.items, hasLength(1));
    expect(o.items.first.productId, 'beef-1kg');
    expect(o.totalMinor, 282500);
    expect(o.createdAt.year, 2026);
    expect(o.status, statusPlaced);
  });

  test('CommissionView parses the settlement response', () {
    final c = CommissionView.fromJson({
      'monthly_orders': 250,
      'sales_minor': 10000000,
      'commission_minor': 700000,
      'commission_rate_bps': 700,
      'commission_capped': false,
    });
    expect(c.rateText, '7.0%');
    expect(c.commissionMinor, 700000);
  });
}
