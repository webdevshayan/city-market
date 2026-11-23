import 'package:city_market_shop/main.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

Widget host(FakeApi api) => MaterialApp(home: SessionGate(api: api));

Future<void> signIn(WidgetTester tester) async {
  SharedPreferences.setMockInitialValues({});
  final api = FakeApi();
  await tester.pumpWidget(host(api));
  await tester.pumpAndSettle();

  await tester.enterText(find.byKey(const ValueKey('shop-phone-field')), '03001234567');
  await tester.enterText(find.byKey(const ValueKey('shop-password-field')), 'long-enough');
  await tester.tap(find.byKey(const ValueKey('shop-submit-btn')));
  await tester.pumpAndSettle();
  return;
}

void main() {
  testWidgets('sign-in lands on the dashboard with the paid banner', (tester) async {
    SharedPreferences.setMockInitialValues({});
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('shop-phone-field')), findsOneWidget);
    await tester.enterText(find.byKey(const ValueKey('shop-phone-field')), '03001234567');
    await tester.enterText(find.byKey(const ValueKey('shop-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('shop-submit-btn')));
    await tester.pumpAndSettle();

    // The fake account is eligible → the green taking-orders banner.
    expect(find.textContaining('Taking orders — paid through'), findsOneWidget);
    // The queue renders the canned order with the accept/cancel pair.
    expect(find.byKey(const ValueKey('accept-ord-shop-1')), findsOneWidget);
    expect(find.textContaining('COD'), findsOneWidget);
    expect(find.textContaining('Near the grain market'), findsOneWidget);
  });

  testWidgets('register mode explains the unpaid start', (tester) async {
    SharedPreferences.setMockInitialValues({});
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('shop-toggle-mode')));
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('shop-name-field')), findsOneWidget);
    expect(find.textContaining('New shops start unpaid'), findsOneWidget);

    await tester.enterText(find.byKey(const ValueKey('shop-name-field')), 'My Shelf');
    await tester.enterText(find.byKey(const ValueKey('shop-phone-field')), '03001234567');
    await tester.enterText(find.byKey(const ValueKey('shop-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('shop-submit-btn')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Taking orders'), findsOneWidget); // fake account is paid
  });

  testWidgets('accepting an order confirms it and the chip updates', (tester) async {
    await signIn(tester);
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('accept-ord-shop-1')));
    await tester.pumpAndSettle();

    expect(api.acceptCalls, ['ord-shop-1']);
    expect(find.text('Confirmed'), findsOneWidget);
    expect(find.byKey(const ValueKey('accept-ord-shop-1')), findsNothing);
  });

  testWidgets('the product form validates and creates via the API', (tester) async {
    await signIn(tester);
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-product-btn')));
    await tester.pumpAndSettle();

    // Invalid: no price, no stock.
    await tester.enterText(find.byKey(const ValueKey('pf-name')), 'Sindh Rice');
    await tester.enterText(find.byKey(const ValueKey('pf-category')), 'grocery');
    await tester.enterText(find.byKey(const ValueKey('pf-brand')), 'noor');
    await tester.enterText(find.byKey(const ValueKey('pf-size')), '5kg');
    await tester.tap(find.byKey(const ValueKey('pf-submit')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Every field matters'), findsOneWidget);

    await tester.enterText(find.byKey(const ValueKey('pf-price')), '2200');
    await tester.enterText(find.byKey(const ValueKey('pf-stock')), '10');
    await tester.tap(find.byKey(const ValueKey('pf-submit')));
    await tester.pumpAndSettle();

    expect(api.createProductCalls.single['price_minor'], 220000);
    expect(api.createProductCalls.single['stock'], 10);
    expect(find.byKey(const ValueKey('pf-submit')), findsNothing); // popped back
  });

  testWidgets('sign out returns to the sign-in screen', (tester) async {
    SharedPreferences.setMockInitialValues({'shop_token': 'saved-token'});
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('shop-phone-field')), findsNothing);
    await tester.tap(find.byTooltip('Sign out'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('shop-phone-field')), findsOneWidget);
  });
}
