package lumber

import (
	"io"

	"github.com/kaminocorp/lumber/internal/download"
)

// Thin wrappers around internal/download — keeps the public API unchanged
// while sharing download logic with cmd/lumber (the CLI wizard).

func downloadModels(destDir string) error {
	return download.DownloadModels(destDir)
}

func downloadORT(destDir string) error {
	return download.DownloadORT(destDir)
}

func ortPlatform() (archiveSuffix, libName string, err error) {
	return download.OrtPlatform()
}

func fileValid(path, expectedSHA256 string) bool {
	return download.FileValid(path, expectedSHA256)
}

func downloadFile(url, dest, expectedSHA256 string) error {
	return download.DownloadFile(url, dest, expectedSHA256)
}

func atomicWriteFromReader(dest string, r io.Reader) error {
	return download.AtomicWriteFromReader(dest, r)
}
