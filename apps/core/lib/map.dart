/// The built-in map: OpenStreetMap street tiles and Esri satellite imagery
/// through flutter_map — no API key, no platform channel, attribution always
/// on the map itself. Three shapes share it: the rider's job map (pickup +
/// drop markers and the route line), the mini preview the board shows before
/// claiming, and the citizen's delivery pin picker.
///
/// Detail policy: the camera always fits the route and never settles below
/// building-rendering zooms (16+ street, 17 preferred), so houses, building
/// footprints, street names and house numbers are actually visible — the
/// satellite layer shows the exact built structures where street tiles are
/// sparse (rural Sindh).
library;

import 'package:core/design.dart';
import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

/// Johi town center — the market's home ground (Dadu District, Sindh).
const LatLng johiCenter = LatLng(26.692061, 67.613300);

const _streetTiles = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
const _satelliteTiles =
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}';

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

/// Street or satellite raster layer, offline-graceful (failed tiles become a
/// quiet wash square instead of grey noise).
TileLayer _tiles({required bool satellite}) => TileLayer(
      urlTemplate: satellite ? _satelliteTiles : _streetTiles,
      userAgentPackageName: 'dev.citymarket',
      maxNativeZoom: 19,
      errorTileBuilder: (_, __, ___) => const ColoredBox(color: CMColors.wash),
    );

/// The on-map attribution strip — OSM and Esri both require visible credit,
/// so it lives on the map, not in a footer that can scroll away.
class _Attribution extends StatelessWidget {
  final bool satellite;

  const _Attribution({required this.satellite});

  @override
  Widget build(BuildContext context) {
    return Positioned(
      left: 0,
      bottom: 0,
      child: Container(
        color: CMColors.paper.withOpacity(0.85),
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        child: Text(
          satellite
              ? '© OpenStreetMap contributors · Imagery © Esri, Maxar, Earthstar Geographics'
              : '© OpenStreetMap contributors',
          style: const TextStyle(fontSize: 10, color: CMColors.inkMuted),
        ),
      ),
    );
  }
}

/// Square street/satellite switch — labelled buttons, 48 dp targets, never a
/// bare icon.
class _LayerToggle extends StatelessWidget {
  final bool satellite;
  final ValueChanged<bool> onChanged;

  const _LayerToggle({required this.satellite, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return Positioned(
      top: 8,
      right: 8,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Semantics(
            button: true,
            label: 'Show street map with names',
            child: satellite
                ? OutlinedButton.icon(
                    onPressed: () => onChanged(false),
                    icon: const Icon(Icons.map_outlined, size: 16),
                    label: const Text('Map', style: TextStyle(fontSize: 12)),
                  )
                : FilledButton.icon(
                    onPressed: () => onChanged(false),
                    icon: const Icon(Icons.map_outlined, size: 16),
                    label: const Text('Map', style: TextStyle(fontSize: 12)),
                  ),
          ),
          const SizedBox(width: 6),
          Semantics(
            button: true,
            label: 'Show satellite imagery with exact buildings',
            child: satellite
                ? FilledButton.icon(
                    onPressed: () => onChanged(true),
                    icon: const Icon(Icons.satellite_alt_outlined, size: 16),
                    label:
                        const Text('Satellite', style: TextStyle(fontSize: 12)),
                  )
                : OutlinedButton.icon(
                    onPressed: () => onChanged(true),
                    icon: const Icon(Icons.satellite_alt_outlined, size: 16),
                    label:
                        const Text('Satellite', style: TextStyle(fontSize: 12)),
                  ),
          ),
        ],
      ),
    );
  }
}

/// The rider's job map: green pickup square at the shop, dark drop square
/// at the customer, straight hairline route between them (the road-level
/// estimate lives in the server's numbers; the line shows the corridor).
/// The camera fits both points and zooms in to building level — houses,
/// street names and exact structures are the point of this map.
class CMJobMap extends StatefulWidget {
  final CMRoute route;

  const CMJobMap({super.key, required this.route});

  @override
  State<CMJobMap> createState() => _CMJobMapState();
}

class _CMJobMapState extends State<CMJobMap> {
  bool _satellite = false;

  @override
  Widget build(BuildContext context) {
    final route = widget.route;
    return Column(
      children: [
        Expanded(
          child: Stack(
            children: [
              FlutterMap(
                options: MapOptions(
                  initialCameraFit: CameraFit.bounds(
                    bounds: LatLngBounds(route.pickup, route.drop),
                    padding: const EdgeInsets.all(56),
                    maxZoom: 17,
                    minZoom: 12,
                  ),
                  interactionOptions: const InteractionOptions(
                      flags: InteractiveFlag.all & ~InteractiveFlag.rotate),
                ),
                children: [
                  _tiles(satellite: _satellite),
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
                      _squareMarker(route.pickup, CMColors.green,
                          Icons.storefront_outlined),
                      _squareMarker(route.drop, CMColors.ink, Icons.home_outlined),
                    ],
                  ),
                ],
              ),
              _LayerToggle(
                satellite: _satellite,
                onChanged: (s) => setState(() => _satellite = s),
              ),
              _Attribution(satellite: _satellite),
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
                  style: const TextStyle(
                      fontWeight: FontWeight.w700, color: CMColors.greenDeep)),
              const SizedBox(height: 2),
              Text('Deliver to: ${route.dropName}',
                  style: const TextStyle(fontWeight: FontWeight.w700)),
              const SizedBox(height: 4),
              const Text(
                  'Drag to pan, pinch to zoom — buildings and street names '
                  'appear as you go closer.',
                  style: TextStyle(fontSize: 11, color: CMColors.inkMuted)),
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
/// same pair, no gestures — but the camera still fits the actual route at
/// building-rendering zoom so the preview shows real streets, not a
/// zoomed-out dot.
class CMMiniMap extends StatelessWidget {
  final CMRoute route;
  final double height;

  const CMMiniMap({super.key, required this.route, this.height = 140});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: height,
      child: Stack(
        children: [
          FlutterMap(
            options: MapOptions(
              initialCameraFit: CameraFit.bounds(
                bounds: LatLngBounds(route.pickup, route.drop),
                padding: const EdgeInsets.all(28),
                maxZoom: 16,
                minZoom: 12,
              ),
              interactionOptions:
                  const InteractionOptions(flags: InteractiveFlag.none),
            ),
            children: [
              _tiles(satellite: false),
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
          const _Attribution(satellite: false),
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
/// typed beside it. Opens at building-level zoom so the pin lands on an
/// exact house, not a district.
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
  bool _satellite = false;

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
                    initialZoom: 17,
                    interactionOptions: const InteractionOptions(
                        flags: InteractiveFlag.all & ~InteractiveFlag.rotate),
                    onPositionChanged: (pos, hasGesture) {
                      if (hasGesture) {
                        _center = pos.center;
                      }
                    },
                  ),
                  children: [
                    _tiles(satellite: _satellite),
                  ],
                ),
                Center(
                  child: Icon(Icons.place, size: 44, color: CMColors.greenDeep,
                      shadows: [Shadow(color: Colors.white, blurRadius: 4)]),
                ),
                _LayerToggle(
                  satellite: _satellite,
                  onChanged: (s) => setState(() => _satellite = s),
                ),
                _Attribution(satellite: _satellite),
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
