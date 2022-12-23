package self

import (
	"math/rand"
	"testing"
	_ "unsafe"

	"github.com/quic-go/qpack"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSelf(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Self Suite")
}

var _ = BeforeSuite(func() {
	rand.Seed(GinkgoRandomSeed())
})

var staticTable []qpack.HeaderField

//go:linkname getStaticTable github.com/quic-go/qpack.getStaticTable
func getStaticTable() []qpack.HeaderField

func init() {
	staticTable = getStaticTable()
}
