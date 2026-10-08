#!/bin/sh
# Isolated integration checks; never touch the working module or live devices.
set -eu
module_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$module_root"
./scripts/prepare-atvremote.sh
atvremote_cache_root=$(go env GOMODCACHE)
atvremote_source="$atvremote_cache_root/github.com/jkiddo/atvremote@v0.0.0-20260806215925-ab5a73b3c68d"
atvremote_test_work=$(mktemp -d "${TMPDIR:-/tmp}/atvremote-prepare-test.XXXXXX")
trap 'rm -rf "$atvremote_test_work"' EXIT
trap 'exit 1' HUP INT TERM
atvremote_fixture="$atvremote_test_work/module"
mkdir -p "$atvremote_fixture/scripts" "$atvremote_fixture/patches"
cp go.mod go.sum "$atvremote_fixture/"
cp scripts/prepare-atvremote.sh "$atvremote_fixture/scripts/"
cp patches/atvremote.patch "$atvremote_fixture/patches/"

snapshot() {
    (cd "$1" && find . -type f -exec cksum {} \; | LC_ALL=C sort)
}
snapshot "$atvremote_source" > "$atvremote_test_work/cache.before"
# Compare file and directory permissions as well as contents.
find "$atvremote_source" -exec ls -ld {} \; | LC_ALL=C sort > "$atvremote_test_work/modes.before"

"$atvremote_fixture/scripts/prepare-atvremote.sh"
atvremote_generated="$atvremote_fixture/third_party/atvremote"
test -f "$atvremote_generated/LICENCE"
snapshot "$atvremote_generated" > "$atvremote_test_work/first"
"$atvremote_fixture/scripts/prepare-atvremote.sh" > "$atvremote_test_work/reuse.log"
grep -q 'up to date' "$atvremote_test_work/reuse.log"
snapshot "$atvremote_generated" > "$atvremote_test_work/second"
cmp "$atvremote_test_work/first" "$atvremote_test_work/second"

cp "$atvremote_generated/.prepared" "$atvremote_test_work/stamp.before"
printf '\n# Cache invalidation check.\n' >> "$atvremote_fixture/patches/atvremote.patch"
"$atvremote_fixture/scripts/prepare-atvremote.sh" > "$atvremote_test_work/changed.log"
grep -q 'prepared github.com/jkiddo/atvremote' "$atvremote_test_work/changed.log"
if cmp -s "$atvremote_test_work/stamp.before" "$atvremote_generated/.prepared"; then
    echo 'patch change did not invalidate cache' >&2
    exit 1
fi
snapshot "$atvremote_generated" > "$atvremote_test_work/good"

# Force a hunk to fail against the pinned source after invalidating its stamp.
sed 's/imeFieldCounter int32/missingUpstreamField int32/' \
    "$atvremote_fixture/patches/atvremote.patch" > "$atvremote_test_work/broken.patch"
mv "$atvremote_test_work/broken.patch" "$atvremote_fixture/patches/atvremote.patch"
if "$atvremote_fixture/scripts/prepare-atvremote.sh" > "$atvremote_test_work/failure.log" 2>&1; then
    echo 'invalid patch unexpectedly succeeded' >&2
    exit 1
fi
grep -q 'existing dependency preserved' "$atvremote_test_work/failure.log"
snapshot "$atvremote_generated" > "$atvremote_test_work/after-failure"
cmp "$atvremote_test_work/good" "$atvremote_test_work/after-failure"
if find "$atvremote_fixture/third_party" -name '.atvremote-prepare.*' | grep -q .; then
    echo 'temporary staging directory leaked' >&2
    exit 1
fi

# A broken patch on a clean clone must not install an unpatched dependency.
rm -rf "$atvremote_generated"
if "$atvremote_fixture/scripts/prepare-atvremote.sh" > "$atvremote_test_work/clean-failure.log" 2>&1; then
    echo 'invalid patch unexpectedly succeeded on clean bootstrap' >&2
    exit 1
fi
test ! -e "$atvremote_generated"

snapshot "$atvremote_source" > "$atvremote_test_work/cache.after"
find "$atvremote_source" -exec ls -ld {} \; | LC_ALL=C sort > "$atvremote_test_work/modes.after"
cmp "$atvremote_test_work/cache.before" "$atvremote_test_work/cache.after"
cmp "$atvremote_test_work/modes.before" "$atvremote_test_work/modes.after"
echo 'prepare-atvremote integration checks passed: clean bootstrap, reuse, invalidation, failure preservation, unchanged module cache'
