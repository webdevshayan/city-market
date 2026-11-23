import 'package:city_market_rider/main.dart';
import 'package:core/models.dart' show statusConfirmed;
import 'package:core/testing.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

Widget host(FakeApi api) => MaterialApp(home: SessionGate(api: api));

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  testWidgets('sign-in lands on the board with duty controls', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const ValueKey('rider-phone-field')), '03441111111');
    await tester.enterText(find.byKey(const ValueKey('rider-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('rider-submit-btn')));
    await tester.pumpAndSettle();

    expect(find.text('On duty — claims are open'), findsOneWidget);
    // The board card shows the full decision set before any commitment.
    expect(find.textContaining('Pick up: Test Shelf'), findsOneWidget);
    expect(find.textContaining('960.00 PKR'), findsOneWidget);
    expect(find.textContaining('~14 min'), findsOneWidget);
    expect(find.textContaining('2.4 km route'), findsOneWidget);
    expect(find.textContaining('Deliver: Near the grain market'), findsOneWidget);
  });

  testWidgets('the duty toggle calls the API both ways', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('rider-phone-field')), '03441111111');
    await tester.enterText(find.byKey(const ValueKey('rider-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('rider-submit-btn')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('duty-toggle')));
    await tester.pumpAndSettle();
    expect(api.setMeCalls.last['available'], false);

    await tester.tap(find.byKey(const ValueKey('duty-toggle')));
    await tester.pumpAndSettle();
    expect(api.setMeCalls.last['available'], true);
  });

  testWidgets('claiming a job moves it into My claimed jobs', (tester) async {
    final api = FakeApi();
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('rider-phone-field')), '03441111111');
    await tester.enterText(find.byKey(const ValueKey('rider-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('rider-submit-btn')));
    await tester.pumpAndSettle();

    // Open the detail screen and claim from there — tap the text strip
    // below the preview map, which owns the tap (the map area consumes
    // gestures by design).
    await tester.tap(find.text('Pick up: Test Shelf'));
    await tester.pumpAndSettle();
    expect(find.text('Before you claim'), findsOneWidget);
    expect(find.textContaining('Pick up:'), findsOneWidget);
    expect(find.textContaining('Deliver to:'), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('claim-ord-shop-1')));
    await tester.pumpAndSettle();

    expect(api.claimCalls, ['ord-shop-1']);
    expect(find.text('MY CLAIMED JOBS'), findsOneWidget);
    expect(find.byKey(const ValueKey('pick-up-ord-shop-1')), findsOneWidget);
  });

  testWidgets('claimed jobs walk pick-up then deliver', (tester) async {
    final api = FakeApi();
    // The canned claimed job has already been accepted by the shop and
    // claimed — the state the pick-up button expects.
    api.applyStatus('ord-shop-1', statusConfirmed);
    await tester.pumpWidget(host(api));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('rider-phone-field')), '03441111111');
    await tester.enterText(find.byKey(const ValueKey('rider-password-field')), 'long-enough');
    await tester.tap(find.byKey(const ValueKey('rider-submit-btn')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('pick-up-ord-shop-1')));
    await tester.pumpAndSettle();
    expect(api.pickUpCalls, ['ord-shop-1']);
    expect(find.text('Picked up'), findsOneWidget); // the status chip

    await tester.tap(find.byKey(const ValueKey('deliver-ord-shop-1')));
    await tester.pumpAndSettle();
    expect(api.deliverCalls, ['ord-shop-1']);
    expect(find.text('Delivered'), findsOneWidget);
  });
}
