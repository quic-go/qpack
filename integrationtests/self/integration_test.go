package self

import (
	"bytes"

	"github.com/marten-seemann/qpack"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Self Tests", func() {
	var (
		// for the encoder
		output  *bytes.Buffer
		encoder *qpack.Encoder
		// for the decoder
		headerFields []qpack.HeaderField
		decoder      *qpack.Decoder
	)

	BeforeEach(func() {
		output = &bytes.Buffer{}
		encoder = qpack.NewEncoder(output)
		headerFields = nil
		decoder = qpack.NewDecoder(func(hf qpack.HeaderField) {
			headerFields = append(headerFields, hf)
		})
	})

	It("encodes and decodes a single header", func() {
		hf := qpack.HeaderField{Name: "foo", Value: "bar"}
		Expect(encoder.WriteField(hf)).To(Succeed())
		_, err := decoder.Write(output.Bytes())
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal([]qpack.HeaderField{hf}))
	})
})
