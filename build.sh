#!/usr/bin/env bash
# Build the full patcher as a single Go binary with the C# patcher exe embedded.
#
# Pipeline:
#   1. dotnet publish patcher/  → produces osu_patcher.exe (self-contained, win-x86)
#   2. copy that exe into ui/patchersource/osu_patcher.exe (the //go:embed target)
#   3. go build the Go UI with the webkit tag
#
# Output: ui/patchersource/patcher  (the single shipable binary)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCHER_DIR="$ROOT/patcher"
GO_DIR="$ROOT/ui/patchersource"
EMBED_TARGET="$GO_DIR/osu_patcher.exe"

echo "── [1/3] dotnet publish (patcher → win-x86, self-contained)"
dotnet publish "$PATCHER_DIR/patcher.csproj" \
    -c Release \
    -r win-x86 \
    --self-contained true \
    -p:PublishSingleFile=true \
    -p:IncludeNativeLibrariesForSelfExtract=true \
    -o "$PATCHER_DIR/publish" \
    --nologo --verbosity quiet

# AssemblyName in patcher.csproj is "osu!" so the published exe is named "osu!.exe".
PUBLISHED_EXE="$PATCHER_DIR/publish/osu!.exe"
if [[ ! -f "$PUBLISHED_EXE" ]]; then
    PUBLISHED_EXE="$(find "$PATCHER_DIR/publish" -maxdepth 1 -name '*.exe' | head -n1)"
fi
if [[ ! -f "$PUBLISHED_EXE" ]]; then
    echo "[!] dotnet publish did not produce a .exe in $PATCHER_DIR/publish" >&2
    exit 1
fi

echo "── [2/3] copy → $EMBED_TARGET"
cp "$PUBLISHED_EXE" "$EMBED_TARGET"
echo "    $(du -h "$EMBED_TARGET" | cut -f1)  $EMBED_TARGET"

echo "── [3/3] go build (webkit2_4_1)"
cd "$GO_DIR"
go build -tags webkit2_4_1 -o patcher .

echo
echo "✓ done → $GO_DIR/patcher"
ls -lh "$GO_DIR/patcher"
