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

	It("encodes and decodes a single header field", func() {
		hf := qpack.HeaderField{Name: "foo", Value: "bar"}
		Expect(encoder.WriteField(hf)).To(Succeed())
		_, err := decoder.Write(output.Bytes())
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal([]qpack.HeaderField{hf}))
	})

	It("encodes and decodes multiple header fields", func() {
		hfs := []qpack.HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "lorem", Value: "ipsum"},
			{Name: "name", Value: "value"},
		}
		for _, hf := range hfs {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}
		_, err := decoder.Write(output.Bytes())
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal(hfs))
	})

	It("encodes and decodes multiple requests", func() {
		hfs1 := []qpack.HeaderField{{Name: "foo", Value: "bar"}}
		hfs2 := []qpack.HeaderField{
			{Name: "lorem", Value: "ipsum"},
			{Name: "name", Value: "value"},
		}
		for _, hf := range hfs1 {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}
		req1 := append([]byte{}, output.Bytes()...)
		output.Reset()
		for _, hf := range hfs2 {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}
		req2 := append([]byte{}, output.Bytes()...)

		_, err := decoder.Write(req1)
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal(hfs1))
		headerFields = nil
		_, err = decoder.Write(req2)
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal(hfs2))
	})
})
