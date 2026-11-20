import 'package:city_market_shop/main.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('the shop queue renders and confirms placed orders', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: ShopHomeScreen(api: api)));
    await tester.pumpAndSettle();

    expect(find.textContaining('ord-shop-1'), findsOneWidget);
    expect(find.text('placed'), findsOneWidget);
    expect(find.byKey(const ValueKey('confirm-ord-shop-1')), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('confirm-ord-shop-1')));
    await tester.pumpAndSettle();

    expect(api.confirmCalls, ['ord-shop-1']);
    expect(find.text('confirmed'), findsOneWidget); // the reloaded status chip
    expect(find.byKey(const ValueKey('confirm-ord-shop-1')), findsNothing);
  });

  testWidgets('switching shop IDs reloads the queue', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(MaterialApp(home: ShopHomeScreen(api: api)));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const ValueKey('shop-id-field')), 's-north');
    await tester.tap(find.byKey(const ValueKey('load-btn')));
    await tester.pumpAndSettle();

    expect(find.text('Order ord-shop-1'), findsOneWidget); // fake serves the same fixture
  });
}
