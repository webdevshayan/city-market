import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';

void main() => runApp(const CityMarketRiderApp());

/// The rider app: assigned orders with the two rider steps of the lifecycle
/// (confirmed → picked_up → delivered) and the payout the engine computed
/// for each route (60 per delivery + 15 per km, see server/finance).
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
      home: RiderHomeScreen(
        api: HttpApiClient(
          const String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:3000'),
        ),
      ),
    );
  }
}

class RiderHomeScreen extends StatefulWidget {
  final ApiClient api;

  const RiderHomeScreen({super.key, required this.api});

  @override
  State<RiderHomeScreen> createState() => _RiderHomeScreenState();
}

class _RiderHomeScreenState extends State<RiderHomeScreen> {
  final _riderId = TextEditingController(text: 'r-1');
  late Future<List<Order>> _orders = _load();

  Future<List<Order>> _load() => widget.api.riderOrders(_riderId.text.trim());

  Future<void> _reload() async {
    // Block body on purpose: an arrow closure here would return the Future,
    // and setState() asserts against that.
    setState(() {
      _orders = _load();
    });
  }

  Future<void> _advance(Order o) async {
    try {
      if (o.status == statusConfirmed) {
        await widget.api.pickUpOrder(o.id);
      } else if (o.status == statusPickedUp) {
        await widget.api.deliverOrder(o.id);
      }
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
    await _reload();
  }

  @override
  void dispose() {
    _riderId.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Rider jobs')),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(children: [
              Expanded(
                child: TextField(
                  controller: _riderId,
                  key: const ValueKey('rider-id-field'),
                  decoration: const InputDecoration(
                    labelText: 'Rider ID',
                    border: OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => _reload(),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(
                key: const ValueKey('load-btn'),
                onPressed: _reload,
                icon: const Icon(Icons.refresh),
                tooltip: 'Load jobs',
              ),
            ]),
          ),
          Expanded(
            child: FutureBuilder<List<Order>>(
              future: _orders,
              builder: (context, snap) {
                if (snap.hasError) {
                  return ListView(children: [
                    ListTile(title: Text('Could not load jobs: ${snap.error}')),
                  ]);
                }
                if (!snap.hasData) {
                  return const Center(child: CircularProgressIndicator());
                }
                final orders = snap.data!;
                if (orders.isEmpty) {
                  return const Center(child: Text('No assigned jobs right now.'));
                }
                return ListView.builder(
                  itemCount: orders.length,
                  itemBuilder: (context, i) {
                    final o = orders[i];
                    final step = switch (o.status) {
                      statusConfirmed => ('Pick up', Icons.shopping_bag_outlined),
                      statusPickedUp => ('Deliver', Icons.local_shipping_outlined),
                      _ => (null, null),
                    };
                    return ListTile(
                      leading: Chip(label: Text(o.status)),
                      title: Text('Order ${o.id}'),
                      subtitle: Text(
                          'From ${o.shopName} · payout ${money(o.riderPayoutMinor)}'),
                      trailing: step.$1 == null
                          ? null
                          : FilledButton.icon(
                              key: ValueKey('${step.$1!.toLowerCase()}-${o.id}'),
                              onPressed: () => _advance(o),
                              icon: Icon(step.$2),
                              label: Text(step.$1!),
                            ),
                    );
                  },
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
