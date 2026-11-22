import 'package:core/api.dart';
import 'package:core/design.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';

void main() => runApp(const CityMarketShopApp());

/// The shop dashboard: the order queue for one shop, with the confirm step
/// of the lifecycle (placed → confirmed). Commission statements are the
/// server's job (fixed Ujrah schedule) — the dashboard only shows what the
/// API computes.
class CityMarketShopApp extends StatelessWidget {
  const CityMarketShopApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Shop',
      theme: buildCMTheme(),
      home: ShopHomeScreen(
        api: HttpApiClient(
          const String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:3000'),
        ),
      ),
    );
  }
}

class ShopHomeScreen extends StatefulWidget {
  final ApiClient api;

  const ShopHomeScreen({super.key, required this.api});

  @override
  State<ShopHomeScreen> createState() => _ShopHomeScreenState();
}

class _ShopHomeScreenState extends State<ShopHomeScreen> {
  final _shopId = TextEditingController(text: 's-central');
  late Future<List<Order>> _orders = _load();

  Future<List<Order>> _load() => widget.api.shopOrders(_shopId.text.trim());

  Future<void> _reload() async {
    // Block body on purpose: an arrow closure here would return the Future,
    // and setState() asserts against that.
    setState(() {
      _orders = _load();
    });
  }

  Future<void> _confirm(Order o) async {
    try {
      await widget.api.confirmOrder(o.id);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
    await _reload();
  }

  @override
  void dispose() {
    _shopId.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Shop dashboard')),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(children: [
              Expanded(
                child: TextField(
                  controller: _shopId,
                  key: const ValueKey('shop-id-field'),
                  decoration: const InputDecoration(
                    labelText: 'Shop ID',
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
                tooltip: 'Load orders',
              ),
            ]),
          ),
          Expanded(
            child: FutureBuilder<List<Order>>(
              future: _orders,
              builder: (context, snap) {
                if (snap.hasError) {
                  return ListView(
                    children: [
                      const SizedBox(height: 80),
                      CMErrorRetry(error: snap.error!, onRetry: _reload),
                    ],
                  );
                }
                if (!snap.hasData) {
                  return const Center(child: CircularProgressIndicator());
                }
                final orders = snap.data!;
                if (orders.isEmpty) {
                  return const Center(child: Text('No orders for this shop yet.'));
                }
                return ListView.builder(
                  itemCount: orders.length,
                  itemBuilder: (context, i) {
                    final o = orders[i];
                    return ListTile(
                      leading: CMStatusChip(o.status),
                      title: Text('Order ${o.id}'),
                      subtitle: Text('${o.items.length} line(s) · ${money(o.totalMinor)}'),
                      trailing: o.status == statusPlaced
                          ? FilledButton(
                              key: ValueKey('confirm-${o.id}'),
                              onPressed: () => _confirm(o),
                              child: const Text('Confirm'),
                            )
                          : null,
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
