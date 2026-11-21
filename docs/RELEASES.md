# Releases — how every change ships installable APKs

Every change merged to `main` is automatically built into a **release bundle**:
four signed Android APKs published as a GitHub Release tagged
`v0.4.<run_number>`. No manual step, no local machine — the workflow in
`.github/workflows/release.yml` owns the whole path.

## What a release contains

| APK | app | what it is |
| --- | --- | --- |
| `citizen-v0.4.N.apk` | City Market Citizen | shopper app: browse, order, track |
| `shop-v0.4.N.apk` | City Market Shop | shop console: order queue, confirm |
| `rider-v0.4.N.apk` | City Market Rider | rider app: deliveries, payouts |
| `admin-v0.4.N.apk` | City Market Admin | admin console: shops, catalog, Ujrah |

- **Versioning**: `versionName` is `0.4.<run_number>`, `versionCode` is the
  workflow run number, so both grow with every merged change. Gaps in the
  sequence are failed runs — never half-published releases.
- **Artifact retention**: the workflow's upload artifact lives 1 day
  (`retention-days: 1`, per the storage policy); **GitHub Releases are the
  permanent home** of the APKs.
- **Notes**: each release body lists the commits since the previous release
  tag, install steps, and the signing certificate SHA-256 fingerprint.

## Installing on a phone

Requires Android 5.0+ (minSdk 21). Download an APK from
[github.com/webdevshayan/city-market/releases](https://github.com/webdevshayan/city-market/releases),
tap it, and allow "install unknown apps" for the browser once — after that,
updates install in place (when releases share the release signature, see
below).

## Signing

The build prefers the **project release key** — a PKCS12 keystore held in
Actions secrets, never committed. When the secrets are absent (forks, or
before the owner sets them), the build falls back to the debug key and the
release notes say so loudly:

- **Debug-signed releases**: the signing key is ephemeral per CI run, so
  uninstall the previous build before installing an update.
- **Release-key releases**: updates install in place over all previous
  release-key builds, and the certificate fingerprint in the release body
  lets anyone verify the signature.

### Owner setup (one-time): activate the release key

1. Download `release-keystore.p12` and `SIGNING-README.txt` (delivered with
   the Phase 5 handoff) and store both somewhere safe — the keystore is the
   identity of every future release; losing it means users must reinstall.
2. On GitHub: **Settings → Secrets and variables → Actions → New repository
   secret**, four times:

   | Secret | value |
   | --- | --- |
   | `ANDROID_KEYSTORE_B64` | `base64 -w0 release-keystore.p12` output |
   | `ANDROID_KEYSTORE_PASSWORD` | the `storePassword` from `SIGNING-README.txt` |
   | `ANDROID_KEY_ALIAS` | `citymarket` |
   | `ANDROID_KEY_PASSWORD` | the `keyPassword` from `SIGNING-README.txt` |

3. Push or re-run the release workflow (`Actions → Release APKs → Run
   workflow`). From that run on, every release is signed with the stable key.

## Honest limits

- The API base URL is **baked at build time**
  (`--dart-define=API_BASE_URL`, currently the local-dev default
  `http://10.0.2.2:3000`). Until a production URL is deployed and wired into
  the workflow, released APKs point at that default — perfect for emulator
  testing against a laptop, not yet for real shoppers.
- APKs only, no `.aab` (Play Store bundle) and no iOS build.
- One universal APK per app (arm32 + arm64 + x86_64); per-ABI splits would
  roughly halve the download size if it ever matters.
