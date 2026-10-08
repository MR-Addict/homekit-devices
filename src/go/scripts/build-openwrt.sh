#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
./scripts/prepare-hap.sh
mkdir -p bin
for name in homekit-wol homekit-temperature; do
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "bin/$name-linux-arm64" "./cmd/$name"
done
