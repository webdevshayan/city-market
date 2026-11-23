import 'package:core/api.dart';
import 'package:core/design.dart';
import 'package:core/map.dart' as cmmap;
import 'package:core/models.dart';
import 'package:flutter/material.dart';
import 'package:latlong2/latlong.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() => runApp(const CityMarketRiderApp());

/// The rider app: go on duty, scan the dispatch board (every job shows
/// where to pick up, where to deliver, how far, how long and what it pays
/// before any commitment), claim a job, then navigate by the built-in map
/// and walk it through pick-up → deliver.
class CityMarketRiderApp extends StatelessWidget {
  const CityMarketRiderApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'City Market — Rider',
      theme: buildCMTheme(),
      home: SessionGate(api: HttpApiClient(
        const String.fromEnvironment('API_BASE_URL', defaultValue: 'https://cmserver.vercel.app'),
      )),
    );
  }
}

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
        _token = prefs.getString('rider_token');
        _ready = true;
      });
    }
  }

  Future<void> _onSignedIn(String token) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('rider_token', token);
    setState(() => _token = token);
  }

  Future<void> _onSignOut() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove('rider_token');
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
    return RiderHome(api: widget.api, token: _token!, onSignOut: _onSignOut);
  }
}

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
        r = await widget.api.registerRider(
          name: _name.text.trim(),
          phone: _phone.text.trim(),
          password: _password.text,
          lat: cmmap.johiCenter.latitude,
          lng: cmmap.johiCenter.longitude,
        );
      } else {
        r = await widget.api.login(role: 'rider', phone: _phone.text.trim(), password: _password.text);
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
      appBar: AppBar(title: const Text('City Market — Rider')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          CMSectionHeader(_registering ? 'Start delivering' : 'Sign in'),
          if (_registering)
            TextField(
              key: const ValueKey('rider-name-field'),
              controller: _name,
              decoration: const InputDecoration(labelText: 'Your name'),
            ),
          if (_registering) const SizedBox(height: 12),
          TextField(
            key: const ValueKey('rider-phone-field'),
            controller: _phone,
            keyboardType: TextInputType.phone,
            decoration: const InputDecoration(labelText: 'Mobile number', hintText: '03001234567'),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const ValueKey('rider-password-field'),
            controller: _password,
            obscureText: true,
            decoration: const InputDecoration(labelText: 'Password (8+ characters)'),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: const TextStyle(color: CMColors.danger, fontWeight: FontWeight.w600)),
          ],
          const SizedBox(height: 16),
          FilledButton(
            key: const ValueKey('rider-submit-btn'),
            onPressed: _busy ? null : _submit,
            child: Text(_busy ? 'Working…' : _registering ? 'Register as rider' : 'Sign in'),
          ),
          const SizedBox(height: 8),
          TextButton(
            key: const ValueKey('rider-toggle-mode'),
            onPressed: _busy
                ? null
                : () {
                    setState(() {
                      _registering = !_registering;
                      _error = null;
                    });
                  },
            child: Text(_registering ? 'Already riding? Sign in' : 'New here? Register as a rider'),
          ),
          if (_registering) ...[
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(border: Border.all(color: CMColors.hairline), color: CMColors.wash),
              child: const Text(
                'Riders earn 60 PKR per delivery plus 15 PKR per route km, paid on '
                'settlement with the admin. You start based in Johi; set your spot '
                'on the map after signing in.',
                style: TextStyle(color: CMColors.inkMuted, fontSize: 13),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class RiderHome extends StatefulWidget {
  final ApiClient api;
  final String token;
  final VoidCallback onSignOut;

  const RiderHome({super.key, required this.api, required this.token, required this.onSignOut});

  @override
  State<RiderHome> createState() => _RiderHomeState();
}

class _RiderHomeState extends State<RiderHome> {
  RiderAccount? _me;
  bool _meError = false;
  // Shop coordinates captured at claim time, keyed by order — the claimed
  // order view does not carry them, so the map uses what the board showed.
  final _claimedShopPos = <String, LatLng>{};
  late Future<List<BoardJob>> _board = widget.api.riderBoard(widget.token);
  late Future<List<Order>> _jobs = widget.api.riderJobs(widget.token);

  @override
  void initState() {
    super.initState();
    _loadMe();
  }

  Future<void> _loadMe() async {
    try {
      final me = await widget.api.riderMe(widget.token);
      if (mounted) {
        setState(() => _me = me);
      }
    } on ApiException {
      if (mounted) {
        setState(() => _meError = true);
      }
    }
  }

  Future<void> _reload() async {
    // Block body on purpose: an arrow closure here would return the Future.
    setState(() {
      _board = widget.api.riderBoard(widget.token);
      _jobs = widget.api.riderJobs(widget.token);
    });
  }

  Future<void> _toggleDuty() async {
    final me = _me;
    if (me == null) {
      return;
    }
    try {
      await widget.api.riderSetMe(widget.token,
          lat: me.lat, lng: me.lng, available: !me.available);
      await _loadMe();
      await _reload();
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
  }

  Future<void> _claim(BoardJob job) async {
    _claimedShopPos[job.orderId] = LatLng(job.shopLat, job.shopLng);
    try {
      await widget.api.claimOrder(widget.token, job.orderId);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
            content: Text('Job claimed — it is on your list below.')));
      }
      await _reload();
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
  }

  Future<void> _advance(Order o) async {
    try {
      if (o.status == statusConfirmed) {
        await widget.api.pickUpOrder(widget.token, o.id);
      } else if (o.status == statusPickedUp) {
        await widget.api.deliverOrder(widget.token, o.id);
      }
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
    await _reload();
  }

  Future<void> _openJobDetail(BoardJob job) async {
    await Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => JobDetailScreen(job: job, onClaim: () => _claim(job)),
    ));
    await _reload();
  }

  Future<void> _openJobMap(Order o) async {
    await Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => ActiveJobMapScreen(order: o, shopPos: _claimedShopPos[o.id]),
    ));
    await _reload();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Rider board'),
        actions: [
          IconButton(tooltip: 'Sign out', onPressed: widget.onSignOut, icon: const Icon(Icons.logout)),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => _reload(),
        child: ListView(
          children: [
            _dutyBanner(),
            const CMSectionHeader('Open jobs — estimates before you commit'),
            FutureBuilder<List<BoardJob>>(
              future: _board,
              builder: (context, snap) {
                if (snap.hasError) {
                  return CMErrorRetry(error: snap.error!, onRetry: _reload);
                }
                if (!snap.hasData) {
                  return const Padding(
                      padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()));
                }
                final jobs = snap.data!;
                if (jobs.isEmpty) {
                  return const Padding(
                    padding: EdgeInsets.all(16),
                    child: Text(
                        'No open jobs right now. Jobs appear here the moment a shop accepts an order — '
                        'each shows the pickup, the drop, the distance, the ETA and the payout before you claim.'),
                  );
                }
                return Column(
                  children: [
                    for (final job in jobs)
                      JobCard(job: job, onOpen: () => _openJobDetail(job)),
                  ],
                );
              },
            ),
            const CMSectionHeader('My claimed jobs'),
            FutureBuilder<List<Order>>(
              future: _jobs,
              builder: (context, snap) {
                if (snap.hasError) {
                  return CMErrorRetry(error: snap.error!, onRetry: _reload);
                }
                if (!snap.hasData) {
                  return const Padding(
                      padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()));
                }
                final jobs = snap.data!;
                if (jobs.isEmpty) {
                  return const Padding(
                    padding: EdgeInsets.all(16),
                    child: Text('Nothing claimed yet — claim an open job above and it lands here.'),
                  );
                }
                return Column(
                  children: [
                    for (final o in jobs)
                      ListTile(
                        leading: CMStatusChip(o.status),
                        title: Text('${o.shopName} → ${o.addressNote.isEmpty ? 'customer pin' : o.addressNote}'),
                        subtitle: Text(
                            '${money(o.riderPayoutMinor)} PKR payout · ${o.estRouteMeters / 1000} km route'),
                        trailing: Row(mainAxisSize: MainAxisSize.min, children: [
                          IconButton(
                            tooltip: 'Map',
                            onPressed: () => _openJobMap(o),
                            icon: const Icon(Icons.map_outlined),
                          ),
                          if (o.status == statusConfirmed || o.status == statusPickedUp)
                            FilledButton(
                              key: ValueKey('${o.status == statusConfirmed ? 'pick-up' : 'deliver'}-${o.id}'),
                              onPressed: () => _advance(o),
                              child: Text(o.status == statusConfirmed ? 'Picked up' : 'Delivered'),
                            ),
                        ]),
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

  Widget _dutyBanner() {
    if (_meError) {
      return CMErrorRetry(error: 'could not load your rider profile', onRetry: _loadMe);
    }
    final me = _me;
    if (me == null) {
      return const Padding(padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()));
    }
    return Container(
      margin: const EdgeInsets.all(16),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        border: Border.all(color: me.available ? CMColors.green : CMColors.hairline),
        color: me.available ? CMColors.paper : CMColors.wash,
      ),
      child: Row(
        children: [
          Icon(
            me.available ? Icons.verified_outlined : Icons.bedtime_outlined,
            color: me.available ? CMColors.green : CMColors.inkMuted,
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  me.available ? 'On duty — claims are open' : 'Off duty — go on duty to claim',
                  style: TextStyle(
                      fontWeight: FontWeight.w700,
                      color: me.available ? CMColors.greenDeep : CMColors.inkMuted),
                ),
                Text(
                  'Signed in as ${me.name} · based at ${me.lat.toStringAsFixed(4)}, ${me.lng.toStringAsFixed(4)}',
                  style: const TextStyle(fontSize: 12, color: CMColors.inkMuted),
                ),
              ],
            ),
          ),
          OutlinedButton(
            key: const ValueKey('duty-toggle'),
            onPressed: _toggleDuty,
            child: Text(me.available ? 'Go off duty' : 'Go on duty'),
          ),
        ],
      ),
    );
  }
}

/// One open job on the board: the full decision set — pickup, drop,
/// distance, ETA, payout — with a mini map preview before any commitment.
class JobCard extends StatelessWidget {
  final BoardJob job;
  final VoidCallback onOpen;

  const JobCard({super.key, required this.job, required this.onOpen});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      key: ValueKey('job-${job.orderId}'),
      onTap: onOpen,
      child: Container(
        margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
        decoration: BoxDecoration(border: Border.all(color: CMColors.hairline)),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            cmmap.CMMiniMap(
              route: cmmap.CMRoute(
                pickup: LatLng(job.shopLat, job.shopLng),
                drop: LatLng(job.dropLat, job.dropLng),
                pickupName: job.shopName,
                dropName: job.addressNote,
              ),
              height: 110,
            ),
            Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text('Pick up: ${job.shopName}',
                            style: const TextStyle(fontWeight: FontWeight.w700, color: CMColors.greenDeep)),
                      ),
                      Text('${job.payoutText} PKR',
                          style: const TextStyle(fontWeight: FontWeight.w700, color: CMColors.greenDeep)),
                    ],
                  ),
                  const SizedBox(height: 4),
                  Text('Deliver: ${job.addressNote.isEmpty ? "customer's pin" : job.addressNote}',
                      style: const TextStyle(color: CMColors.inkMuted)),
                  const SizedBox(height: 8),
                  Row(
                    children: [
                      _stat(Icons.shopping_bag_outlined, '${job.lines} line(s)'),
                      const SizedBox(width: 12),
                      _stat(Icons.route_outlined, '${job.routeKm.toStringAsFixed(1)} km route'),
                      const SizedBox(width: 12),
                      _stat(Icons.schedule_outlined, '~${job.etaMinutes} min'),
                      const SizedBox(width: 12),
                      _stat(Icons.near_me_outlined, '${job.pickupKm.toStringAsFixed(1)} km to pickup'),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _stat(IconData icon, String text) {
    return Row(mainAxisSize: MainAxisSize.min, children: [
      Icon(icon, size: 14, color: CMColors.inkMuted),
      const SizedBox(width: 4),
      Text(text, style: const TextStyle(fontSize: 12, color: CMColors.inkMuted)),
    ]);
  }
}

/// The full decision screen: the whole map, the pickup and drop named, and
/// the claim button — the rider commits only from here.
class JobDetailScreen extends StatelessWidget {
  final BoardJob job;
  final VoidCallback onClaim;

  const JobDetailScreen({super.key, required this.job, required this.onClaim});

  @override
  Widget build(BuildContext context) {
    final route = cmmap.CMRoute(
      pickup: LatLng(job.shopLat, job.shopLng),
      drop: LatLng(job.dropLat, job.dropLng),
      pickupName: job.shopName,
      dropName: job.addressNote,
    );
    return Scaffold(
      appBar: AppBar(title: const Text('Before you claim')),
      body: Column(
        children: [
          Expanded(child: cmmap.CMJobMap(route: route)),
          Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    _fact('Route', '${job.routeKm.toStringAsFixed(1)} km'),
                    _fact('To pickup', '${job.pickupKm.toStringAsFixed(1)} km'),
                    _fact('ETA', '~${job.etaMinutes} min'),
                    _fact('You earn', '${job.payoutText} PKR'),
                  ],
                ),
                const SizedBox(height: 14),
                FilledButton.icon(
                  key: ValueKey('claim-${job.orderId}'),
                  onPressed: () {
                    Navigator.of(context).pop();
                    onClaim();
                  },
                  icon: const Icon(Icons.check),
                  label: const Text('Claim this job'),
                ),
                const SizedBox(height: 6),
                const Text(
                  'Claiming is first-come: a slower rider gets told the job is taken. '
                  'The payout locks to the number above. Map data © OpenStreetMap contributors.',
                  style: TextStyle(fontSize: 11, color: CMColors.inkMuted),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _fact(String label, String value) {
    return Column(
      children: [
        Text(label.toUpperCase(),
            style: const TextStyle(fontSize: 10, letterSpacing: 0.8, color: CMColors.inkMuted)),
        Text(value, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
      ],
    );
  }
}

/// The claimed job's navigation map: pickup square at the shop, drop square
/// at the customer, route line between — plus the address strip.
class ActiveJobMapScreen extends StatelessWidget {
  final Order order;
  final LatLng? shopPos;

  const ActiveJobMapScreen({super.key, required this.order, this.shopPos});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('Job ${order.id.substring(0, 10)}…')),
      body: cmmap.CMJobMap(
        route: cmmap.CMRoute(
          pickup: shopPos ?? cmmap.johiCenter,
          drop: LatLng(order.lat, order.lng),
          pickupName: order.shopName,
          dropName: order.addressNote.isEmpty
              ? 'Customer pin (${order.lat.toStringAsFixed(4)}, ${order.lng.toStringAsFixed(4)})'
              : order.addressNote,
        ),
      ),
    );
  }
}
