#!/bin/sh
set -eu

module_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$module_root"
atvremote_version=v0.0.0-20260806215925-ab5a73b3c68d
atvremote_module=github.com/jkiddo/atvremote
atvremote_patch="$module_root/patches/atvremote.patch"
atvremote_target="$module_root/third_party/atvremote"

fail() {
    echo "prepare-atvremote: $*" >&2
    exit 1
}

command -v go >/dev/null 2>&1 || fail "Go is required"
command -v patch >/dev/null 2>&1 || fail "patch is required"
test -f "$atvremote_patch" || fail "missing patch for $atvremote_version"
atvremote_required=$(awk '$1 == "github.com/jkiddo/atvremote" { print $2; exit }' go.mod)
test "$atvremote_required" = "$atvremote_version" || fail "go.mod must pin $atvremote_module $atvremote_version; update the patch and script together"
# cksum is a portable cache invalidation key, not a security checksum.
# Go verifies the downloaded upstream module against go.sum.
atvremote_stamp=$(printf '%s\n%s\n%s\n' "$atvremote_version" "$(cksum < "$atvremote_patch")" "$(cksum < "$module_root/scripts/prepare-atvremote.sh")")
if test -f "$atvremote_target/.prepared" && test "$(cat "$atvremote_target/.prepared")" = "$atvremote_stamp" &&
    test -f "$atvremote_target/go.mod" && test -f "$atvremote_target/LICENCE" &&
    test -f "$atvremote_target/pkg/v2/remote/protocol.go" &&
    test -f "$atvremote_target/pkg/v2/remote/homekit_support.go"; then
    echo "prepare-atvremote: $atvremote_version is up to date"
    exit 0
fi

# An explicit version downloads the original module despite the local replace.
# Never chmod or patch the shared module cache.
go mod download "$atvremote_module@$atvremote_version"
atvremote_cache_root=$(go env GOMODCACHE)
atvremote_source="$atvremote_cache_root/$atvremote_module@$atvremote_version"
test -f "$atvremote_source/go.mod" && test -f "$atvremote_source/LICENCE" || fail "upstream module unavailable"
mkdir -p "$module_root/third_party"
atvremote_work=$(mktemp -d "$module_root/third_party/.atvremote-prepare.XXXXXX")
atvremote_stage="$atvremote_work/module"
atvremote_previous="$atvremote_work/previous"
cleanup() {
    # Restore the last good dependency if installation failed after moving it.
    if test -d "$atvremote_previous" && ! test -e "$atvremote_target"; then
        mv "$atvremote_previous" "$atvremote_target"
    fi
    rm -rf "$atvremote_work"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
mkdir "$atvremote_stage"
cp -R "$atvremote_source/." "$atvremote_stage/"
chmod -R u+w "$atvremote_stage"
if ! patch -d "$atvremote_stage" -p1 -F0 -t -N < "$atvremote_patch"; then
    fail "patch failed; existing dependency preserved, no unpatched fallback"
fi
printf '%s\n' "$atvremote_stamp" > "$atvremote_stage/.prepared"
if test -e "$atvremote_target"; then
    mv "$atvremote_target" "$atvremote_previous"
fi
mv "$atvremote_stage" "$atvremote_target"
echo "prepare-atvremote: prepared $atvremote_module $atvremote_version"
