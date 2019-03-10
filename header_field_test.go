package qpack

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Header Field", func() {
	It("says if it is pseudo", func() {
		Expect((HeaderField{Name: ":status"}).IsPseudo()).To(BeTrue())
		Expect((HeaderField{Name: ":authority"}).IsPseudo()).To(BeTrue())
		Expect((HeaderField{Name: ":foobar"}).IsPseudo()).To(BeTrue())
		Expect((HeaderField{Name: "status"}).IsPseudo()).To(BeFalse())
		Expect((HeaderField{Name: "foobar"}).IsPseudo()).To(BeFalse())
	})
})
