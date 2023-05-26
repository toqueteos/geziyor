package diskcache

import (
	"github.com/toqueteos/geziyor/cache"
	"io/ioutil"
	"os"
	"testing"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "cache")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cache.PleaseCache(t, New(tempDir))
}
