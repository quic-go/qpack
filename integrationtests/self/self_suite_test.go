package self

import (
	"math/rand"
	"testing"
	_ "unsafe"

	"github.com/marten-seemann/qpack"

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

//go:linkname getStaticTable github.com/marten-seemann/qpack.getStaticTable
func getStaticTable() []qpack.HeaderField

func init() {
	staticTable = getStaticTable()
}
