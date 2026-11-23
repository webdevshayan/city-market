/// Square chart primitives for the admin console, painted by hand — no
/// chart library, no rounded corners, green-on-white only. Every chart
/// carries its numbers as a semantics label so screen readers announce the
/// real values instead of "graphic".
library;

import 'dart:math' as math;

import 'package:core/design.dart';
import 'package:core/models.dart';
import 'package:flutter/material.dart';

/// Vertical bar chart over the daily series (orders per day, payouts per
/// day). Bars are flat green fills on hairline gridlines; the maximum is
/// labelled.
class CMBarChart extends StatelessWidget {
  final List<String> labels;
  final List<int> values;
  final String semanticsLabel;

  const CMBarChart({
    super.key,
    required this.labels,
    required this.values,
    required this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    assert(labels.length == values.length);
    final maxV = values.fold<int>(1, (a, b) => a > b ? a : b);
    return Semantics(
      label: semanticsLabel,
      excludeSemantics: true,
      child: Container(
        decoration: BoxDecoration(
          border: Border.all(color: CMColors.hairline),
          color: CMColors.paper,
        ),
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            SizedBox(
              height: 120,
              child: CustomPaint(
                painter: _BarPainter(values: values, max: maxV),
              ),
            ),
            const SizedBox(height: 6),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                for (var i = 0; i < labels.length; i++)
                  if (labels.length <= 8 || i % (labels.length ~/ 4 + 1) == 0)
                    Text(
                      labels[i].substring(5), // MM-DD
                      style: const TextStyle(fontSize: 9, color: CMColors.inkMuted),
                    ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// A flat line/area chart for money series (GMV per day), same square
/// language: hairline frame, green stroke, no rounded anything.
class CMLineChart extends StatelessWidget {
  final List<String> labels;
  final List<int> valuesMinor;
  final String semanticsLabel;

  const CMLineChart({
    super.key,
    required this.labels,
    required this.valuesMinor,
    required this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: semanticsLabel,
      excludeSemantics: true,
      child: Container(
        decoration: BoxDecoration(
          border: Border.all(color: CMColors.hairline),
          color: CMColors.paper,
        ),
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            SizedBox(
              height: 120,
              child: CustomPaint(
                painter: _LinePainter(values: valuesMinor),
              ),
            ),
            const SizedBox(height: 6),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                if (labels.isNotEmpty) ...[
                  Text(labels.first.substring(5),
                      style: const TextStyle(fontSize: 9, color: CMColors.inkMuted)),
                  Text(labels.last.substring(5),
                      style: const TextStyle(fontSize: 9, color: CMColors.inkMuted)),
                ],
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _BarPainter extends CustomPainter {
  final List<int> values;
  final int max;

  _BarPainter({required this.values, required this.max});

  @override
  void paint(Canvas canvas, Size size) {
    if (values.isEmpty) {
      return;
    }
    final grid = Paint()
      ..color = CMColors.hairline
      ..strokeWidth = 1;
    for (var i = 1; i <= 2; i++) {
      final y = size.height * i / 3;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), grid);
    }
    final slot = size.width / values.length;
    final barWidth = math.min(slot * 0.6, 18.0);
    final fill = Paint()..color = CMColors.green;
    for (var i = 0; i < values.length; i++) {
      final h = max <= 0 ? 0.0 : size.height * values[i] / max;
      final left = slot * i + (slot - barWidth) / 2;
      canvas.drawRect(
        Rect.fromLTWH(left, size.height - h, barWidth, h),
        fill,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _BarPainter old) =>
      old.values != values || old.max != max;
}

class _LinePainter extends CustomPainter {
  final List<int> values;

  _LinePainter({required this.values});

  @override
  void paint(Canvas canvas, Size size) {
    if (values.isEmpty) {
      return;
    }
    final maxV = values.fold<int>(1, (a, b) => a > b ? a : b);
    final grid = Paint()
      ..color = CMColors.hairline
      ..strokeWidth = 1;
    for (var i = 1; i <= 2; i++) {
      final y = size.height * i / 3;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), grid);
    }
    final step = values.length <= 1 ? size.width : size.width / (values.length - 1);
    final points = <Offset>[
      for (var i = 0; i < values.length; i++)
        Offset(step * i, size.height - (maxV <= 0 ? 0.0 : size.height * values[i] / maxV)),
    ];
    final line = Paint()
      ..color = CMColors.green
      ..strokeWidth = 2
      ..style = PaintingStyle.stroke;
    canvas.drawPath(Path()..addPolygon(points, false), line);
    final dot = Paint()..color = CMColors.greenDeep;
    for (final p in points) {
      canvas.drawRect(Rect.fromCenter(center: p, width: 4, height: 4), dot);
    }
  }

  @override
  bool shouldRepaint(covariant _LinePainter old) => old.values != values;
}

/// The 14-day charts section: orders per day as bars, GMV per day as a
/// line, payouts per day as bars — built straight from the stats snapshot.
class CMDashboardCharts extends StatelessWidget {
  final DailySeries series;

  const CMDashboardCharts({super.key, required this.series});

  @override
  Widget build(BuildContext context) {
    if (series.days.isEmpty) {
      return const SizedBox.shrink();
    }
    final ordersSum = series.orders.fold<int>(0, (a, b) => a + b);
    final gmvSum = series.gmvMinor.fold<int>(0, (a, b) => a + b);
    final payoutSum = series.payoutMinor.fold<int>(0, (a, b) => a + b);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        CMSectionHeader('Last 14 days — orders'),
        CMBarChart(
          labels: series.days,
          values: series.orders,
          semanticsLabel:
              'Orders per day chart, last 14 days, total $ordersSum orders. '
              'Days oldest to newest: ${_spoken(series.days, series.orders)}',
        ),
        CMSectionHeader('Last 14 days — GMV (PKR)'),
        CMLineChart(
          labels: series.days,
          valuesMinor: series.gmvMinor,
          semanticsLabel:
              'Gross market value per day chart, last 14 days, total ${money(gmvSum)} rupees.',
        ),
        CMSectionHeader('Last 14 days — rider payouts (PKR)'),
        CMBarChart(
          labels: series.days,
          values: series.payoutMinor,
          semanticsLabel:
              'Rider payouts per day chart, last 14 days, total ${money(payoutSum)} rupees.',
        ),
      ],
    );
  }

  String _spoken(List<String> days, List<int> values) {
    final parts = <String>[];
    for (var i = 0; i < days.length && i < values.length; i++) {
      if (values[i] > 0) {
        parts.add('${days[i].substring(5)}: ${values[i]}');
      }
    }
    return parts.isEmpty ? 'no orders yet' : parts.join(', ');
  }
}

