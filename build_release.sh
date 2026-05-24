#!/usr/bin/env bash
# Release build: produces shippable launcher binaries in ./export/
#
#   export/patcher       — Linux x86_64 (statically linked Fyne, webkit2_4_1)
#   export/patcher.exe   — Windows x86_64 (cross-compiled via mingw-w64)
#
# Both binaries embed:
#   * the C# osu_patcher.exe (win-x86, self-contained, single-file)
#   * the preset themes from ui/patchersource/themes/{Kurumi,Astolfo,Nakuru}
#
# Requirements:
#   dotnet 6+                            (for the C# patcher)
#   go 1.22+                             (for the launcher)
#   gcc + webkit2gtk-4.1 headers         (Linux build)
#   mingw-w64 (x86_64-w64-mingw32-gcc)   (Windows cross-compile)
#
# Usage:
#   ./build_release.sh              # build both
#   ./build_release.sh linux        # Linux only
#   ./build_release.sh windows      # Windows only

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCHER_DIR="$ROOT/patcher"
GO_DIR="$ROOT/ui/patchersource"
EXPORT_DIR="$ROOT/export"
EMBED_TARGET="$GO_DIR/osu_patcher.exe"

if [[ $# -eq 0 ]]; then
    TARGETS=(linux windows)
else
    TARGETS=("$@")
fi
WANT_LINUX=0
WANT_WINDOWS=0
for t in "${TARGETS[@]}"; do
    case "$t" in
        linux)   WANT_LINUX=1 ;;
        windows) WANT_WINDOWS=1 ;;
        all)     WANT_LINUX=1; WANT_WINDOWS=1 ;;
        *) echo "[!] unknown target: $t (use linux | windows | all)" >&2; exit 1 ;;
    esac
done

mkdir -p "$EXPORT_DIR"

# ── 1. dotnet publish the C# patcher (always — both targets embed it) ────────
echo "── [1/3] dotnet publish (osu_patcher.exe, win-x86, self-contained)"
dotnet publish "$PATCHER_DIR/patcher.csproj" \
    -c Release \
    -r win-x86 \
    --self-contained true \
    -p:PublishSingleFile=true \
    -p:IncludeNativeLibrariesForSelfExtract=true \
    -o "$PATCHER_DIR/publish" \
    --nologo --verbosity quiet

# AssemblyName is "osu!" so the output is "osu!.exe"; find it generically too.
PUBLISHED_EXE="$PATCHER_DIR/publish/osu!.exe"
if [[ ! -f "$PUBLISHED_EXE" ]]; then
    PUBLISHED_EXE="$(find "$PATCHER_DIR/publish" -maxdepth 1 -name '*.exe' | head -n1)"
fi
if [[ ! -f "$PUBLISHED_EXE" ]]; then
    echo "[!] dotnet publish did not produce a .exe in $PATCHER_DIR/publish" >&2
    exit 1
fi
echo "    → $(du -h "$PUBLISHED_EXE" | cut -f1)  $PUBLISHED_EXE"

echo "── [2/3] copy embed target → $EMBED_TARGET"
cp "$PUBLISHED_EXE" "$EMBED_TARGET"

cd "$GO_DIR"

# ── 3a. Linux build ──────────────────────────────────────────────────────────
if [[ "$WANT_LINUX" == "1" ]]; then
    echo "── [3/3] go build (linux/amd64, webkit2_4_1)"
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
        go build -tags webkit2_4_1 -trimpath -ldflags="-s -w" \
        -o "$EXPORT_DIR/patcher" .
    echo "    ✓ $(du -h "$EXPORT_DIR/patcher" | cut -f1)  $EXPORT_DIR/patcher"
fi

# ── 3b. Windows cross-compile ────────────────────────────────────────────────
if [[ "$WANT_WINDOWS" == "1" ]]; then
    if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
        echo "[!] x86_64-w64-mingw32-gcc not found — install mingw-w64 to build Windows binary." >&2
        echo "    Arch:    sudo pacman -S mingw-w64-gcc" >&2
        echo "    Debian:  sudo apt install mingw-w64" >&2
        exit 1
    fi
    echo "── [3/3] go build (windows/amd64, mingw-w64)"
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        CC=x86_64-w64-mingw32-gcc \
        CXX=x86_64-w64-mingw32-g++ \
        go build -trimpath -ldflags="-s -w -H windowsgui" \
        -o "$EXPORT_DIR/patcher.exe" .
    echo "    ✓ $(du -h "$EXPORT_DIR/patcher.exe" | cut -f1)  $EXPORT_DIR/patcher.exe"
fi

echo
echo "✓ done → $EXPORT_DIR"
ls -lh "$EXPORT_DIR"
