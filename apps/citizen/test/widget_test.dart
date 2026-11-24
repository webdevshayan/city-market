import 'package:city_market_citizen/main.dart';
import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

Widget host(FakeApi api) => MaterialApp(home: SessionGate(api: api));

Future<FakeApi> pumpGuest(WidgetTester tester) async {
  silenceTileFetchErrors(); // installed in the body — setUp's handler is clobbered by the binding
  SharedPreferences.setMockInitialValues({});
  final api = FakeApi();
  await tester.pumpWidget(host(api));
  await tester.pumpAndSettle();
  return api;
}

Future<void> signIn(WidgetTester tester, FakeApi api, {bool register = false}) async {
  await tester.tap(find.byKey(const ValueKey('open-signin-btn')));
  await tester.pumpAndSettle();
  if (register) {
    await tester.tap(find.byKey(const ValueKey('citizen-toggle-mode')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('citizen-name-field')), 'Aisha Khaskheli');
  }
  await tester.enterText(find.byKey(const ValueKey('citizen-phone-field')), '03111234567');
  await tester.enterText(find.byKey(const ValueKey('citizen-password-field')), 'long-enough');
  await tester.tap(find.byKey(const ValueKey('citizen-submit-btn')));
  await tester.pumpAndSettle();
}

void main() {
  setUp(silenceTileFetchErrors);

  testWidgets('a guest browses, checks out and orders without any account', (tester) async {
    final api = await pumpGuest(tester);

    expect(find.text('Halal Beef Boneless'), findsOneWidget);
    expect(find.textContaining('In stock: 12'), findsOneWidget);
    expect(find.byKey(const ValueKey('open-signin-btn')), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle();
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
    expect(api.placeOrderBearers.single, ''); // a guest order carries no token
    expect(find.textContaining('ord-test-1'), findsOneWidget);
    expect(find.text('Placed'), findsOneWidget);
    expect(find.text('2825.00'), findsOneWidget);
  });

  testWidgets('register creates the account and lands back signed in', (tester) async {
    final api = await pumpGuest(tester);

    await tester.tap(find.byKey(const ValueKey('open-signin-btn')));
    await tester.pumpAndSettle();
    expect(find.textContaining('You can also order without an account'), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('citizen-toggle-mode')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('citizen-name-field')), 'Aisha Khaskheli');
    await tester.enterText(find.byKey(const ValueKey('citizen-phone-field')), '0311-1234567');
    await tester.enterText(find.byKey(const ValueKey('citizen-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('citizen-submit-btn')));
    await tester.pumpAndSettle();

    expect(api.citizenRegisterCalls.single['phone'], '0311-1234567');
    expect(find.byKey(const ValueKey('account-menu')), findsOneWidget); // signed in
    expect(find.byKey(const ValueKey('open-signin-btn')), findsNothing);
  });

  testWidgets('login keeps the session and My Orders shows history', (tester) async {
    final api = await pumpGuest(tester);
    await signIn(tester, api);
    // re-run with the same api to record the login call
    expect(api.loginCalls.single['role'], 'citizen');
    expect(find.byKey(const ValueKey('account-menu')), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('account-menu')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('My orders'));
    await tester.pumpAndSettle();

    expect(find.textContaining('Madina Halal Store'), findsOneWidget);
    expect(find.textContaining('1425.00 PKR'), findsOneWidget);
    expect(find.textContaining('Near the water tank'), findsOneWidget);
  });

  testWidgets('a signed-in checkout attaches the account token', (tester) async {
    SharedPreferences.setMockInitialValues({});
    silenceTileFetchErrors();
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('open-signin-btn')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('citizen-phone-field')), '03111234567');
    await tester.enterText(find.byKey(const ValueKey('citizen-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('citizen-submit-btn')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('address-field')), 'Near the water tank');
    await tester.dragUntilVisible(
      find.byKey(const ValueKey('place-order-btn')),
      find.byType(ListView).last,
      const Offset(0, -150),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();

    expect(api.placeOrderBearers.single, 'token-fixture'); // history attaches
    expect(find.textContaining('ord-test-1'), findsOneWidget);
  });

  testWidgets('sign out returns the app to guest mode', (tester) async {
    final api = FakeApi();
    SharedPreferences.setMockInitialValues({});
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('open-signin-btn')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('citizen-phone-field')), '03111234567');
    await tester.enterText(find.byKey(const ValueKey('citizen-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('citizen-submit-btn')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('account-menu')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Sign out'));
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('open-signin-btn')), findsOneWidget);
    expect(find.byKey(const ValueKey('account-menu')), findsNothing);
  });

  testWidgets('out-of-stock products cannot be added', (tester) async {
    SharedPreferences.setMockInitialValues({});
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
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    expect(find.text('Out of stock'), findsOneWidget);
    expect(find.byKey(const ValueKey('add-mutton-1kg')), findsNothing);
    expect(find.byKey(const ValueKey('checkout-btn')), findsNothing);
  });

  testWidgets('the shop filter narrows the listing', (tester) async {
    SharedPreferences.setMockInitialValues({});
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
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('shop-s-central')));
    await tester.pumpAndSettle();
    expect(find.text('Halal Beef Boneless'), findsOneWidget);
    expect(find.text('Basmati Rice'), findsNothing);

    await tester.tap(find.byKey(const ValueKey('shop-all-chip')));
    await tester.pumpAndSettle();
    expect(find.text('Basmati Rice'), findsOneWidget);
  });

  testWidgets('ApiException surfaces as a real message, not a crash', (tester) async {
    SharedPreferences.setMockInitialValues({});
    silenceTileFetchErrors();
    final api = _FailingApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('add-beef-1kg')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('checkout-btn')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('address-field')), 'somewhere in Johi');
    await tester.dragUntilVisible(
      find.byKey(const ValueKey('place-order-btn')),
      find.byType(ListView).last,
      const Offset(0, -150),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('place-order-btn')));
    await tester.pumpAndSettle();

    expect(find.textContaining('not enough stock'), findsOneWidget);
    expect(find.byKey(const ValueKey('place-order-btn')), findsOneWidget);
  });
}

class _FailingApi extends FakeApi {
  @override
  Future<Order> placeOrder(List<OrderDraftItem> items, double lat, double lng,
      {String? shopId, String addressNote = '', String bearer = ''}) async {
    placeOrderCalls.add(items);
    placeOrderShopIds.add(shopId);
    placeOrderBearers.add(bearer);
    throw ApiException(409, 'orders: not enough stock on the shelf');
  }
}
