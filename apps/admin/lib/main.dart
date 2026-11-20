import 'package:flutter/material.dart';

void main() => runApp(const CityMarketAdminApp());

/// Phase 1 boilerplate. The admin console — shop/rider oversight, dispute
/// handling and Halal compliance flags — is built in Phase 4.
class CityMarketAdminApp extends StatelessWidget {
  const CityMarketAdminApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Admin',
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF15803D), // halal-700 brand green
        useMaterial3: true,
      ),
      home: Scaffold(
        appBar: AppBar(title: const Text('Admin Console')),
        body: const Center(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text(
              'Admin console — marketplace oversight.\n\n'
              'Shop approvals, rider disputes and monthly settlement reports '
              'arrive in Phase 4.',
              textAlign: TextAlign.center,
            ),
          ),
        ),
      ),
    );
  }
}
