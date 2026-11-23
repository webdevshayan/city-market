import 'package:core/api.dart';
import 'package:core/design.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() => runApp(const CityMarketShopApp());

/// The shop owner's app: sign in (or register the shop), manage the shelf
/// with stock, accept or cancel incoming orders, and see the subscription
/// state that decides whether the market dispatches to the shop this month.
class CityMarketShopApp extends StatelessWidget {
  const CityMarketShopApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Shop',
      theme: buildCMTheme(),
      home: SessionGate(api: HttpApiClient(
        const String.fromEnvironment('API_BASE_URL', defaultValue: 'https://cmserver.vercel.app'),
      )),
    );
  }
}

/// Chooses between the sign-in screen and the dashboard based on the saved
/// bearer token (shared_preferences survives restarts).
class SessionGate extends StatefulWidget {
  final ApiClient api;

  const SessionGate({super.key, required this.api});

  @override
  State<SessionGate> createState() => _SessionGateState();
}

class _SessionGateState extends State<SessionGate> {
  String? _token;
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _restore();
  }

  Future<void> _restore() async {
    final prefs = await SharedPreferences.getInstance();
    if (mounted) {
      setState(() {
        _token = prefs.getString('shop_token');
        _ready = true;
      });
    }
  }

  Future<void> _onSignedIn(String token) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('shop_token', token);
    setState(() => _token = token);
  }

  Future<void> _onSignOut() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove('shop_token');
    setState(() => _token = null);
  }

  @override
  Widget build(BuildContext context) {
    if (!_ready) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (_token == null) {
      return SignInScreen(api: widget.api, onSignedIn: _onSignedIn);
    }
    return ShopDashboard(api: widget.api, token: _token!, onSignOut: _onSignOut);
  }
}

/// Sign in with phone + password, or register a new shop. Registration
/// answers with a live token and lands the shop in the market — unpaid,
/// so the admin's monthly mark still has to switch ordering on.
class SignInScreen extends StatefulWidget {
  final ApiClient api;
  final ValueChanged<String> onSignedIn;

  const SignInScreen({super.key, required this.api, required this.onSignedIn});

  @override
  State<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends State<SignInScreen> {
  final _phone = TextEditingController();
  final _password = TextEditingController();
  final _name = TextEditingController();
  bool _registering = false;
  bool _busy = false;
  String? _error;

  Future<void> _submit() async {
    // Block body on purpose: an arrow closure here would return the Future.
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final AuthResult r;
      if (_registering) {
        r = await widget.api.registerShop(
          name: _name.text.trim(),
          phone: _phone.text.trim(),
          password: _password.text,
          lat: johiCenterLat,
          lng: johiCenterLng,
        );
      } else {
        r = await widget.api.login(role: 'shop', phone: _phone.text.trim(), password: _password.text);
      }
      if (mounted) {
        widget.onSignedIn(r.token);
      }
    } on ApiException catch (e) {
      if (mounted) {
        setState(() {
          _error = e.message;
          _busy = false;
        });
      }
      return;
    }
    if (mounted) {
      setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _phone.dispose();
    _password.dispose();
    _name.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('City Market — Shop owner')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          CMSectionHeader(_registering ? 'Register your shop' : 'Sign in'),
          if (_registering) ...[
            TextField(
              key: const ValueKey('shop-name-field'),
              controller: _name,
              decoration: const InputDecoration(labelText: 'Shop name'),
            ),
            const SizedBox(height: 12),
          ],
          TextField(
            key: const ValueKey('shop-phone-field'),
            controller: _phone,
            keyboardType: TextInputType.phone,
            decoration: const InputDecoration(
              labelText: 'Mobile number',
              hintText: '03001234567',
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('shop-password-field'),
            controller: _password,
            obscureText: true,
            decoration: const InputDecoration(labelText: 'Password (8+ characters)'),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!,
                style: const TextStyle(color: CMColors.danger, fontWeight: FontWeight.w600)),
          ],
          const SizedBox(height: 16),
          FilledButton(
            key: const ValueKey('shop-submit-btn'),
            onPressed: _busy ? null : _submit,
            child: Text(_busy
                ? 'Working…'
                : _registering
                    ? 'Register shop'
                    : 'Sign in'),
          ),
          const SizedBox(height: 8),
          TextButton(
            key: const ValueKey('shop-toggle-mode'),
            onPressed: _busy
                ? null
                : () {
                    setState(() {
                      _registering = !_registering;
                      _error = null;
                    });
                  },
            child: Text(_registering
                ? 'Already have an account? Sign in'
                : 'New here? Register your shop'),
          ),
          if (_registering) ...[
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                border: Border.all(color: CMColors.hairline),
                color: CMColors.wash,
              ),
              child: const Text(
                'New shops start unpaid: browse the app freely, but orders arrive '
                'only after the market admin marks this month paid (payment is '
                'settled with the admin directly — EasyPaisa/JazzCash coming). '
                'The shop pins to Johi bazaar for now; tell the admin the exact '
                'location and it gets fixed on the map.',
                style: TextStyle(color: CMColors.inkMuted, fontSize: 13),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

/// Johi town center, duplicated from the map module to keep this app from
/// dragging the whole map package into its login screen.
const johiCenterLat = 26.692061;
const johiCenterLng = 67.613300;

/// The signed-in dashboard: subscription banner, shelf manager and the
/// order queue in one scroll.
class ShopDashboard extends StatefulWidget {
  final ApiClient api;
  final String token;
  final VoidCallback onSignOut;

  const ShopDashboard({super.key, required this.api, required this.token, required this.onSignOut});

  @override
  State<ShopDashboard> createState() => _ShopDashboardState();
}

class _ShopDashboardState extends State<ShopDashboard> {
  late Future<ShopAccount> _me = _loadMe();
  late Future<List<Product>> _shelf = widget.api.shopProducts(widget.token);
  late Future<List<Order>> _orders = widget.api.shopOrders(widget.token);

  Future<ShopAccount> _loadMe() => widget.api.shopMe(widget.token);

  Future<void> _reload({bool meToo = true}) async {
    // Block body on purpose: an arrow closure here would return the Future.
    setState(() {
      _shelf = widget.api.shopProducts(widget.token);
      _orders = widget.api.shopOrders(widget.token);
      if (meToo) {
        _me = _loadMe();
      }
    });
  }

  Future<void> _accept(Order o) async {
    try {
      await widget.api.acceptOrder(widget.token, o.id);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
    await _reload();
  }

  Future<void> _cancel(Order o) async {
    try {
      await widget.api.cancelOrder(widget.token, o.id);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
    await _reload();
  }

  Future<void> _openProductForm([Product? existing]) async {
    final created = await Navigator.of(context).push<bool>(MaterialPageRoute(
      builder: (_) => ProductFormScreen(api: widget.api, token: widget.token, existing: existing),
    ));
    if (created == true) {
      await _reload(meToo: false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Shop dashboard'),
        actions: [
          IconButton(
            tooltip: 'Sign out',
            onPressed: widget.onSignOut,
            icon: const Icon(Icons.logout),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => _reload(),
        child: ListView(
          children: [
            FutureBuilder<ShopAccount>(
              future: _me,
              builder: (context, snap) {
                if (snap.hasError) {
                  return CMErrorRetry(error: snap.error!, onRetry: () => _reload());
                }
                if (!snap.hasData) {
                  return const Padding(
                      padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()));
                }
                final acc = snap.data!;
                return SubscriptionBanner(account: acc);
              },
            ),
            const CMSectionHeader('Incoming orders'),
            FutureBuilder<List<Order>>(
              future: _orders,
              builder: (context, snap) {
                if (snap.hasError) {
                  return CMErrorRetry(error: snap.error!, onRetry: () => _reload(meToo: false));
                }
                if (!snap.hasData) {
                  return const Padding(
                      padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()));
                }
                final orders = snap.data!;
                if (orders.isEmpty) {
                  return const Padding(
                    padding: EdgeInsets.all(16),
                    child: Text('No orders yet — they appear here the moment a customer checks out.'),
                  );
                }
                return Column(
                  children: [
                    for (final o in orders)
                      ListTile(
                        leading: CMStatusChip(o.status),
                        title: Text('Order ${o.id.substring(0, 10)}…'),
                        subtitle: Text(
                            '${o.items.length} line(s) · ${money(o.totalMinor)} PKR · COD'
                            '${o.addressNote.isEmpty ? '' : '\n${o.addressNote}'}'),
                        isThreeLine: o.addressNote.isNotEmpty,
                        trailing: o.status == statusPlaced
                            ? Row(mainAxisSize: MainAxisSize.min, children: [
                                FilledButton(
                                  key: ValueKey('accept-${o.id}'),
                                  onPressed: () => _accept(o),
                                  child: const Text('Accept'),
                                ),
                                const SizedBox(width: 8),
                                OutlinedButton(
                                  key: ValueKey('cancel-${o.id}'),
                                  onPressed: () => _cancel(o),
                                  child: const Text('Cancel'),
                                ),
                              ])
                            : null,
                      ),
                  ],
                );
              },
            ),
            const CMSectionHeader('My shelf'),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: FilledButton.icon(
                key: const ValueKey('add-product-btn'),
                onPressed: () => _openProductForm(),
                icon: const Icon(Icons.add),
                label: const Text('Add product'),
              ),
            ),
            FutureBuilder<List<Product>>(
              future: _shelf,
              builder: (context, snap) {
                if (snap.hasError) {
                  return CMErrorRetry(error: snap.error!, onRetry: () => _reload(meToo: false));
                }
                if (!snap.hasData) {
                  return const Padding(
                      padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()));
                }
                final products = snap.data!;
                if (products.isEmpty) {
                  return const Padding(
                    padding: EdgeInsets.all(16),
                    child: Text('The shelf is empty — add your first product above.'),
                  );
                }
                return Column(
                  children: [
                    for (final p in products)
                      ListTile(
                        title: Text('${p.name} · ${p.size}'),
                        subtitle: Text('${p.priceText} PKR · stock ${p.stock}'),
                        trailing: OutlinedButton(
                          onPressed: () => _openProductForm(p),
                          child: const Text('Edit'),
                        ),
                      ),
                  ],
                );
              },
            ),
            const SizedBox(height: 24),
          ],
        ),
      ),
    );
  }
}

/// The subscription state, impossible to miss: green when the market is
/// dispatching to this shop this month, a loud red wall when not — with
/// exactly what to do about it.
class SubscriptionBanner extends StatelessWidget {
  final ShopAccount account;

  const SubscriptionBanner({super.key, required this.account});

  @override
  Widget build(BuildContext context) {
    if (account.eligible) {
      return Container(
        margin: const EdgeInsets.all(16),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(border: Border.all(color: CMColors.green)),
        child: Row(
          children: [
            const Icon(Icons.verified_outlined, color: CMColors.green),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                'Taking orders — paid through ${account.paidThrough}. '
                'Ujrah settles monthly with the admin.',
                style: const TextStyle(color: CMColors.greenDeep, fontWeight: FontWeight.w600),
              ),
            ),
          ],
        ),
      );
    }
    return Container(
      margin: const EdgeInsets.all(16),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(border: Border.all(color: CMColors.danger, width: 2)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.pause_circle_outline, color: CMColors.danger),
              const SizedBox(width: 10),
              Expanded(
                child: Text('Not taking orders — ${_reason(account)}',
                    style: const TextStyle(color: CMColors.danger, fontWeight: FontWeight.w700)),
              ),
            ],
          ),
          const SizedBox(height: 6),
          const Text(
            'Customers can see the shop, but checkouts skip it until the admin '
            'marks this month paid. Settle the monthly Ujrah with the admin '
            '(cash, EasyPaisa or JazzCash) and it switches back on — no app '
            'update needed.',
            style: TextStyle(color: CMColors.inkMuted, fontSize: 13),
          ),
        ],
      ),
    );
  }

  String _reason(ShopAccount a) =>
      !a.active ? 'the shop is closed' : 'the monthly payment is not marked yet';
}

/// Add / edit one listing. The same form serves both, price in PKR.
class ProductFormScreen extends StatefulWidget {
  final ApiClient api;
  final String token;
  final Product? existing;

  const ProductFormScreen({super.key, required this.api, required this.token, this.existing});

  @override
  State<ProductFormScreen> createState() => _ProductFormScreenState();
}

class _ProductFormScreenState extends State<ProductFormScreen> {
  late final TextEditingController _name =
      TextEditingController(text: widget.existing?.name ?? '');
  late final TextEditingController _category =
      TextEditingController(text: widget.existing?.category ?? 'grocery');
  late final TextEditingController _brand =
      TextEditingController(text: widget.existing?.brand ?? '');
  late final TextEditingController _size =
      TextEditingController(text: widget.existing?.size ?? '');
  late final TextEditingController _price = TextEditingController(
      text: widget.existing == null ? '' : widget.existing!.priceText);
  late final TextEditingController _stock =
      TextEditingController(text: widget.existing == null ? '' : '${widget.existing!.stock}');
  bool _busy = false;
  String? _error;

  Future<void> _submit() async {
    final priceMinor = (double.tryParse(_price.text.trim()) ?? -1) * 100;
    final stock = int.tryParse(_stock.text.trim());
    if (_name.text.trim().isEmpty ||
        _category.text.trim().isEmpty ||
        _brand.text.trim().isEmpty ||
        _size.text.trim().isEmpty ||
        priceMinor < 100 ||
        stock == null ||
        stock < 0) {
      setState(() => _error =
          'Every field matters: name, category, brand, size, a price of at least 1 PKR, and a stock of 0 or more.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      if (widget.existing == null) {
        await widget.api.shopCreateProduct(widget.token,
            name: _name.text.trim(),
            category: _category.text.trim().toLowerCase(),
            brand: _brand.text.trim().toLowerCase().replaceAll(' ', '-'),
            size: _size.text.trim().toLowerCase(),
            priceMinor: priceMinor.round(),
            stock: stock);
      } else {
        await widget.api.shopUpdateProduct(widget.token,
            id: widget.existing!.id,
            name: _name.text.trim(),
            category: _category.text.trim().toLowerCase(),
            brand: _brand.text.trim().toLowerCase().replaceAll(' ', '-'),
            size: _size.text.trim().toLowerCase(),
            priceMinor: priceMinor.round(),
            stock: stock);
      }
      if (mounted) {
        Navigator.of(context).pop(true);
      }
    } on ApiException catch (e) {
      if (mounted) {
        setState(() {
          _error = e.message;
          _busy = false;
        });
      }
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _category.dispose();
    _brand.dispose();
    _size.dispose();
    _price.dispose();
    _stock.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(widget.existing == null ? 'Add product' : 'Edit product')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          TextField(
            key: const ValueKey('pf-name'),
            controller: _name,
            decoration: const InputDecoration(labelText: 'Product name'),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('pf-category'),
            controller: _category,
            decoration: const InputDecoration(
                labelText: 'Category (grocery, meat, bakery…)', hintText: 'grocery'),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('pf-brand'),
            controller: _brand,
            decoration: const InputDecoration(labelText: 'Brand / shop label'),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('pf-size'),
            controller: _size,
            decoration: const InputDecoration(labelText: 'Pack size', hintText: '5kg / 12pcs'),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('pf-price'),
            controller: _price,
            keyboardType:
                const TextInputType.numberWithOptions(decimal: true),
            decoration: const InputDecoration(labelText: 'Price (PKR)', hintText: '2200'),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('pf-stock'),
            controller: _stock,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(labelText: 'Stock (units on the shelf)'),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: const TextStyle(color: CMColors.danger)),
          ],
          const SizedBox(height: 16),
          FilledButton(
            key: const ValueKey('pf-submit'),
            onPressed: _busy ? null : _submit,
            child: Text(_busy ? 'Saving…' : 'Save'),
          ),
        ],
      ),
    );
  }
}
