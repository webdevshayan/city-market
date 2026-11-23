import 'package:city_market_citizen/main.dart';
import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('catalog renders products with stock and filter chips', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    expect(find.text('Halal Beef Boneless'), findsOneWidget);
    expect(find.textContaining('In stock: 12'), findsOneWidget);
    expect(find.text('Meat'), findsOneWidget); // the category filter chip
    expect(find.byKey(const ValueKey('shop-all-chip')), findsOneWidget);
    expect(find.byKey(const ValueKey('shop-s-central')), findsOneWidget);
  });

  testWidgets('out-of-stock products cannot be added', (tester) async {
    final api = FakeApi(products: [
      const Product(
          id: 'mutton-1kg',
          name: 'Mutton Leg Cuts',
          category: 'meat',
          brand: 'madina',
          size: '1kg',
          image: '',
          shopId: 's-north',
          priceMinor: 240000,
          stock: 0),
    ]);
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    expect(find.text('Out of stock'), findsOneWidget);
    expect(find.byKey(const ValueKey('add-mutton-1kg')), findsNothing);
    expect(find.byKey(const ValueKey('checkout-btn')), findsNothing);
  });

  testWidgets('checkout takes a delivery address and places a COD order for the shop', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle();

    // The address is required — the rider reads it before setting out.
    await tester.dragUntilVisible(
      find.byKey(const ValueKey('place-order-btn')),
      find.byType(ListView).last,
      const Offset(0, -150),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Add a short delivery address'), findsOneWidget);

    await tester.enterText(find.byKey(const ValueKey('address-field')), 'Near the water tank, green gate');
    await tester.dragUntilVisible(
      find.byKey(const ValueKey('place-order-btn')),
      find.byType(ListView).last,
      const Offset(0, -150),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();

    expect(api.placeOrderCalls, hasLength(1));
    expect(api.placeOrderShopIds.single, 's-central'); // the specific shop
    expect(api.placeOrderNotes.single, 'Near the water tank, green gate');
    expect(find.textContaining('ord-test-1'), findsOneWidget);
    expect(find.text('Placed'), findsOneWidget);
    expect(find.textContaining('Madina Halal Store'), findsOneWidget);
    expect(find.textContaining('Waiting for a rider'), findsOneWidget);
    expect(find.textContaining('cash on delivery'), findsOneWidget);
    expect(find.text('2825.00'), findsOneWidget); // server-priced total
    // The fake echoes its canned order, whose address is the grain market one.
    expect(find.textContaining('Near the grain market'), findsOneWidget);
  });

  testWidgets('ApiException surfaces as a real message, not a crash', (tester) async {
    final api = _FailingApi();
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle();
    await tester.pumpAndSettle(); // let the fullscreen dialog finish entering
    await tester.enterText(find.byKey(const ValueKey('address-field')), 'somewhere in Johi');
    await tester.dragUntilVisible(
      find.byKey(const ValueKey('place-order-btn')),
      find.byType(ListView).last,
      const Offset(0, -150),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();

    // The checkout screen surfaces the server's reason inline, not as a snackbar.
    expect(find.textContaining('not enough stock'), findsOneWidget);
    expect(find.byKey(const ValueKey('place-order-btn')), findsOneWidget); // stayed on checkout
  });

  testWidgets('the shop filter narrows the listing', (tester) async {
    final api = FakeApi(products: [
      const Product(
          id: 'beef-1kg',
          name: 'Halal Beef Boneless',
          category: 'meat',
          brand: 'al-safwa',
          size: '1kg',
          image: '',
          shopId: 's-central',
          priceMinor: 140000,
          stock: 10),
      const Product(
          id: 'rice-5kg',
          name: 'Basmati Rice',
          category: 'grocery',
          brand: 'noor',
          size: '5kg',
          image: '',
          shopId: 's-north',
          priceMinor: 220000,
          stock: 8),
    ]);
    await tester.pumpWidget(MaterialApp(home: CatalogScreen(api: api)));
    await tester.pumpAndSettle();

    expect(find.text('Halal Beef Boneless'), findsOneWidget);
    expect(find.text('Basmati Rice'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('shop-s-central')));
    await tester.pumpAndSettle();
    expect(find.text('Halal Beef Boneless'), findsOneWidget);
    expect(find.text('Basmati Rice'), findsNothing); // another shop's shelf

    await tester.tap(find.byKey(const ValueKey('shop-all-chip')));
    await tester.pumpAndSettle();
    expect(find.text('Basmati Rice'), findsOneWidget);
  });
}

class _FailingApi extends FakeApi {
  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng,
      {String? shopId, String addressNote = ''}) async {
    placeOrderCalls.add(items);
    placeOrderShopIds.add(shopId);
    throw ApiException(409, 'orders: not enough stock on the shelf');
  }
}
