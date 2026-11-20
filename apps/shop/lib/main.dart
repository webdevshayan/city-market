import 'package:flutter/material.dart';

void main() => runApp(const CityMarketShopApp());

/// Phase 1 boilerplate. The shop dashboard — catalogue management, incoming
/// orders and the monthly Ujrah commission statement (tiered: 0% up to 50
/// orders, 5% to 200, 7% to 500, 9% capped at 7000 beyond) — is built in
/// Phase 4. Commission math itself lives server-side in `server/finance`.
class CityMarketShopApp extends StatelessWidget {
  const CityMarketShopApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Shop',
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF15803D), // halal-700 brand green
        useMaterial3: true,
      ),
      home: Scaffold(
        appBar: AppBar(title: const Text('Shop Dashboard')),
        body: const Center(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text(
              'Shop app — manage catalogue and orders.\n\n'
              'Dashboard, commission statements and Halal certificate upload '
              'arrive in Phase 4.',
              textAlign: TextAlign.center,
            ),
          ),
        ),
      ),
    );
  }
}
