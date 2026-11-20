# citizen (customer app)

Flutter app for customers ordering from certified-Halal shops. Product-first
UI: image, name, price, add to cart — store/location details stay out of the
browse experience per the product spec.

- Checkout charges **one flat delivery fee** (see `server/finance`).
- Talks to the Go server API only; no business rules in the client.

Run (once the Flutter SDK is available): `flutter run` from this directory.
Full implementation lands in Phase 4.
