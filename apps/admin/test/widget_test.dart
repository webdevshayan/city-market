import 'package:city_market_admin/main.dart';
import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  Future<void> pump(WidgetTester tester, FakeApi api) async {
    // The dashboard is one long scrolling list; a tall viewport keeps every
    // section built so key-based finders work throughout.
    tester.view.physicalSize = const Size(800, 2600);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(MaterialApp(home: AdminHomeScreen(api: api)));
    await tester.pumpAndSettle();
  }

  Future<void> signIn(WidgetTester tester) async {
    await tester.enterText(find.byKey(const ValueKey('admin-token')), 'sekret');
    await tester.tap(find.byKey(const ValueKey('stats-go')));
    await tester.pumpAndSettle();
  }

  testWidgets('sign-in reveals the stats dashboard from the FakeApi fixture', (tester) async {
    final api = FakeApi();
    await pump(tester, api);
    await signIn(tester);

    // Revenue: the platform's Ujrah (311.00) and GMV (4650.00).
    expect(find.text('311.00'), findsWidgets);
    expect(find.text('4650.00'), findsWidgets);
    // Payouts: 255.00 paid out over the fixture's deliveries.
    expect(find.text('255.00'), findsWidgets);
    // Workers: one of two shops has delivered, one of three riders paid.
    expect(find.text('1 of 2'), findsWidgets);
    expect(find.text('1 of 3'), findsWidgets);
    // Settlement and earnings rows from the fixture.
    expect(find.text('Central Market Halal Store'), findsOneWidget);
    expect(find.text('Rider 1'), findsOneWidget);
  });

  testWidgets('a rejected token surfaces the server message with a retry', (tester) async {
    final api = _LockedApi();
    await pump(tester, api);
    await signIn(tester);

    expect(find.text('Could not reach the market server'), findsOneWidget);
    expect(find.textContaining('invalid admin token'), findsOneWidget);
    expect(find.text('Try again'), findsOneWidget);
  });

  testWidgets('sign out returns to the token gate', (tester) async {
    final api = FakeApi();
    await pump(tester, api);
    await signIn(tester);

    await tester.tap(find.byKey(const ValueKey('signout-btn')));
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('admin-token')), findsOneWidget);
  });
}

class _LockedApi extends FakeApi {
  @override
  Future<AdminStats> adminStats(String adminToken) async {
    throw ApiException(401, 'invalid admin token');
  }
}
