package main

import (
	_ "embed"
)

//go:embed osu_patcher.exe
var embeddedPatcherExe []byte

// minEmbeddedBytes is the size below which we treat the embed as a placeholder
// (the build script writes the real dotnet-published exe, which is in the MB
// range). Anything smaller means the user ran a plain `go build` without the
// build.sh step that copies the real exe in.
const minEmbeddedBytes = 64 * 1024

func hasEmbeddedPatcher() bool {
	return len(embeddedPatcherExe) >= minEmbeddedBytes
}
