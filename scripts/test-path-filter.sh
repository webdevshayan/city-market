#!/usr/bin/env bash
# Mirrors the path-scoping logic of the `changes` jobs in
# .github/workflows/ci.yml and .github/workflows/release.yml and asserts it
# against real commits from this repo's history. Run from the repo root:
#   bash scripts/test-path-filter.sh
# Any FAIL (exit 1) means the workflow filters and this mirror have drifted
# — fix both together.
set -u
REPO="$(git rev-parse --show-toplevel)"
fail=0

# ci_filter BEFORE AFTER EVENT -> prints "apps= server= web=" (sorted)
ci_filter() {
	local BEFORE="$1" AFTER="$2" EVENT="$3"
	local GITHUB_OUTPUT; GITHUB_OUTPUT=$(mktemp)
	all() {
		echo "server=true" >> "$GITHUB_OUTPUT"
		echo "web=true" >> "$GITHUB_OUTPUT"
		echo "apps=true" >> "$GITHUB_OUTPUT"
	}
	if [ "$EVENT" != "push" ] || [ -z "$BEFORE" ] || [ "$BEFORE" = "0000000000000000000000000000000000000000" ]; then
		all
	elif grep -q '^\.github/workflows/ci\.yml' < <(git -C "$REPO" diff --name-only "$BEFORE" "$AFTER"); then
		all
	else
		changed=$(git -C "$REPO" diff --name-only "$BEFORE" "$AFTER")
		if grep -q '^server/' <<< "$changed"; then echo "server=true" >> "$GITHUB_OUTPUT"; else echo "server=false" >> "$GITHUB_OUTPUT"; fi
		if grep -q '^web/' <<< "$changed"; then echo "web=true" >> "$GITHUB_OUTPUT"; else echo "web=false" >> "$GITHUB_OUTPUT"; fi
		if grep -q '^apps/' <<< "$changed"; then echo "apps=true" >> "$GITHUB_OUTPUT"; else echo "apps=false" >> "$GITHUB_OUTPUT"; fi
	fi
	sort "$GITHUB_OUTPUT" | tr '\n' ' ' | sed 's/ *$//'; echo
	rm -f "$GITHUB_OUTPUT"
}

# release_filter BEFORE AFTER EVENT -> prints "apps="
release_filter() {
	local BEFORE="$1" AFTER="$2" EVENT="$3"
	local GITHUB_OUTPUT; GITHUB_OUTPUT=$(mktemp)
	if [ "$EVENT" != "push" ] || [ -z "$BEFORE" ] || [ "$BEFORE" = "0000000000000000000000000000000000000000" ]; then
		echo "apps=true" >> "$GITHUB_OUTPUT"
	elif grep -q '^\.github/workflows/release\.yml' < <(git -C "$REPO" diff --name-only "$BEFORE" "$AFTER"); then
		echo "apps=true" >> "$GITHUB_OUTPUT"
	else
		changed=$(git -C "$REPO" diff --name-only "$BEFORE" "$AFTER")
		if grep -q '^apps/' <<< "$changed"; then echo "apps=true" >> "$GITHUB_OUTPUT"; else echo "apps=false" >> "$GITHUB_OUTPUT"; fi
	fi
	tr '\n' ' ' < "$GITHUB_OUTPUT" | sed 's/ *$//'; echo
	rm -f "$GITHUB_OUTPUT"
}

# find_only PREFIX -> the most recent commit whose diff against its parent
# touches ONLY paths under PREFIX (empty when history has none).
find_only() {
	for s in $(git -C "$REPO" log --format=%H -60 -- "$1"); do
		files=$(git -C "$REPO" diff --name-only "${s}^" "$s" 2>/dev/null) || continue
		if [ -n "$files" ] && ! grep -qv "^$1/" <<< "$files"; then
			echo "$s"
			return
		fi
	done
}

check() {
	if [ "$2" = "$3" ]; then
		echo "PASS: $1 → $3"
	else
		echo "FAIL: $1 → $3 (want $2)"
		fail=1
	fi
}

echo "== ci.yml changes job =="
check "workflow_dispatch runs everything" \
	"apps=true server=true web=true" \
	"$(ci_filter x x workflow_dispatch)"

BASE="$(git -C "$REPO" rev-parse HEAD)"
check "current tree (ci.yml edited) runs everything" \
	"apps=true server=true web=true" \
	"$(ci_filter 4e0aec7eca4fdf943c472d0767f1ccf496d71da1 "$BASE" push)"

S="$(find_only server)"
if [ -n "${S:-}" ]; then
	check "server-only push: only the server jobs" \
		"apps=false server=true web=false" \
		"$(ci_filter "${S}^" "$S" push)"
else
	echo "SKIP: no server-only commit in history"
fi

W="$(find_only web)"
if [ -n "${W:-}" ]; then
	check "web-only push: only the web jobs" \
		"apps=false server=false web=true" \
		"$(ci_filter "${W}^" "$W" push)"
else
	echo "SKIP: no web-only commit in history"
fi

A="$(find_only apps)"
if [ -n "${A:-}" ]; then
	check "apps-only push: no server/web deploys" \
		"apps=true server=false web=false" \
		"$(ci_filter "${A}^" "$A" push)"
else
	echo "SKIP: no apps-only commit in history"
fi

D="$(find_only docs)"
if [ -n "${D:-}" ]; then
	check "docs-only push: nothing runs" \
		"apps=false server=false web=false" \
		"$(ci_filter "${D}^" "$D" push)"
else
	echo "SKIP: no docs-only commit in history"
fi

echo "== release.yml changes job =="
if [ -n "${A:-}" ]; then
	check "apps-only push: APK bundle rebuilds" \
		"apps=true" \
		"$(release_filter "${A}^" "$A" push)"
fi
if [ -n "${S:-}" ]; then
	check "server-only push: no APK rebuild" \
		"apps=false" \
		"$(release_filter "${S}^" "$S" push)"
fi
check "workflow_dispatch forces a release build" \
	"apps=true" \
	"$(release_filter x x workflow_dispatch)"
check "release.yml edit forces a rebuild" \
	"apps=true" \
	"$(release_filter "$BASE^" "$BASE" push)"

if [ "$fail" -ne 0 ]; then
	echo "one or more assertions failed"
	exit 1
fi
echo "all path-filter assertions passed"
