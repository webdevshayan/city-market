import 'package:core/design.dart';
import 'package:core/map.dart';
import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

void main() {
  const route = CMRoute(
    pickup: LatLng(26.692061, 67.613300),
    drop: LatLng(26.695000, 67.615000),
    pickupName: 'Johi Store',
    dropName: 'The customer',
  );

  Widget host(Widget child) => MaterialApp(
        theme: buildCMTheme(),
        home: Scaffold(body: SizedBox(height: 320, child: child)),
      );

  testWidgets('job map: markers, on-map attribution and the layer toggle', (tester) async {
    await tester.pumpWidget(host(CMJobMap(route: route)));
    await tester.pump();

    // The route legend and the OSM credit are always present; pump() only —
    // tile image streams never settle in tests by design.
    expect(find.text('Pick up: Johi Store'), findsOneWidget);
    expect(find.text('Deliver to: The customer'), findsOneWidget);
    expect(find.text('© OpenStreetMap contributors'), findsOneWidget);
    expect(find.byIcon(Icons.storefront_outlined), findsOneWidget);
    expect(find.byIcon(Icons.home_outlined), findsOneWidget);

    // One tap switches to satellite imagery and the credit switches with it.
    await tester.tap(find.byIcon(Icons.satellite_alt_outlined));
    await tester.pump();
    expect(find.textContaining('Esri'), findsOneWidget);

    // And back to street tiles.
    await tester.tap(find.byIcon(Icons.map_outlined));
    await tester.pump();
    expect(find.text('© OpenStreetMap contributors'), findsOneWidget);
  });

  testWidgets('mini map: fits the route and keeps attribution on the map', (tester) async {
    await tester.pumpWidget(host(CMMiniMap(route: route)));
    await tester.pump();

    expect(find.byType(FlutterMap), findsOneWidget);
    expect(find.text('© OpenStreetMap contributors'), findsOneWidget);
  });
}
