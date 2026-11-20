import 'package:city_market_admin/main.dart';
import 'package:core/api.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  Future<void> pump(WidgetTester tester, FakeApi api) async {
    // The console is one long scrolling form; a tall viewport keeps every
    // section built so key-based finders and enterText work throughout.
    tester.view.physicalSize = const Size(600, 2600);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(MaterialApp(home: AdminHomeScreen(api: api)));
    await tester.pumpAndSettle();
  }

  testWidgets('lists render and the shop upsert records a call', (tester) async {
    final api = FakeApi();
    await pump(tester, api);

    expect(find.text('Central Market Halal Store'), findsOneWidget);
    expect(find.text('Halal Beef Boneless'), findsOneWidget);

    await tester.enterText(find.byKey(const ValueKey('admin-token')), 'sekret');
    await tester.enterText(find.byKey(const ValueKey('shop-name')), 'DHA Store');
    await tester.tap(find.byKey(const ValueKey('shop-save')));
    await tester.pumpAndSettle();

    expect(api.upsertShopCalls, hasLength(1));
    expect(api.upsertShopCalls.single, containsPair('name', 'DHA Store'));
    expect(find.text('Shop saved'), findsOneWidget);
  });

  testWidgets('the commission calculator shows the fixed-schedule result', (tester) async {
    final api = FakeApi();
    await pump(tester, api);

    await tester.enterText(find.byKey(const ValueKey('admin-token')), 'sekret');
    await tester.enterText(find.byKey(const ValueKey('commission-orders')), '250');
    await tester.enterText(find.byKey(const ValueKey('commission-sales')), '10000000');
    await tester.tap(find.byKey(const ValueKey('commission-go')));
    await tester.pumpAndSettle();

    expect(find.textContaining('Commission: 7000.00'), findsOneWidget);
    expect(find.textContaining('7.0%'), findsOneWidget);
  });

  testWidgets('ApiException messages surface as snacks', (tester) async {
    final api = _LockedApi();
    await pump(tester, api);

    await tester.tap(find.byKey(const ValueKey('shop-save')));
    await tester.pumpAndSettle();

    expect(find.textContaining('invalid admin token'), findsOneWidget);
  });
}

class _LockedApi extends FakeApi {
  @override
  Future<void> adminUpsertShop(String adminToken,
      {required String id, required String name, required double lat, required double lng, required bool active}) async {
    upsertShopCalls.add({'id': id, 'name': name});
    throw ApiException(401, 'invalid admin token');
  }
}
