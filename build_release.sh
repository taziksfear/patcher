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
#   mingw-w64 or zig                     (Windows cross-compile)
#
# Usage:
#   ./build_release.sh              # build both
#   ./build_release.sh linux        # Linux only
#   ./build_release.sh windows      # Windows only

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DLLS_DIR="$ROOT/dlls"
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

# ── 1. build the injectable runtime from source ──────────────────────────────
echo "── [1/4] dotnet build (dlls → _patcher.dll)"
dotnet build "$DLLS_DIR/Patcher.csproj" \
    -c Release \
    --nologo --verbosity quiet

BUILT_DLL="$DLLS_DIR/bin/Release/_patcher.dll"
if [[ ! -f "$BUILT_DLL" ]]; then
    echo "[!] dotnet build did not produce $BUILT_DLL" >&2
    exit 1
fi
cp "$BUILT_DLL" "$PATCHER_DIR/_patcher.dll"

# ── 2. dotnet publish the C# patcher (always — both targets embed it) ────────
echo "── [2/4] dotnet publish (osu_patcher.exe, win-x86, self-contained)"
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
echo "    → $(du -h "$PUBLISHED_EXE" | cut -f1)  $PUBLISHED_EXE"

echo "── [3/4] copy embed target → $EMBED_TARGET"
cp "$PUBLISHED_EXE" "$EMBED_TARGET"

cd "$GO_DIR"

# ── 3a. Linux build ──────────────────────────────────────────────────────────
if [[ "$WANT_LINUX" == "1" ]]; then
    echo "── [4/4] go build (linux/amd64, webkit2_4_1)"
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
        go build -tags webkit2_4_1 -trimpath -ldflags="-s -w" \
        -o "$EXPORT_DIR/patcher" .
    echo "    ✓ $(du -h "$EXPORT_DIR/patcher" | cut -f1)  $EXPORT_DIR/patcher"
fi

# ── 3b. Windows cross-compile ────────────────────────────────────────────────
if [[ "$WANT_WINDOWS" == "1" ]]; then
    # Fyne needs cgo, so a Windows C toolchain is required. mingw-w64 is the
    # usual one; zig also works and needs no root, since it ships the mingw
    # headers and CRT itself.
    if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
        WIN_CC=x86_64-w64-mingw32-gcc
        WIN_CXX=x86_64-w64-mingw32-g++
    else
        # A bare `zig` on PATH may be a version-manager shim that refuses to run
        # until a version is selected, so only accept one that actually works.
        ZIG_BIN="$(command -v zig 2>/dev/null || true)"
        if [[ -n "$ZIG_BIN" ]] && ! "$ZIG_BIN" version >/dev/null 2>&1; then
            ZIG_BIN=""
        fi
        if [[ -z "$ZIG_BIN" ]] && command -v mise >/dev/null 2>&1; then
            ZIG_BIN="$(mise exec zig -- sh -c 'command -v zig' 2>/dev/null | tail -1 || true)"
            if [[ -n "$ZIG_BIN" ]] && ! "$ZIG_BIN" version >/dev/null 2>&1; then
                ZIG_BIN=""
            fi
        fi
        if [[ -z "$ZIG_BIN" ]]; then
            echo "[!] no Windows C toolchain found — install one of:" >&2
            echo "    sudo pacman -S mingw-w64-gcc   (Arch)" >&2
            echo "    sudo apt install mingw-w64     (Debian/Ubuntu)" >&2
            echo "    mise install zig               (no root needed)" >&2
            exit 1
        fi
        # cgo splits CC on spaces, so the target flag has to live in a wrapper.
        ZIG_WRAP="$(mktemp -d)"
        trap 'rm -rf "$ZIG_WRAP"' EXIT
        printf '#!/bin/sh\nexec %s cc -target x86_64-windows-gnu "$@"\n' "$ZIG_BIN" > "$ZIG_WRAP/cc"
        printf '#!/bin/sh\nexec %s c++ -target x86_64-windows-gnu "$@"\n' "$ZIG_BIN" > "$ZIG_WRAP/cxx"
        chmod +x "$ZIG_WRAP/cc" "$ZIG_WRAP/cxx"
        WIN_CC="$ZIG_WRAP/cc"
        WIN_CXX="$ZIG_WRAP/cxx"
    fi

    echo "── [4/4] go build (windows/amd64, CC=$(basename "$WIN_CC"))"
    # -H windowsgui alone only reaches the internal linker; cgo links externally,
    # so the subsystem has to be handed to the linker too or the app launches
    # with a console window attached.
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        CC="$WIN_CC" \
        CXX="$WIN_CXX" \
        go build -trimpath -ldflags="-s -w -H windowsgui -extldflags=-Wl,--subsystem,windows" \
        -o "$EXPORT_DIR/patcher.exe" .
    echo "    ✓ $(du -h "$EXPORT_DIR/patcher.exe" | cut -f1)  $EXPORT_DIR/patcher.exe"
fi

echo
echo "✓ done → $EXPORT_DIR"
ls -lh "$EXPORT_DIR"
