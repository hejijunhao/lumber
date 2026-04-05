package lumber

import "github.com/kaminocorp/lumber/internal/download"

func defaultCacheDir() (string, error) {
	return download.DefaultCacheDir()
}
