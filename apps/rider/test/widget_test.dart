import 'package:city_market_rider/main.dart';
import 'package:core/models.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('confirmed orders offer Pick up, then Deliver', (tester) async {
    final api = FakeApi(orders: [
      Order(
          id: 'ord-r-1',
          status: statusConfirmed,
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
          riderPayoutMinor: 10500,
          lat: 24.8607,
          lng: 67.0011,
          createdAt: DateTime.parse('2026-11-20T10:00:00Z')),
    ]);
    await tester.pumpWidget(MaterialApp(home: RiderHomeScreen(api: api)));
    await tester.pumpAndSettle();

    expect(find.textContaining('ord-r-1'), findsOneWidget);
    expect(find.textContaining('payout 105.00'), findsOneWidget); // 3 km route
    expect(find.byKey(const ValueKey('pick up-ord-r-1')), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('pick up-ord-r-1')));
    await tester.pumpAndSettle();
    expect(api.pickUpCalls, ['ord-r-1']);
    expect(find.text('Picked up'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('deliver-ord-r-1')));
    await tester.pumpAndSettle();
    expect(api.deliverCalls, ['ord-r-1']);
    expect(find.text('Delivered'), findsOneWidget);
    // Delivered orders show no action button.
    expect(find.byKey(const ValueKey('deliver-ord-r-1')), findsNothing);
  });

  testWidgets('rider ID switches the job list', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: RiderHomeScreen(api: api)));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const ValueKey('rider-id-field')), 'r-2');
    await tester.tap(find.byKey(const ValueKey('load-btn')));
    await tester.pumpAndSettle();

    expect(find.text('Order ord-shop-1'), findsOneWidget); // fake fixture
  });
}
