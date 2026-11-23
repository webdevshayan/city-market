# Phase 5 — APK Releases: every change ships installable Android apps

Status: **complete, awaiting review**. The four Flutter apps were code-complete
and CI-verified since Phase 4, but nobody could actually install them — there
was no `android/` platform folder, no signing story, no release channel. This
phase closes that gap: every change merged to `main` now produces a versioned
GitHub Release containing four installable APKs.

## android/ platform scaffolding (citizen, shop, rider, admin)

Generated with `flutter create --platforms=android --org dev.citymarket` using
each app's pubspec `name:` (so Dart imports stayed intact), then hand-tuned:

- **INTERNET permission in every main manifest.** Flutter's template only
  grants INTERNET to debug and profile builds; a release APK built straight
  from the template would have shipped offline. All four apps talk to the Go
  API, so the permission is now explicit in `android/app/src/main/`.
- **User-facing labels** — "City Market Citizen / Shop / Rider / Admin"
  instead of raw package names on the launcher.
- **Gradle wrapper 8.3 → 8.5.** The template pins Gradle 8.3, which rejects
  JDK 21 (the dev machine's Java); 8.5 accepts both JDK 17 (CI) and JDK 21
  (dev) without forking the template's AGP settings.
- **applicationId** `dev.citymarket.<app>` — one namespace, stable across
  releases, so updates install in place over previous versions.

## Signing: stable release key, safe fallback

`android/app/build.gradle` in every app now loads `android/keystore.properties`
(storeFile, storeType, storePassword, keyAlias, keyPassword) and signs release
builds with it. The properties file is **never committed**:

- CI assembles it from four Actions secrets: `ANDROID_KEYSTORE_B64` (the
  base64 PKCS12 keystore), `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`
  (citymarket), `ANDROID_KEY_PASSWORD`.
- Until the secrets are set, the workflow prints a loud `::warning::` and
  ships debug-signed APKs. Debug keys are ephemeral per CI run, so each
  debug-signed release requires uninstall-then-reinstall; releases become
  update-in-place the moment the secrets are set. No workflow change needed.
- The release key itself was generated once with `keytool` (PKCS12, RSA 2048,
  30-year validity, CN=City Market Release) and is kept out of the
  repository; its certificate SHA-256 fingerprint is printed into every
  release body so installs can be verified.

## Release workflow (`.github/workflows/release.yml`)

Runs on every push to `main` and on `workflow_dispatch`:

1. Restore keystore from secrets (or warn and fall back).
2. Build all four apps: `flutter build apk --release --build-name=0.4.N
   --build-number=N` where `N` is the workflow run number.
3. Sanity-check each APK (> 1 MB), collect into `dist/<app>-v0.4.N.apk`.
4. Write release notes: app table, changes since the previous release tag,
   install steps, signature fingerprint.
5. Upload the bundle as a 1-day-retention artifact (storage policy), then
   publish GitHub Release `v0.4.<run_number>` with the four APKs attached.

Deliberate decisions: **no tag trigger** (the release step creates the tag —
a tag trigger would re-run this workflow on its own tag pushes and loop);
`cancel-in-progress: false` (never kill a release halfway); Gradle caches
between runs (`~/.gradle`, keyed on the wrapper/build files). Failed runs
leave gaps in the version sequence — a gap is simply a red run, never a
half-published release.

## Verified locally

- `flutter create` scaffolds cleanly; manifests verified for INTERNET,
  labels and `applicationId`; gradle wiring asserted by script (3/3 checks
  per app) and YAML validated with a parser.
- A real local validation build ran the full release path: Android SDK
  (cmdline-tools + platform 34 + build-tools 34) installed under `.tools/`,
  `flutter build apk --release` succeeded on the citizen app with the
  **project release keystore** wired through `keystore.properties`, and
  `apksigner verify` confirmed the APK is signed by CN=City Market Release.
- CI remains the authoritative gate: Go tests, stylesheet drift check, and
  Flutter analyze/test all run on the PR before the release workflow ever
  fires on `main`.

## Honest limits

- The remote Turso path and a Vercel deployment are still unverified
  (Phase 3 note stands); released APKs bake the local-dev API base
  (`http://10.0.2.2:3000`) until a production `API_BASE_URL` is chosen.
- No Play Store / signed bundle (.aab) — side-load APKs only.
- One universal APK per app (arm32 + arm64 + x86_64); per-ABI splits are a
  size optimization for later.
