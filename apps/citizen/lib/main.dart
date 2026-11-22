import 'package:core/api.dart';
import 'package:core/design.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';

void main() => runApp(const CityMarketCitizenApp());

/// The customer app: product-first browsing, quantity picks, flat-fee
/// checkout, and a live order-status screen. All money and dispatch logic
/// lives on the Go server (server/finance, server/orders) — the client
/// shows what the API says, nothing more.
///
/// The API base URL is baked in with --dart-define=API_BASE_URL=... and
/// defaults to the Android-emulator host loopback (10.0.2.2 = host's
/// localhost from the emulator).
class CityMarketCitizenApp extends StatelessWidget {
  const CityMarketCitizenApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Citizen',
      theme: buildCMTheme(),
      home: CatalogScreen(
        api: HttpApiClient(
          const String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:3000'),
        ),
      ),
    );
  }
}

class CatalogScreen extends StatefulWidget {
  final ApiClient api;

  const CatalogScreen({super.key, required this.api});

  @override
  State<CatalogScreen> createState() => _CatalogScreenState();
}

class _CatalogScreenState extends State<CatalogScreen> {
  late Future<List<Product>> _products = widget.api.products();
  final _qty = <String, int>{};

  int get _count => _qty.values.fold(0, (a, b) => a + b);

  Future<void> _reload() async {
    // Block body on purpose: an arrow closure here would return the Future,
    // and setState() asserts against that.
    setState(() {
      _products = widget.api.products();
    });
  }

  Future<void> _checkout() async {
    final items = [for (final e in _qty.entries) OrderDraftItem(e.key, e.value)];
    final location = await _askLocation();
    if (location == null || !mounted) return;
    try {
      final order = await widget.api.placeOrder(items, location.$1, location.$2);
      if (!mounted) return;
      setState(_qty.clear);
      await Navigator.of(context).push(MaterialPageRoute(
        builder: (_) => OrderScreen(api: widget.api, order: order),
      ));
      await _reload();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<(double, double)?> _askLocation() async {
    final latCtrl = TextEditingController(text: '24.8607');
    final lngCtrl = TextEditingController(text: '67.0011');
    final ok = await showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        title: const Text('Delivery location'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: latCtrl,
              key: const ValueKey('lat-field'),
              decoration: const InputDecoration(labelText: 'Latitude'),
              keyboardType: TextInputType.number,
            ),
            TextField(
              controller: lngCtrl,
              key: const ValueKey('lng-field'),
              decoration: const InputDecoration(labelText: 'Longitude'),
              keyboardType: TextInputType.number,
            ),
          ],
        ),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(dialogCtx, false),
              child: const Text('Cancel')),
          FilledButton(
              key: const ValueKey('place-order-btn'),
              onPressed: () => Navigator.pop(dialogCtx, true),
              child: const Text('Place order')),
        ],
      ),
    );
    if (ok != true) return null;
    return (double.tryParse(latCtrl.text) ?? 0, double.tryParse(lngCtrl.text) ?? 0);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('City Market')),
      body: RefreshIndicator(
        onRefresh: _reload,
        child: FutureBuilder<List<Product>>(
          future: _products,
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
            final products = snap.data!;
            return ListView.builder(
              itemCount: products.length,
              itemBuilder: (context, i) {
                final p = products[i];
                final qty = _qty[p.id] ?? 0;
                return ListTile(
                  leading: const Icon(Icons.inventory_2_outlined),
                  title: Text(p.name),
                  subtitle: Text('${p.size} · ${money(p.priceMinor)}'),
                  trailing: qty == 0
                      ? IconButton(
                          key: ValueKey('add-${p.id}'),
                          icon: const Icon(Icons.add_shopping_cart),
                          tooltip: 'Add to order',
                          onPressed: () => setState(() => _qty[p.id] = 1),
                        )
                      : Row(mainAxisSize: MainAxisSize.min, children: [
                          IconButton(
                              onPressed: () => setState(() {
                                    if (qty == 1) {
                                      _qty.remove(p.id);
                                    } else {
                                      _qty[p.id] = qty - 1;
                                    }
                                  }),
                              icon: const Icon(Icons.remove)),
                          Text('$qty'),
                          IconButton(
                              onPressed: () => setState(() => _qty[p.id] = qty + 1),
                              icon: const Icon(Icons.add)),
                        ]),
                );
              },
            );
          },
        ),
      ),
      bottomNavigationBar: _count == 0
          ? null
          : SafeArea(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: FilledButton.icon(
                  key: const ValueKey('checkout-btn'),
                  onPressed: _checkout,
                  icon: const Icon(Icons.local_shipping_outlined),
                  label: Text('Order $_count item${_count == 1 ? '' : 's'}'),
                ),
              ),
            ),
    );
  }
}

class OrderScreen extends StatefulWidget {
  final ApiClient api;
  final Order order;

  const OrderScreen({super.key, required this.api, required this.order});

  @override
  State<OrderScreen> createState() => _OrderScreenState();
}

class _OrderScreenState extends State<OrderScreen> {
  late Order _order = widget.order;
  bool _refreshing = false;

  Future<void> _refresh() async {
    setState(() => _refreshing = true);
    try {
      final o = await widget.api.order(_order.id);
      if (mounted) setState(() => _order = o);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    } finally {
      if (mounted) setState(() => _refreshing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final o = _order;
    return Scaffold(
      appBar: AppBar(title: const Text('Your order')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Row(children: [
            Expanded(child: Text('Order ${o.id}', style: Theme.of(context).textTheme.titleMedium)),
            CMStatusChip(o.status),
            IconButton(
              key: const ValueKey('refresh-btn'),
              onPressed: _refreshing ? null : _refresh,
              icon: const Icon(Icons.refresh),
            ),
          ]),
          const SizedBox(height: 8),
          ListTile(
            leading: const Icon(Icons.store_outlined),
            title: Text(o.shopName),
            subtitle: const Text('Fulfilment shop'),
          ),
          ListTile(
            leading: const Icon(Icons.delivery_dining),
            title: Text(o.riderName),
            subtitle: const Text('Assigned rider'),
          ),
          const Divider(),
          for (final it in o.items)
            ListTile(
              title: Text(it.productId),
              subtitle: Text('${it.qty} × ${money(it.unitPriceMinor)}'),
              trailing: Text(money(it.qty * it.unitPriceMinor)),
            ),
          const Divider(),
          ListTile(title: const Text('Subtotal'), trailing: Text(money(o.subtotalMinor))),
          ListTile(title: const Text('Delivery fee (flat)'), trailing: Text(money(o.deliveryFeeMinor))),
          ListTile(
            title: Text('Total', style: Theme.of(context).textTheme.titleMedium),
            trailing: Text(money(o.totalMinor), style: Theme.of(context).textTheme.titleMedium),
          ),
          const SizedBox(height: 12),
          Text(
            'Halal note: the shop pays Ujrah commission monthly — you never '
            'pay it; your checkout adds one flat delivery fee only.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }
}
