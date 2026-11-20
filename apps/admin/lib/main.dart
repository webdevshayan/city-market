import 'package:core/api.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';

void main() => runApp(const CityMarketAdminApp());

/// The admin console: manages shops, riders and the catalog against the Go
/// server's admin API, and renders the settlement view of the FIXED Ujrah
/// schedule (the schedule is computed by server/finance — it is never
/// stored or edited). The server default-denies the whole console until
/// ADMIN_TOKEN is set; the token lives only in this app's memory and travels
/// as the X-Admin-Token header.
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
      home: AdminHomeScreen(
        api: HttpApiClient(
          const String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:3000'),
        ),
      ),
    );
  }
}

class AdminHomeScreen extends StatefulWidget {
  final ApiClient api;

  const AdminHomeScreen({super.key, required this.api});

  @override
  State<AdminHomeScreen> createState() => _AdminHomeScreenState();
}

class _AdminHomeScreenState extends State<AdminHomeScreen> {
  final _token = TextEditingController(text: '');
  late Future<List<Place>> _shops = _loadShops();
  late Future<List<Place>> _riders = _loadRiders();
  late Future<List<Product>> _products = _loadProducts();

  // shop form
  final _shopId = TextEditingController(text: 's-dha');
  final _shopName = TextEditingController(text: 'DHA Store');
  final _shopLat = TextEditingController(text: '24.825');
  final _shopLng = TextEditingController(text: '67.070');
  bool _shopActive = true;

  // product form
  final _productId = TextEditingController(text: 'gajar-1kg');
  final _productName = TextEditingController(text: 'Gajar ka Halal Mix');
  final _productPrice = TextEditingController(text: '50000');
  final _productShop = TextEditingController(text: 's-dha');
  final _productCategory = TextEditingController(text: 'grocery');
  final _productBrand = TextEditingController(text: 'noor');
  final _productSize = TextEditingController(text: '1kg');

  // commission calculator
  final _commOrders = TextEditingController(text: '250');
  final _commSales = TextEditingController(text: '10000000');
  CommissionView? _commission;

  String get _t => _token.text.trim();

  Future<List<Place>> _loadShops() => widget.api.adminShops(_t);
  Future<List<Place>> _loadRiders() => widget.api.adminRiders(_t);
  Future<List<Product>> _loadProducts() => widget.api.adminProducts(_t);

  void _reload() {
    setState(() {
      _shops = _loadShops();
      _riders = _loadRiders();
      _products = _loadProducts();
    });
  }

  void _snack(String message) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
  }

  Future<void> _saveShop() async {
    try {
      await widget.api.adminUpsertShop(_t,
          id: _shopId.text.trim(),
          name: _shopName.text.trim(),
          lat: double.tryParse(_shopLat.text) ?? 0,
          lng: double.tryParse(_shopLng.text) ?? 0,
          active: _shopActive);
      _snack('Shop saved');
      _reload();
    } on ApiException catch (e) {
      _snack(e.message);
    }
  }

  Future<void> _saveProduct() async {
    try {
      await widget.api.adminUpsertProduct(_t,
          id: _productId.text.trim(),
          name: _productName.text.trim(),
          category: _productCategory.text.trim(),
          brand: _productBrand.text.trim(),
          size: _productSize.text.trim(),
          priceMinor: int.tryParse(_productPrice.text) ?? 0,
          shopId: _productShop.text.trim());
      _snack('Product saved — live in the shop immediately');
      _reload();
    } on ApiException catch (e) {
      _snack(e.message);
    }
  }

  Future<void> _calcCommission() async {
    try {
      final c = await widget.api.commission(_t,
          orders: int.tryParse(_commOrders.text) ?? 0,
          salesMinor: int.tryParse(_commSales.text) ?? 0);
      setState(() => _commission = c);
    } on ApiException catch (e) {
      _snack(e.message);
    }
  }

  @override
  void dispose() {
    for (final c in [
      _token, _shopId, _shopName, _shopLat, _shopLng,
      _productId, _productName, _productPrice, _productShop,
      _productCategory, _productBrand, _productSize, _commOrders, _commSales,
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Admin console')),
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          TextField(
            controller: _token,
            key: const ValueKey('admin-token'),
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Admin token (ADMIN_TOKEN on the server)',
              border: OutlineInputBorder(),
            ),
            onSubmitted: (_) => _reload(),
          ),
          IconButton.filledTonal(
            key: const ValueKey('reload-btn'),
            onPressed: _reload,
            icon: const Icon(Icons.refresh),
            tooltip: 'Reload lists',
          ),

          _header('Shops'),
          FutureBuilder<List<Place>>(
            future: _shops,
            builder: (context, snap) => _placeList(snap, (p) => p.active ? null : 'closed'),
          ),
          Row(children: [
            _field(_shopId, 'ID', 'shop-id'),
            _field(_shopName, 'Name', 'shop-name'),
            _field(_shopLat, 'Lat', 'shop-lat'),
            _field(_shopLng, 'Lng', 'shop-lng'),
          ]),
          Row(children: [
            Switch(key: const ValueKey('shop-active'), value: _shopActive, onChanged: (v) => setState(() => _shopActive = v)),
            const Text('Open'),
            const Spacer(),
            FilledButton(
              key: const ValueKey('shop-save'),
              onPressed: _saveShop,
              child: const Text('Save shop'),
            ),
          ]),

          _header('Riders'),
          FutureBuilder<List<Place>>(
            future: _riders,
            builder: (context, snap) => _placeList(snap, (p) => p.active ? null : 'off duty'),
          ),

          _header('Catalog'),
          FutureBuilder<List<Product>>(
            future: _products,
            builder: (context, snap) => _productList(snap),
          ),
          Row(children: [
            _field(_productId, 'ID', 'product-id'),
            _field(_productName, 'Name', 'product-name'),
            _field(_productPrice, 'Price (minor)', 'product-price'),
            _field(_productShop, 'Shop', 'product-shop'),
          ]),
          FilledButton(
            key: const ValueKey('product-save'),
            onPressed: _saveProduct,
            child: Text('Save product (${_productCategory.text}/${_productBrand.text}/${_productSize.text})'),
          ),

          _header('Ujrah commission (fixed schedule — settlement view)'),
          Row(children: [
            _field(_commOrders, 'Monthly orders', 'commission-orders'),
            _field(_commSales, 'Sales (minor)', 'commission-sales'),
            FilledButton(
              key: const ValueKey('commission-go'),
              onPressed: _calcCommission,
              child: const Text('Compute'),
            ),
          ]),
          if (_commission != null)
            ListTile(
              key: const ValueKey('result-commission'),
              title: Text('Commission: ${money(_commission!.commissionMinor)} '
                  '(${_commission!.rateText}${_commission!.capped ? ', capped' : ''})'),
            ),
        ],
      ),
    );
  }

  Widget _header(String text) => Padding(
        padding: const EdgeInsets.only(top: 16, bottom: 4),
        child: Text(text, style: Theme.of(context).textTheme.titleMedium),
      );

  Widget _field(TextEditingController ctrl, String label, String key) => Expanded(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 4),
          child: TextField(
            controller: ctrl,
            key: ValueKey(key),
            decoration: InputDecoration(labelText: label, border: const OutlineInputBorder()),
            keyboardType: TextInputType.text,
          ),
        ),
      );

  Widget _placeList(AsyncSnapshot<List<Place>> snap, String? Function(Place) tag) {
    if (snap.hasError) return ListTile(title: Text('Load failed: ${snap.error}'));
    if (!snap.hasData) return const LinearProgressIndicator();
    return Column(
      children: [
        for (final p in snap.data!)
          ListTile(
            dense: true,
            leading: Icon(p.active ? Icons.check_circle : Icons.cancel, size: 18),
            title: Text(p.name),
            subtitle: Text('${p.id} · ${p.lat}, ${p.lng}'),
            trailing: tag(p) == null ? null : Chip(label: Text(tag(p)!)),
          ),
      ],
    );
  }

  Widget _productList(AsyncSnapshot<List<Product>> snap) {
    if (snap.hasError) return ListTile(title: Text('Load failed: ${snap.error}'));
    if (!snap.hasData) return const LinearProgressIndicator();
    return Column(
      children: [
        for (final p in snap.data!)
          ListTile(
            dense: true,
            title: Text(p.name),
            subtitle: Text('${p.id} · ${p.shopId}'),
            trailing: Text(money(p.priceMinor)),
          ),
      ],
    );
  }
}
