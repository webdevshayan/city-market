import 'package:city_market_citizen/main.dart';
import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('catalog renders products and money', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    expect(find.text('Halal Beef Boneless'), findsOneWidget);
    expect(find.text('1kg · 1400.00'), findsOneWidget);
    expect(find.byIcon(Icons.add_shopping_cart), findsOneWidget);
  });

  testWidgets('add to cart, place order, land on the status screen', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    expect(find.text('Order 1 item'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle(); // dialog
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();

    expect(api.placeOrderCalls, hasLength(1));
    expect(api.placeOrderCalls.single.single.productId, 'beef-1kg');
    expect(find.textContaining('ord-test-1'), findsOneWidget);
    expect(find.text('placed'), findsOneWidget);
    expect(find.text('Central Market Halal Store'), findsOneWidget);
    expect(find.text('2825.00'), findsOneWidget); // server-priced total
  });

  testWidgets('ApiException surfaces as a real message, not a crash', (tester) async {
    final api = _FailingApi();
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();

    expect(find.byType(SnackBar), findsOneWidget);
    expect(find.textContaining('no rider available'), findsOneWidget);
    expect(find.textContaining('ord-test-1'), findsNothing); // stayed on catalog
  });
}

class _FailingApi extends FakeApi {
  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng) async {
    placeOrderCalls.add(items);
    throw ApiException(409, 'no rider available right now');
  }
}
