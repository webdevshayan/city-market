import 'package:flutter/material.dart';

void main() => runApp(const CityMarketCitizenApp());

/// Phase 1 boilerplate. The real customer experience — product-first browsing,
/// cart and flat-fee checkout — is implemented in Phase 4 against the Go
/// server API. No marketplace logic lives in the client.
class CityMarketCitizenApp extends StatelessWidget {
  const CityMarketCitizenApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Citizen',
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF15803D), // halal-700 brand green
        useMaterial3: true,
      ),
      home: Scaffold(
        appBar: AppBar(title: const Text('City Market')),
        body: const Center(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text(
              'Customer app — 100% Halal marketplace.\n\n'
              'Browse, cart and checkout (flat delivery fee) arrive in Phase 4.',
              textAlign: TextAlign.center,
            ),
          ),
        ),
      ),
    );
  }
}
