import 'package:core/api.dart';
import 'package:core/design.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';

void main() => runApp(const CityMarketAdminApp());

/// The admin console, stats-only by design: revenue (GMV and the platform's
/// Ujrah), rider payouts, and who is working and earning. Catalog and place
/// management moved back to the server owner's tooling — the console reads
/// one endpoint, GET /v1/admin/stats, guarded by ADMIN_TOKEN. The token
/// lives only in this app's memory and travels as the X-Admin-Token header.
class CityMarketAdminApp extends StatelessWidget {
  const CityMarketAdminApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Admin',
      theme: buildCMTheme(),
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
  String? _tokenValue; // null until the admin signs in
  late Future<AdminStats> _stats = _load();

  Future<AdminStats> _load() => widget.api.adminStats(_tokenValue ?? '');

  void _signIn() {
    final t = _token.text.trim();
    if (t.isEmpty) return;
    setState(() {
      _tokenValue = t;
      _stats = _load();
    });
  }

  void _signOut() => setState(() => _tokenValue = null);

  void _reload() => setState(() => _stats = _load());

  @override
  void dispose() {
    _token.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return _tokenValue == null ? _login() : _dashboard();
  }

  Widget _login() {
    return Scaffold(
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const CMSectionHeader('City Market admin'),
                const Text(
                  'Stats only: revenue, payouts and worker earnings. '
                  'Sign in with the server ADMIN_TOKEN.',
                ),
                const SizedBox(height: 16),
                TextField(
                  controller: _token,
                  key: const ValueKey('admin-token'),
                  obscureText: true,
                  autofillHints: const [AutofillHints.password],
                  decoration: const InputDecoration(
                    labelText: 'Admin token',
                  ),
                  onSubmitted: (_) => _signIn(),
                ),
                const SizedBox(height: 12),
                FilledButton(
                  key: const ValueKey('stats-go'),
                  onPressed: _signIn,
                  child: const Text('View stats'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _dashboard() {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Admin — stats'),
        actions: [
          IconButton(
            key: const ValueKey('reload-btn'),
            tooltip: 'Refresh stats',
            onPressed: _reload,
            icon: const Icon(Icons.refresh),
          ),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: TextButton(
              key: const ValueKey('signout-btn'),
              onPressed: _signOut,
              child: const Text('Sign out'),
            ),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => _reload(),
        child: FutureBuilder<AdminStats>(
          future: _stats,
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
            return _statsList(snap.data!);
          },
        ),
      ),
    );
  }

  Widget _statsList(AdminStats s) {
    final delivered = s.ordersByStatus[statusDelivered] ?? 0;
    final cancelled = s.ordersByStatus[statusCancelled] ?? 0;
    final open = s.ordersTotal - delivered - cancelled;
    final avgPayout = delivered > 0 ? s.riderPayoutsMinor ~/ delivered : 0;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        const CMSectionHeader('Revenue'),
        Row(children: [
          Expanded(
            child: CMStatTile(
              'Platform ujrah · month',
              money(s.platformUjrahMinor),
              hint: 'The market commission on this month\'s shop sales, '
                  'recomputed by the fixed schedule',
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: CMStatTile(
              'GMV · month',
              money(s.gmvMonthMinor),
              hint: 'Order value placed this month, cancelled excluded',
            ),
          ),
        ]),
        const SizedBox(height: 12),
        Row(children: [
          Expanded(
            child: CMStatTile(
              'GMV · all time',
              money(s.gmvMinor),
              hint: 'All non-cancelled orders since launch',
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: CMStatTile(
              'Orders · all time',
              '${s.ordersTotal}',
              hint: 'Placed, delivered and cancelled together',
            ),
          ),
        ]),
        const CMSectionHeader('Rider payouts'),
        Row(children: [
          Expanded(
            child: CMStatTile(
              'Paid out · all time',
              money(s.riderPayoutsMinor),
              hint: 'Delivered orders only — payouts are earned at delivery',
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: CMStatTile(
              'Avg per delivery',
              money(avgPayout),
              hint: 'Paid out divided by delivered orders',
            ),
          ),
        ]),
        const CMSectionHeader('Workers'),
        Row(children: [
          Expanded(
            child: CMStatTile(
              'Shops working',
              '${s.shopsSelling} of ${s.shopsTotal}',
              hint: 'Shops with at least one delivered order, all time',
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: CMStatTile(
              'Shops open',
              '${s.shopsActive} of ${s.shopsTotal}',
            ),
          ),
        ]),
        const SizedBox(height: 12),
        Row(children: [
          Expanded(
            child: CMStatTile(
              'Riders on duty',
              '${s.ridersAvailable} of ${s.ridersTotal}',
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: CMStatTile(
              'Riders paid',
              '${s.ridersPaid} of ${s.ridersTotal}',
              hint: 'Riders with at least one delivered order, all time',
            ),
          ),
        ]),
        const CMSectionHeader('Orders'),
        Row(children: [
          Expanded(child: CMStatTile('Delivered', '$delivered')),
          const SizedBox(width: 12),
          Expanded(child: CMStatTile('Open', '$open', hint: 'Placed, confirmed or picked up')),
          const SizedBox(width: 12),
          Expanded(child: CMStatTile('Cancelled', '$cancelled')),
        ]),
        const CMSectionHeader('Shop settlements · this month'),
        _tableHeader(const ['Shop', 'Sales', 'Ujrah', 'Net']),
        for (final sh in s.shops)
          _tableRow(
            [sh.name, money(sh.salesMinor), money(sh.ujrahMinor), money(sh.netMinor)],
            semantic:
                '${sh.name}: sales ${money(sh.salesMinor)}, ujrah ${money(sh.ujrahMinor)}, net ${money(sh.netMinor)}',
          ),
        if (s.shops.isEmpty)
          const Text('No settlements yet — no delivered orders this month.',
              style: TextStyle(color: CMColors.inkMuted)),
        const CMSectionHeader('Rider earnings · all time'),
        _tableHeader(const ['Rider', 'Deliveries', 'Payout']),
        for (final rd in s.riders)
          _tableRow(
            [rd.name, '${rd.deliveries}', money(rd.payoutMinor)],
            semantic:
                '${rd.name}: ${rd.deliveries} deliveries, paid ${money(rd.payoutMinor)}',
          ),
        if (s.riders.isEmpty)
          const Text('No payouts yet.', style: TextStyle(color: CMColors.inkMuted)),
        const SizedBox(height: 24),
        const Text(
          'Halal note: shop commission is the fixed Ujrah schedule computed '
          'by the server monthly over delivered sales; riders are paid per '
          'delivery plus per kilometre. Nothing here is stored — every '
          'number is recomputed from the order ledger.',
          style: TextStyle(color: CMColors.inkMuted, fontSize: 12),
        ),
      ],
    );
  }

  Widget _tableHeader(List<String> cells) => Container(
        decoration: const BoxDecoration(
          border: Border(bottom: BorderSide(color: CMColors.hairline)),
        ),
        padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 4),
        child: Row(
          children: [
            for (final c in cells.take(cells.length - 1))
              Expanded(
                child: Text(c.toUpperCase(),
                    style: const TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 0.8,
                        color: CMColors.inkMuted)),
              ),
            Expanded(
              child: Text(
                cells.last.toUpperCase(),
                textAlign: TextAlign.right,
                style: const TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 0.8,
                    color: CMColors.inkMuted),
              ),
            ),
          ],
        ),
      );

  Widget _tableRow(List<String> cells, {required String semantic}) => Semantics(
        label: semantic,
        excludeSemantics: true,
        child: Container(
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: CMColors.hairline)),
          ),
          padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 4),
          child: Row(
            children: [
              for (final c in cells.take(cells.length - 1))
                Expanded(child: Text(c, style: const TextStyle(fontSize: 14))),
              Expanded(
                child: Text(
                  cells.last,
                  textAlign: TextAlign.right,
                  style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
                ),
              ),
            ],
          ),
        ),
      );
}
