/// City Market's design system — one theme factory and a handful of
/// components shared by all four apps. Green and white only, square corners
/// (zero radius everywhere — deliberately not the platform's default look),
/// flat hairline dividers instead of shadows, and the brand green reserved
/// for primary actions and active states so it stays meaningful.
///
/// Accessibility is built in, not bolted on: every color pair in
/// CMColors passes WCAG AA on text (verified in PR), interactive targets
/// are at least 48×48, sections are announced as headers to screen
/// readers, icon-only information always carries a text or semantics
/// label, and the theme keeps Material's text scaling and focus behavior.
library;

import 'package:core/models.dart';
import 'package:flutter/material.dart';

/// The palette. All fg/bg pairs used by the theme reach WCAG AA contrast.
abstract final class CMColors {
  /// halal-700 — primary buttons and active states, used sparingly.
  static const green = Color(0xFF15803D);

  /// halal-900 — section headers and strong accents on white.
  static const greenDeep = Color(0xFF14532D);

  static const onGreen = Color(0xFFFFFFFF);
  static const ink = Color(0xFF111827);
  static const inkMuted = Color(0xFF374151);
  static const paper = Color(0xFFFFFFFF);
  static const wash = Color(0xFFF3F4F6);
  static const hairline = Color(0xFFD1D5DB);
  static const danger = Color(0xFFB91C1C);
}

/// The shared theme. Square, flat, white surfaces, hairline strokes.
ThemeData buildCMTheme() {
  const scheme = ColorScheme.light(
    primary: CMColors.green,
    onPrimary: CMColors.onGreen,
    secondary: CMColors.greenDeep,
    onSecondary: CMColors.onGreen,
    surface: CMColors.paper,
    onSurface: CMColors.ink,
    error: CMColors.danger,
    onError: CMColors.onGreen,
    outline: CMColors.hairline,
    outlineVariant: CMColors.hairline,
    surfaceContainerHighest: CMColors.wash,
  );
  final base = ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: CMColors.paper,
    // No ink splashes: state changes carry the feedback instead.
    splashFactory: NoSplash.splashFactory,
    materialTapTargetSize: MaterialTapTargetSize.padded,
  );
  const zero = RoundedRectangleBorder(borderRadius: BorderRadius.zero);
  return base.copyWith(
    appBarTheme: const AppBarTheme(
      backgroundColor: CMColors.paper,
      foregroundColor: CMColors.ink,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      shape: Border(bottom: BorderSide(color: CMColors.hairline)),
      titleTextStyle: TextStyle(
          fontSize: 18, fontWeight: FontWeight.w700, color: CMColors.ink),
    ),
    textTheme: base.textTheme.apply(
        bodyColor: CMColors.ink, displayColor: CMColors.ink),
    dividerTheme:
        const DividerThemeData(color: CMColors.hairline, thickness: 1, space: 1),
    chipTheme: base.chipTheme.copyWith(
      backgroundColor: CMColors.wash,
      side: const BorderSide(color: CMColors.hairline),
      shape: zero,
      labelStyle: const TextStyle(
          color: CMColors.inkMuted, fontWeight: FontWeight.w600),
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: ButtonStyle(
        backgroundColor: const WidgetStatePropertyAll(CMColors.green),
        foregroundColor: const WidgetStatePropertyAll(CMColors.onGreen),
        side: const WidgetStatePropertyAll(
            BorderSide(color: CMColors.greenDeep)),
        shape: const WidgetStatePropertyAll(zero),
        minimumSize: const WidgetStatePropertyAll(Size(48, 48)),
        padding:
            const WidgetStatePropertyAll(EdgeInsets.symmetric(horizontal: 20, vertical: 14)),
        textStyle: const WidgetStatePropertyAll(
            TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: ButtonStyle(
        backgroundColor: const WidgetStatePropertyAll(CMColors.paper),
        foregroundColor: const WidgetStatePropertyAll(CMColors.ink),
        side: const WidgetStatePropertyAll(BorderSide(color: CMColors.hairline)),
        shape: const WidgetStatePropertyAll(zero),
        minimumSize: const WidgetStatePropertyAll(Size(48, 48)),
        padding:
            const WidgetStatePropertyAll(EdgeInsets.symmetric(horizontal: 20, vertical: 14)),
        textStyle: const WidgetStatePropertyAll(
            TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: ButtonStyle(
        foregroundColor: const WidgetStatePropertyAll(CMColors.green),
        textStyle: const WidgetStatePropertyAll(
            TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
        minimumSize: const WidgetStatePropertyAll(Size(48, 48)),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      border: OutlineInputBorder(
          borderRadius: BorderRadius.zero,
          borderSide: const BorderSide(color: CMColors.hairline)),
      enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.zero,
          borderSide: const BorderSide(color: CMColors.hairline)),
      focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.zero,
          borderSide: const BorderSide(color: CMColors.green, width: 2)),
      labelStyle: const TextStyle(color: CMColors.inkMuted),
      floatingLabelStyle: const TextStyle(color: CMColors.greenDeep),
      filled: true,
      fillColor: CMColors.paper,
      contentPadding:
          const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
    ),
    listTileTheme: const ListTileThemeData(
      iconColor: CMColors.greenDeep,
      textColor: CMColors.ink,
      contentPadding: EdgeInsets.symmetric(horizontal: 8),
    ),
    snackBarTheme: const SnackBarThemeData(
      backgroundColor: CMColors.ink,
      contentTextStyle: TextStyle(color: CMColors.onGreen),
      behavior: SnackBarBehavior.floating,
      shape: zero,
    ),
    dialogTheme: const DialogThemeData(
        shape: zero, backgroundColor: CMColors.paper),
    bottomSheetTheme: const BottomSheetThemeData(shape: zero, backgroundColor: CMColors.paper),
    progressIndicatorTheme: const ProgressIndicatorThemeData(
        color: CMColors.green, linearTrackColor: CMColors.wash),
    switchTheme: SwitchThemeData(
      thumbColor: const WidgetStatePropertyAll(CMColors.paper),
      trackColor: WidgetStateProperty.resolveWith(
        (states) => states.contains(WidgetState.selected)
            ? CMColors.green
            : CMColors.hairline,
      ),
    ),
    iconButtonTheme: const IconButtonThemeData(
      style: ButtonStyle(iconColor: WidgetStatePropertyAll(CMColors.greenDeep)),
    ),
    floatingActionButtonTheme: const FloatingActionButtonThemeData(
        shape: zero, backgroundColor: CMColors.green, foregroundColor: CMColors.onGreen),
  );
}

/// An uppercased section title announced as a header by screen readers.
class CMSectionHeader extends StatelessWidget {
  final String text;

  const CMSectionHeader(this.text, {super.key});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Padding(
        padding: const EdgeInsets.only(top: 24, bottom: 8),
        child: Text(
          text.toUpperCase(),
          style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.1,
              color: CMColors.greenDeep),
        ),
      ),
    );
  }
}

/// One statistic: a muted uppercase label over a large value, announced to
/// screen readers as one complete line ("Revenue: 45,750.00").
class CMStatTile extends StatelessWidget {
  final String label;
  final String value;
  final String? hint;

  const CMStatTile({super.key, required this.label, required this.value, this.hint});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: hint == null ? '$label: $value' : '$label: $value. $hint',
      excludeSemantics: true,
      child: Container(
        decoration: BoxDecoration(
          color: CMColors.paper,
          border: Border.all(color: CMColors.hairline),
        ),
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              label.toUpperCase(),
              style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.8,
                  color: CMColors.inkMuted),
            ),
            const SizedBox(height: 6),
            FittedBox(
              fit: BoxFit.scaleDown,
              child: Text(
                value,
                style: const TextStyle(
                    fontSize: 24, fontWeight: FontWeight.w700, color: CMColors.ink),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// An order status badge: icon + word, never color alone.
class CMStatusChip extends StatelessWidget {
  final String status;

  const CMStatusChip(this.status, {super.key});

  @override
  Widget build(BuildContext context) {
    final (IconData icon, Color color, String word) = switch (status) {
      statusDelivered => (Icons.check_circle_outline, CMColors.green, 'Delivered'),
      statusCancelled => (Icons.cancel_outlined, CMColors.danger, 'Cancelled'),
      statusConfirmed => (Icons.task_alt_outlined, CMColors.greenDeep, 'Confirmed'),
      statusPickedUp => (Icons.local_shipping_outlined, CMColors.greenDeep, 'Picked up'),
      _ => (Icons.schedule_outlined, CMColors.inkMuted, 'Placed'),
    };
    return Semantics(
      label: 'Status: $word',
      excludeSemantics: true,
      child: Container(
        decoration: BoxDecoration(
          border: Border.all(color: color),
          color: CMColors.paper,
        ),
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: color),
            const SizedBox(width: 4),
            Text(word,
                style: TextStyle(
                    fontSize: 12, fontWeight: FontWeight.w600, color: color)),
          ],
        ),
      ),
    );
  }
}

/// The load-failure state: what failed, the server's reason, and a labelled
/// retry button — never an endless spinner.
class CMErrorRetry extends StatelessWidget {
  final Object error;
  final VoidCallback onRetry;

  const CMErrorRetry({super.key, required this.error, required this.onRetry});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.cloud_off_outlined, size: 40, color: CMColors.inkMuted),
            const SizedBox(height: 12),
            Text('Could not reach the market server',
                style: Theme.of(context)
                    .textTheme
                    .titleMedium
                    ?.copyWith(fontWeight: FontWeight.w700)),
            const SizedBox(height: 4),
            Text(
              '$error',
              textAlign: TextAlign.center,
              style: const TextStyle(color: CMColors.inkMuted),
            ),
            const SizedBox(height: 16),
            OutlinedButton.icon(
              onPressed: onRetry,
              icon: const Icon(Icons.refresh),
              label: const Text('Try again'),
            ),
          ],
        ),
      ),
    );
  }
}
