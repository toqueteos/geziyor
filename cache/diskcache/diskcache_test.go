package diskcache

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/toqueteos/geziyor/cache"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "cache")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	cache.PleaseCache(t, New(tempDir))
}
