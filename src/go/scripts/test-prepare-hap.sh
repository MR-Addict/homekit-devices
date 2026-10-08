#!/bin/sh
# Isolated integration checks; never touch the working module or live devices.
set -eu
module_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$module_root"
./scripts/prepare-hap.sh
hap_cache_root=$(go env GOMODCACHE)
hap_source="$hap_cache_root/github.com/brutella/hap@v0.0.35"
hap_test_work=$(mktemp -d "${TMPDIR:-/tmp}/hap-prepare-test.XXXXXX")
trap 'rm -rf "$hap_test_work"' EXIT
trap 'exit 1' HUP INT TERM
hap_fixture="$hap_test_work/module"
mkdir -p "$hap_fixture/scripts" "$hap_fixture/patches"
cp go.mod go.sum "$hap_fixture/"
cp scripts/prepare-hap.sh "$hap_fixture/scripts/"
cp patches/hap-v0.0.35.patch "$hap_fixture/patches/"

snapshot() {
    (cd "$1" && find . -type f -exec cksum {} \; | LC_ALL=C sort)
}
snapshot "$hap_source" > "$hap_test_work/cache.before"
# Compare file and directory permissions as well as contents.
find "$hap_source" -exec ls -ld {} \; | LC_ALL=C sort > "$hap_test_work/modes.before"

"$hap_fixture/scripts/prepare-hap.sh"
hap_generated="$hap_fixture/third_party/hap"
test -f "$hap_generated/LICENSE"
snapshot "$hap_generated" > "$hap_test_work/first"
"$hap_fixture/scripts/prepare-hap.sh" > "$hap_test_work/reuse.log"
grep -q 'up to date' "$hap_test_work/reuse.log"
snapshot "$hap_generated" > "$hap_test_work/second"
cmp "$hap_test_work/first" "$hap_test_work/second"

cp "$hap_generated/.prepared" "$hap_test_work/stamp.before"
printf '\n# Cache invalidation check.\n' >> "$hap_fixture/patches/hap-v0.0.35.patch"
"$hap_fixture/scripts/prepare-hap.sh" > "$hap_test_work/changed.log"
grep -q 'prepared github.com/brutella/hap' "$hap_test_work/changed.log"
if cmp -s "$hap_test_work/stamp.before" "$hap_generated/.prepared"; then
    echo 'patch change did not invalidate cache' >&2
    exit 1
fi
snapshot "$hap_generated" > "$hap_test_work/good"

# Force a hunk to fail against the pinned source after invalidating its stamp.
sed 's/SetValueRequestFunc func(value interface{}/MissingUpstreamSymbol func(value interface{}/' \
    "$hap_fixture/patches/hap-v0.0.35.patch" > "$hap_test_work/broken.patch"
mv "$hap_test_work/broken.patch" "$hap_fixture/patches/hap-v0.0.35.patch"
if "$hap_fixture/scripts/prepare-hap.sh" > "$hap_test_work/failure.log" 2>&1; then
    echo 'invalid patch unexpectedly succeeded' >&2
    exit 1
fi
grep -q 'existing dependency preserved' "$hap_test_work/failure.log"
snapshot "$hap_generated" > "$hap_test_work/after-failure"
cmp "$hap_test_work/good" "$hap_test_work/after-failure"
if find "$hap_fixture/third_party" -name '.hap-prepare.*' | grep -q .; then
    echo 'temporary staging directory leaked' >&2
    exit 1
fi

# A broken patch on a clean clone must not install an unpatched dependency.
rm -rf "$hap_generated"
if "$hap_fixture/scripts/prepare-hap.sh" > "$hap_test_work/clean-failure.log" 2>&1; then
    echo 'invalid patch unexpectedly succeeded on clean bootstrap' >&2
    exit 1
fi
test ! -e "$hap_generated"

snapshot "$hap_source" > "$hap_test_work/cache.after"
find "$hap_source" -exec ls -ld {} \; | LC_ALL=C sort > "$hap_test_work/modes.after"
cmp "$hap_test_work/cache.before" "$hap_test_work/cache.after"
cmp "$hap_test_work/modes.before" "$hap_test_work/modes.after"
echo 'prepare-hap integration checks passed: clean bootstrap, reuse, invalidation, failure preservation, unchanged module cache'
