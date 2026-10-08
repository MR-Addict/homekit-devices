#!/bin/sh
set -eu

module_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$module_root"
hap_version=v0.0.35
hap_module=github.com/brutella/hap
hap_patch="$module_root/patches/hap-$hap_version.patch"
hap_target="$module_root/third_party/hap"

fail() {
    echo "prepare-hap: $*" >&2
    exit 1
}

command -v go >/dev/null 2>&1 || fail "Go is required"
command -v patch >/dev/null 2>&1 || fail "patch is required"
test -f "$hap_patch" || fail "missing patch for $hap_version"
hap_required=$(awk '$1 == "github.com/brutella/hap" { print $2; exit }' go.mod)
test "$hap_required" = "$hap_version" || fail "go.mod must pin $hap_module $hap_version; update the patch and script together"
# cksum is a portable cache invalidation key, not a security checksum.
# Go verifies the downloaded upstream module against go.sum.
hap_stamp=$(printf '%s\n%s\n%s\n' "$hap_version" "$(cksum < "$hap_patch")" "$(cksum < "$module_root/scripts/prepare-hap.sh")")
if test -f "$hap_target/.prepared" && test "$(cat "$hap_target/.prepared")" = "$hap_stamp" &&
    test -f "$hap_target/go.mod" && test -f "$hap_target/LICENSE" &&
    test -f "$hap_target/characteristic/c.go" && test -f "$hap_target/server.go" &&
    test -f "$hap_target/listen_tcp.go" && test -f "$hap_target/listen_tcp_test.go"; then
    echo "prepare-hap: $hap_version is up to date"
    exit 0
fi

# An explicit version downloads the original module despite the local replace.
# Never chmod or patch the shared module cache.
go mod download "$hap_module@$hap_version"
hap_cache_root=$(go env GOMODCACHE)
hap_source="$hap_cache_root/$hap_module@$hap_version"
test -f "$hap_source/go.mod" && test -f "$hap_source/LICENSE" || fail "upstream module unavailable"
mkdir -p "$module_root/third_party"
hap_work=$(mktemp -d "$module_root/third_party/.hap-prepare.XXXXXX")
hap_stage="$hap_work/module"
hap_previous="$hap_work/previous"
cleanup() {
    # Restore the last good dependency if installation failed after moving it.
    if test -d "$hap_previous" && ! test -e "$hap_target"; then
        mv "$hap_previous" "$hap_target"
    fi
    rm -rf "$hap_work"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
mkdir "$hap_stage"
cp -R "$hap_source/." "$hap_stage/"
chmod -R u+w "$hap_stage"
if ! patch -d "$hap_stage" -p1 -F0 -t -N < "$hap_patch"; then
    fail "patch failed; existing dependency preserved, no unpatched fallback"
fi
printf '%s\n' "$hap_stamp" > "$hap_stage/.prepared"
if test -e "$hap_target"; then
    mv "$hap_target" "$hap_previous"
fi
mv "$hap_stage" "$hap_target"
echo "prepare-hap: prepared $hap_module $hap_version"
