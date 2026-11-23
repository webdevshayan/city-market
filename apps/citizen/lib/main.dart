import 'package:core/api.dart';
import 'package:core/design.dart';
import 'package:core/map.dart' as cmmap;
import 'package:core/models.dart';
import 'package:flutter/material.dart';
import 'package:latlong2/latlong.dart';

void main() => runApp(const CityMarketCitizenApp());

/// The customer app: product-first browsing with stock chips, category and
/// shop filters (ordering from one specific shop is one tap), a map-pin
/// checkout with a delivery address note, cash on delivery, and a live
/// order-status screen. All money and dispatch logic lives on the Go
/// server (server/finance, server/orders) — the client shows what the API
/// says, nothing more.
///
/// The API base URL is baked in with --dart-define=API_BASE_URL=...; the
/// default is the production deployment.
class CityMarketCitizenApp extends StatelessWidget {
  const CityMarketCitizenApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Citizen',
      theme: buildCMTheme(),
      home: CatalogScreen(
        api: HttpApiClient(
          const String.fromEnvironment('API_BASE_URL', defaultValue: 'https://cmserver.vercel.app'),
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
  List<Product> _all = [];
  String _category = '';
  String _shop = '';

  int get _count => _qty.values.fold(0, (a, b) => a + b);

  Future<void> _reload() async {
    // Block body on purpose: an arrow closure here would return the Future,
    // and setState() asserts against that.
    setState(() {
      _products = widget.api.products();
    });
  }

  void _syncAll(List<Product> products) {
    _all = products;
  }

  List<Product> get _visible => _all.where((p) {
        if (_category.isNotEmpty && p.category != _category) {
          return false;
        }
        if (_shop.isNotEmpty && p.shopId != _shop) {
          return false;
        }
        return true;
      }).toList();

  List<String> get _categories {
    final seen = <String>{};
    for (final p in _all) {
      seen.add(p.category);
    }
    return seen.toList();
  }

  List<Product> get _shops {
    final seen = <String, Product>{};
    for (final p in _all) {
      seen[p.shopId] = p;
    }
    return seen.values.toList();
  }

  Future<void> _checkout() async {
    final picked = await Navigator.of(context).push<DeliveryDetails>(MaterialPageRoute(
      fullscreenDialog: true,
      builder: (_) => CheckoutScreen(api: widget.api, products: _all, qty: Map.of(_qty)),
    ));
    if (picked == null || !mounted) {
      return;
    }
    setState(_qty.clear);
    await Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => OrderScreen(api: widget.api, order: picked.orders.first),
    ));
    await _reload();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('City Market Johi')),
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
            _syncAll(snap.data!);
            final products = _visible;
            return ListView(
              children: [
                _filterRow(),
                if (_shop.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    child: Text(
                      'Browsing one shop — your order goes to ${_shops.firstWhere((s) => s.shopId == _shop, orElse: () => Product(id: '', name: '', category: '', brand: '', size: '', image: '', shopId: _shop, priceMinor: 0)).brand} shop',
                      style: const TextStyle(fontSize: 12, color: CMColors.inkMuted),
                    ),
                  ),
                for (final p in products) _productTile(p),
                const SizedBox(height: 96),
              ],
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
                  label: Text('Checkout — $_count item${_count == 1 ? '' : 's'} · COD'),
                ),
              ),
            ),
    );
  }

  Widget _filterRow() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 8,
            runSpacing: 4,
            children: [
              ChoiceChip(
                label: const Text('Everything'),
                selected: _category.isEmpty,
                onSelected: (_) => setState(() => _category = ''),
              ),
              for (final c in _categories)
                ChoiceChip(
                  label: Text(displaySlug(c)),
                  selected: _category == c,
                  onSelected: (_) => setState(() => _category = c),
                ),
            ],
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 4,
            children: [
              ChoiceChip(
                key: const ValueKey('shop-all-chip'),
                label: const Text('Any shop'),
                selected: _shop.isEmpty,
                onSelected: (_) => setState(() => _shop = ''),
              ),
              for (final s in _shops)
                ChoiceChip(
                  key: ValueKey('shop-${s.shopId}'),
                  label: Text(displaySlug(s.brand)),
                  selected: _shop == s.shopId,
                  onSelected: (_) => setState(() => _shop = s.shopId),
                ),
            ],
          ),
          const Divider(height: 24),
        ],
      ),
    );
  }

  Widget _productTile(Product p) {
    final qty = _qty[p.id] ?? 0;
    final outOfStock = !p.inStock;
    return ListTile(
      leading: const Icon(Icons.inventory_2_outlined),
      title: Text(p.name),
      subtitle: Text(
          '${p.size} · ${money(p.priceMinor)} PKR · ${outOfStock ? 'Out of stock' : 'In stock: ${p.stock}'}',
          style: TextStyle(
              color: outOfStock ? CMColors.danger : CMColors.inkMuted,
              fontWeight: outOfStock ? FontWeight.w700 : FontWeight.w400)),
      trailing: outOfStock
          ? const Chip(label: Text('Out of stock'))
          : qty == 0
              ? IconButton(
                  key: ValueKey('add-${p.id}'),
                  icon: const Icon(Icons.add_shopping_cart),
                  tooltip: 'Add to order',
                  onPressed: () => setState(() => _qty[p.id] = 1),
                )
              : Row(mainAxisSize: MainAxisSize.min, children: [
                  IconButton(
                      key: ValueKey('remove-${p.id}'),
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
                      key: ValueKey('more-${p.id}'),
                      onPressed: () => setState(() {
                            if (qty < p.stock) {
                              _qty[p.id] = qty + 1;
                            }
                          }),
                      icon: const Icon(Icons.add)),
                ]),
    );
  }
}

/// Checkout: one delivery pin + address note, the cart grouped per shop (a
/// mixed cart becomes one order per shop — the market's rule), cash on
/// delivery, flat delivery fee per order.
class CheckoutScreen extends StatefulWidget {
  final ApiClient api;
  final List<Product> products;
  final Map<String, int> qty;

  const CheckoutScreen({super.key, required this.api, required this.products, required this.qty});

  @override
  State<CheckoutScreen> createState() => _CheckoutScreenState();
}

class DeliveryDetails {
  final List<Order> orders;
  const DeliveryDetails(this.orders);
}

class _CheckoutScreenState extends State<CheckoutScreen> {
  LatLng _pin = cmmap.johiCenter;
  final _address = TextEditingController();
  bool _busy = false;
  String? _error;

  Map<String, List<Product>> get _groups {
    final out = <String, List<Product>>{};
    for (final e in widget.qty.entries) {
      final p = widget.products.firstWhere((p) => p.id == e.key,
          orElse: () => const Product(id: '', name: '', category: '', brand: '', size: '', image: '', shopId: '', priceMinor: 0));
      out.putIfAbsent(p.shopId, () => []).add(p);
    }
    return out;
  }

  Future<void> _pickOnMap() async {
    final picked = await Navigator.of(context).push<LatLng>(MaterialPageRoute(
      fullscreenDialog: true,
      builder: (_) => cmmap.CMLocationPicker(initial: _pin, onPicked: (p) => Navigator.pop(context, p)),
    ));
    if (picked != null) {
      setState(() => _pin = picked);
    }
  }

  Future<void> _place() async {
    if (_address.text.trim().isEmpty) {
      setState(() => _error = 'Add a short delivery address — the rider reads it before setting out.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    final placed = <Order>[];
    try {
      for (final entry in _groups.entries) {
        final items = [
          for (final p in entry.value) OrderDraftItem(p.id, widget.qty[p.id] ?? 1),
        ];
        final o = await widget.api.placeOrder(
          items,
          _pin.latitude,
          _pin.longitude,
          shopId: entry.key,
          addressNote: _address.text.trim(),
        );
        placed.add(o);
      }
      if (!mounted) return;
      Navigator.of(context).pop(DeliveryDetails(placed));
    } on ApiException catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e.message;
        });
      }
    }
  }

  @override
  void dispose() {
    _address.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final groups = _groups.entries.toList();
    return Scaffold(
      appBar: AppBar(title: const Text('Checkout — cash on delivery')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          const CMSectionHeader('Deliver to'),
          Container(
            decoration: BoxDecoration(border: Border.all(color: CMColors.hairline)),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                SizedBox(
                  height: 150,
                  child: cmmap.CMMiniMap(
                    route: cmmap.CMRoute(
                      pickup: cmmap.johiCenter,
                      drop: _pin,
                      pickupName: 'Johi bazaar',
                      dropName: 'your door',
                    ),
                    height: 150,
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.all(12),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Pin: ${_pin.latitude.toStringAsFixed(5)}, ${_pin.longitude.toStringAsFixed(5)}',
                        style: const TextStyle(fontSize: 12, color: CMColors.inkMuted),
                      ),
                      const SizedBox(height: 8),
                      OutlinedButton.icon(
                        key: const ValueKey('pick-location-btn'),
                        onPressed: _pickOnMap,
                        icon: const Icon(Icons.pin_drop_outlined),
                        label: const Text('Choose the spot on the map'),
                      ),
                      const SizedBox(height: 12),
                      TextField(
                        key: const ValueKey('address-field'),
                        controller: _address,
                        decoration: const InputDecoration(
                          labelText: 'Delivery address note',
                          hintText: 'Street, colour of the gate, landmark…',
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
          const CMSectionHeader('Your order — per shop'),
          for (final g in groups) ...[
            ListTile(
              leading: const Icon(Icons.store_outlined),
              title: Text(displaySlug(widget.products.firstWhere((p) => p.shopId == g.key, orElse: () => g.value.first).brand)),
              subtitle: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  for (final p in g.value)
                    Text('${widget.qty[p.id]} × ${p.name} — ${money(p.priceMinor * widget.qty[p.id]!)} PKR'),
                ],
              ),
            ),
            const Divider(),
          ],
          const ListTile(
            title: Text('Payment'),
            trailing: Text('Cash on delivery', style: TextStyle(fontWeight: FontWeight.w700)),
          ),
          if (groups.length > 1)
            Padding(
              padding: const EdgeInsets.all(12),
              child: Text(
                'Your cart spans ${groups.length} shops, so it becomes ${groups.length} orders — '
                'each with its own flat delivery fee and its own rider.',
                style: const TextStyle(color: CMColors.inkMuted, fontSize: 13),
              ),
            ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Text(_error!, style: const TextStyle(color: CMColors.danger)),
            ),
          FilledButton.icon(
            key: const ValueKey('place-order-btn'),
            onPressed: _busy ? null : _place,
            icon: const Icon(Icons.check),
            label: Text(_busy ? 'Placing…' : 'Place ${groups.length > 1 ? "${groups.length} orders" : "order"}'),
          ),
          const SizedBox(height: 12),
          const Text(
            'You pay the rider at the door. Shops pay the platform\u2019s monthly '
            'Ujrah — it never lands on your checkout; only the flat delivery '
            'fee does. Map data © OpenStreetMap contributors.',
            style: TextStyle(fontSize: 11, color: CMColors.inkMuted),
          ),
        ],
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
            title: Text(o.riderName.isEmpty ? 'Waiting for a rider to claim the job' : o.riderName),
            subtitle: Text(o.riderName.isEmpty ? 'A nearby rider sees your job with the route and distance' : 'Your rider'),
          ),
          if (o.addressNote.isNotEmpty)
            ListTile(
              leading: const Icon(Icons.pin_drop_outlined),
              title: Text(o.addressNote),
              subtitle: const Text('Delivery address'),
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
            title: Text('Total — cash on delivery', style: Theme.of(context).textTheme.titleMedium),
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
