package memdb

import (
	"testing"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/syndtr/goleveldb/leveldb/testutil"
)

func TestMemDB(t *testing.T) {
	testutil.RunSuite(t, "MemDB Suite")
}
