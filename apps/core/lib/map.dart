/// The built-in map: OpenStreetMap tiles through flutter_map — no API key,
/// no platform channel, attribution always on. Three shapes share it: the
/// rider's job map (pickup + drop markers and the route line), the mini
/// preview the board shows before claiming, and the citizen's delivery
/// pin picker.
library;

import 'package:core/design.dart';
import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

/// Johi town center — the market's home ground (Dadu District, Sindh).
const LatLng johiCenter = LatLng(26.692061, 67.613300);

const _tileUrl = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';

/// One pickup/drop pair drawn on the map.
class CMRoute {
  final LatLng pickup;
  final LatLng drop;
  final String pickupName;
  final String dropName;

  const CMRoute({
    required this.pickup,
    required this.drop,
    required this.pickupName,
    required this.dropName,
  });
}

/// The rider's job map: green pickup square at the shop, dark drop square
/// at the customer, straight hairline route between them (the road-level
/// estimate lives in the server's numbers; the line shows the corridor).
class CMJobMap extends StatelessWidget {
  final CMRoute route;

  const CMJobMap({super.key, required this.route});

  @override
  Widget build(BuildContext context) {
    final bounds = LatLngBounds(route.pickup, route.drop);
    return Column(
      children: [
        Expanded(
          child: FlutterMap(
            options: MapOptions(
              initialCenter: LatLngBoundsTools.center(bounds),
              initialZoom: 14,
              interactionOptions: const InteractionOptions(
                  flags: InteractiveFlag.all & ~InteractiveFlag.rotate),
            ),
            children: [
              TileLayer(
                urlTemplate: _tileUrl,
                userAgentPackageName: 'dev.citymarket',
              ),
              PolylineLayer(
                polylines: [
                  Polyline(
                    points: [route.pickup, route.drop],
                    color: CMColors.green,
                    strokeWidth: 3,
                  ),
                ],
              ),
              MarkerLayer(
                markers: [
                  _squareMarker(route.pickup, CMColors.green, Icons.storefront_outlined),
                  _squareMarker(route.drop, CMColors.ink, Icons.home_outlined),
                ],
              ),
            ],
          ),
        ),
        Container(
          decoration: const BoxDecoration(
            border: Border(top: BorderSide(color: CMColors.hairline)),
            color: CMColors.paper,
          ),
          padding: const EdgeInsets.all(10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Pick up: ${route.pickupName}',
                  style: const TextStyle(fontWeight: FontWeight.w700, color: CMColors.greenDeep)),
              const SizedBox(height: 2),
              Text('Deliver to: ${route.dropName}',
                  style: const TextStyle(fontWeight: FontWeight.w700)),
              const SizedBox(height: 4),
              const Text('Map data © OpenStreetMap contributors',
                  style: TextStyle(fontSize: 10, color: CMColors.inkMuted)),
            ],
          ),
        ),
      ],
    );
  }

  Marker _squareMarker(LatLng point, Color color, IconData icon) {
    return Marker(
      point: point,
      width: 34,
      height: 34,
      child: Container(
        decoration: BoxDecoration(
          color: color,
          border: Border.all(color: CMColors.paper, width: 2),
        ),
        child: Icon(icon, size: 18, color: CMColors.onGreen),
      ),
    );
  }
}

/// A small non-interactive preview (the board card's reassurance map):
/// same pair, no gestures, no zoom controls.
class CMMiniMap extends StatelessWidget {
  final CMRoute route;
  final double height;

  const CMMiniMap({super.key, required this.route, this.height = 140});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: height,
      child: FlutterMap(
        options: const MapOptions(
          initialCenter: johiCenter,
          initialZoom: 13,
          interactionOptions: InteractionOptions(flags: InteractiveFlag.none),
        ),
        children: [
          TileLayer(
            urlTemplate: _tileUrl,
            userAgentPackageName: 'dev.citymarket',
          ),
          PolylineLayer(
            polylines: [
              Polyline(
                points: [route.pickup, route.drop],
                color: CMColors.green,
                strokeWidth: 3,
              ),
            ],
          ),
          MarkerLayer(
            markers: [
              _dot(route.pickup, CMColors.green),
              _dot(route.drop, CMColors.ink),
            ],
          ),
        ],
      ),
    );
  }

  Marker _dot(LatLng point, Color color) {
    return Marker(
      point: point,
      width: 16,
      height: 16,
      child: Container(color: color),
    );
  }
}

/// The citizen's delivery-location picker: drag the map, the pin stays
/// centered, "use this spot" returns the coordinates with the address note
/// typed beside it.
class CMLocationPicker extends StatefulWidget {
  final LatLng initial;
  final ValueChanged<LatLng> onPicked;

  const CMLocationPicker({super.key, this.initial = johiCenter, required this.onPicked});

  @override
  State<CMLocationPicker> createState() => _CMLocationPickerState();
}

class _CMLocationPickerState extends State<CMLocationPicker> {
  late LatLng _center = widget.initial;
  late final MapController _controller = MapController();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Where should we deliver?')),
      body: Column(
        children: [
          Expanded(
            child: Stack(
              children: [
                FlutterMap(
                  mapController: _controller,
                  options: MapOptions(
                    initialCenter: widget.initial,
                    initialZoom: 14,
                    onPositionChanged: (pos, hasGesture) {
                      if (hasGesture) {
                        _center = pos.center;
                      }
                    },
                  ),
                  children: [
                    TileLayer(
                      urlTemplate: _tileUrl,
                      userAgentPackageName: 'dev.citymarket',
                    ),
                  ],
                ),
                Center(
                  child: Icon(Icons.place, size: 44, color: CMColors.greenDeep,
                      shadows: [Shadow(color: Colors.white, blurRadius: 4)]),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  'Pin position: ${_center.latitude.toStringAsFixed(5)}, ${_center.longitude.toStringAsFixed(5)}',
                  style: const TextStyle(color: CMColors.inkMuted),
                ),
                const SizedBox(height: 10),
                FilledButton.icon(
                  key: const ValueKey('confirm-location'),
                  onPressed: () => widget.onPicked(_center),
                  icon: const Icon(Icons.check),
                  label: const Text('Deliver here'),
                ),
                const SizedBox(height: 4),
                const Text('Map data © OpenStreetMap contributors',
                    style: TextStyle(fontSize: 10, color: CMColors.inkMuted)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Small helpers around LatLngBounds.
extension LatLngBoundsTools on LatLngBounds {
  static LatLng center(LatLngBounds b) => LatLng((b.south + b.north) / 2, (b.west + b.east) / 2);
}
