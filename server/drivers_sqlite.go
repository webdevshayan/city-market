//go:build sqlite

// Development build tag: embed a pure-Go sqlite driver so local file: DSNs
// work without a running Turso instance — e.g.
//
//	DATABASE_URL=file:/tmp/dev.db go run -tags sqlite ./server
//
// The default production build stays lean and speaks remote libsql:// only
// (the libsql driver routes file: DSNs to whatever sqlite driver is
// registered, and production registers none — a file: DSN there fails loudly
// and the server falls back to seed fixtures).
package main

import _ "modernc.org/sqlite"
