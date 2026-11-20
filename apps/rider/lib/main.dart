import 'package:flutter/material.dart';

void main() => runApp(const CityMarketRiderApp());

/// Phase 1 boilerplate. The rider app — job board, route tracking and the
/// payout statement (60 per delivery + 15 per km, computed server-side in
/// `server/finance`) — is built in Phase 4.
class CityMarketRiderApp extends StatelessWidget {
  const CityMarketRiderApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Rider',
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF15803D), // halal-700 brand green
        useMaterial3: true,
      ),
      home: Scaffold(
        appBar: AppBar(title: const Text('Rider App')),
        body: const Center(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text(
              'Rider app — deliveries and payouts.\n\n'
              'Payout rule: 60 per delivery + 15 per km.\n'
              'Job board and route tracking arrive in Phase 4.',
              textAlign: TextAlign.center,
            ),
          ),
        ),
      ),
    );
  }
}
