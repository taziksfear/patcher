#!/usr/bin/env bash
# Build the full patcher as a single Go binary with the C# patcher exe embedded.
#
# Pipeline:
#   1. dotnet build dlls/       → produces _patcher.dll (the injectable runtime, built from source)
#   2. copy that dll into patcher/_patcher.dll (the file patcher.csproj embeds)
#   3. dotnet publish patcher/  → produces osu_patcher.exe (self-contained, win-x86)
#   4. copy that exe into ui/patchersource/osu_patcher.exe (the //go:embed target)
#   5. go build the Go UI with the webkit tag
#
# Output: ui/patchersource/patcher  (the single shipable binary)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DLLS_DIR="$ROOT/dlls"
PATCHER_DIR="$ROOT/patcher"
GO_DIR="$ROOT/ui/patchersource"
EMBED_TARGET="$GO_DIR/osu_patcher.exe"

echo "── [1/5] dotnet build (dlls → _patcher.dll)"
dotnet build "$DLLS_DIR/Patcher.csproj" \
    -c Release \
    --nologo --verbosity quiet

BUILT_DLL="$(find "$DLLS_DIR/bin/Release" -maxdepth 1 -name '_patcher.dll' | head -n1)"
if [[ ! -f "$BUILT_DLL" ]]; then
    echo "[!] dotnet build did not produce _patcher.dll in $DLLS_DIR/bin/Release" >&2
    exit 1
fi

echo "── [2/5] copy → $PATCHER_DIR/_patcher.dll"
cp "$BUILT_DLL" "$PATCHER_DIR/_patcher.dll"

echo "── [3/5] dotnet publish (patcher → win-x86, self-contained)"
dotnet publish "$PATCHER_DIR/patcher.csproj" \
    -c Release \
    -r win-x86 \
    --self-contained true \
    -p:PublishSingleFile=true \
    -p:IncludeNativeLibrariesForSelfExtract=true \
    -o "$PATCHER_DIR/publish" \
    --nologo --verbosity quiet

PUBLISHED_EXE="$PATCHER_DIR/publish/osu_patcher.exe"
if [[ ! -f "$PUBLISHED_EXE" ]]; then
    echo "[!] dotnet publish did not produce $PUBLISHED_EXE" >&2
    exit 1
fi

echo "── [4/5] copy → $EMBED_TARGET"
cp "$PUBLISHED_EXE" "$EMBED_TARGET"
echo "    $(du -h "$EMBED_TARGET" | cut -f1)  $EMBED_TARGET"

echo "── [5/5] go build (webkit2_4_1)"
cd "$GO_DIR"
go build -tags webkit2_4_1 -o patcher .

echo
echo "✓ done → $GO_DIR/patcher"
ls -lh "$GO_DIR/patcher"
