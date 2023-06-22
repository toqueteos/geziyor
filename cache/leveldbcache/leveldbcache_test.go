package leveldbcache

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/toqueteos/geziyor/cache"
)

func TestDiskCache(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "cache")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	c, err := New(filepath.Join(tempDir, "Db"))
	if err != nil {
		t.Fatalf("New leveldb,: %v", err)
	}

	cache.PleaseCache(t, c)
}
