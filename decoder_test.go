package qpack

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/http2/hpack"
)

var _ = Describe("Decoder", func() {
	var (
		decoder      *Decoder
		headerFields []HeaderField
	)

	BeforeEach(func() {
		headerFields = nil
		decoder = NewDecoder(func(hf HeaderField) {
			headerFields = append(headerFields, hf)
		})
	})

	insertPrefix := func(data []byte) []byte {
		prefix := appendVarInt(nil, 8, 0)
		prefix = appendVarInt(prefix, 7, 0)
		return append(prefix, data...)
	}

	doPartialWrites := func(data []byte) {
		for i := 0; i < len(data)-1; i++ {
			n, err := decoder.Write([]byte{data[i]})
			Expect(err).ToNot(HaveOccurred())
			Expect(n).To(Equal(1))
			Expect(headerFields).To(BeEmpty())
		}
		n, err := decoder.Write([]byte{data[len(data)-1]})
		Expect(err).ToNot(HaveOccurred())
		Expect(n).To(Equal(1))
		Expect(headerFields).To(HaveLen(1))
	}

	It("rejects a non-zero Required Insert Count", func() {
		prefix := appendVarInt(nil, 8, 1)
		prefix = appendVarInt(prefix, 7, 0)
		_, err := decoder.Write(prefix)
		Expect(err).To(MatchError("decoding error: expected Required Insert Count to be zero"))
	})

	It("rejects a non-zero Delta Base", func() {
		prefix := appendVarInt(nil, 8, 0)
		prefix = appendVarInt(prefix, 7, 1)
		_, err := decoder.Write(prefix)
		Expect(err).To(MatchError("decoding error: expected Base to be zero"))
	})

	It("rejects unknown type bytes", func() {
		_, err := decoder.Write(insertPrefix([]byte{0x10}))
		Expect(err).To(MatchError("unexpected type byte: 0x10"))
	})

	Context("indexed header field", func() {
		It("parses an indexed header field", func() {
			data := appendVarInt(nil, 6, 20)
			data[0] ^= 0x80 | 0x40
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).ToNot(HaveOccurred())
			Expect(headerFields).To(HaveLen(1))
			Expect(headerFields[0]).To(Equal(staticTableEntries[20]))
		})

		It("rejects an indexed header field that references the dynamic table", func() {
			data := appendVarInt(nil, 6, 20)
			data[0] ^= 0x80 // don't set the static flag (0x40)
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).To(MatchError(errNoDynamicTable))
		})

		It("errors when a non-existant static table entry is referenced", func() {
			data := appendVarInt(nil, 6, 10000)
			data[0] ^= 0x80 | 0x40
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).To(MatchError("decoding error: invalid indexed representation index 10000"))
			Expect(headerFields).To(BeEmpty())
		})

		It("handles partial writes", func() {
			data := appendVarInt(nil, 6, 20)
			data[0] ^= 0x80 | 0x40
			data = insertPrefix(data)

			doPartialWrites(data)
		})
	})

	Context("header field with name reference", func() {
		It("parses a literal header field with name reference", func() {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 | 0x10
			data = appendVarInt(data, 7, 6)
			data = append(data, []byte("foobar")...)
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).ToNot(HaveOccurred())
			Expect(headerFields).To(HaveLen(1))
			Expect(headerFields[0].Name).To(Equal("content-type"))
			Expect(headerFields[0].Value).To(Equal("foobar"))
		})

		It("parses a literal header field with name reference, with Huffman encoding", func() {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 | 0x10
			data2 := appendVarInt(nil, 7, hpack.HuffmanEncodeLength("foobar"))
			data2[0] ^= 0x80
			data = hpack.AppendHuffmanString(append(data, data2...), "foobar")
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).ToNot(HaveOccurred())
			Expect(headerFields).To(HaveLen(1))
			Expect(headerFields[0].Name).To(Equal("content-type"))
			Expect(headerFields[0].Value).To(Equal("foobar"))
		})

		It("rejects a literal header field with name reference that references the dynamic table", func() {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 // don't set the static flag (0x10)
			data = appendVarInt(data, 7, 6)
			data = append(data, []byte("foobar")...)
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).To(MatchError(errNoDynamicTable))
		})

		It("handles partial writes", func() {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 | 0x10
			data = appendVarInt(data, 7, 6)
			data = append(data, []byte("foobar")...)
			data = insertPrefix(data)

			doPartialWrites(data)
		})

		It("handles partial writes, when using Huffman encoding", func() {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 | 0x10
			data2 := appendVarInt(nil, 7, hpack.HuffmanEncodeLength("foobar"))
			data2[0] ^= 0x80
			data = hpack.AppendHuffmanString(append(data, data2...), "foobar")
			data = insertPrefix(data)

			doPartialWrites(data)
		})
	})

	Context("header field without name reference", func() {
		It("parses a literal header field without name reference", func() {
			data := appendVarInt(nil, 3, 3)
			data[0] ^= 0x20
			data = append(data, []byte("foo")...)
			data2 := appendVarInt(nil, 7, 3)
			data2 = append(data2, []byte("bar")...)
			data = append(data, data2...)
			_, err := decoder.Write(insertPrefix(data))
			Expect(err).ToNot(HaveOccurred())
			Expect(headerFields).To(HaveLen(1))
			Expect(headerFields[0].Name).To(Equal("foo"))
			Expect(headerFields[0].Value).To(Equal("bar"))
		})

		It("handles partial writes", func() {
			data := appendVarInt(nil, 3, 3)
			data[0] ^= 0x20
			data = append(data, []byte("foo")...)
			data2 := appendVarInt(nil, 7, 3)
			data2 = append(data2, []byte("bar")...)
			data = append(data, data2...)
			data = insertPrefix(data)

			doPartialWrites(data)
		})
	})

	Context("using DecodeFull", func() {
		It("decodes nothing", func() {
			data, err := NewDecoder(nil).DecodeFull([]byte{})
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(BeEmpty())
		})

		It("decodes multiple entries", func() {
			buf := &bytes.Buffer{}
			enc := NewEncoder(buf)
			Expect(enc.WriteField(HeaderField{Name: "foo", Value: "bar"})).To(Succeed())
			Expect(enc.WriteField(HeaderField{Name: "lorem", Value: "ipsum"})).To(Succeed())
			data, err := NewDecoder(nil).DecodeFull(buf.Bytes())
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(Equal([]HeaderField{
				{Name: "foo", Value: "bar"},
				{Name: "lorem", Value: "ipsum"},
			}))
		})

		It("returns an error if the data is incomplete", func() {
			buf := &bytes.Buffer{}
			enc := NewEncoder(buf)
			Expect(enc.WriteField(HeaderField{Name: "foo", Value: "bar"})).To(Succeed())
			_, err := NewDecoder(nil).DecodeFull(buf.Bytes()[:buf.Len()-2])
			Expect(err).To(MatchError("decoding error: truncated headers"))
		})

		It("restores the emitFunc afterwards", func() {
			var emitFuncCalled bool
			emitFunc := func(HeaderField) {
				emitFuncCalled = true
			}
			decoder := NewDecoder(emitFunc)
			buf := &bytes.Buffer{}
			enc := NewEncoder(buf)
			Expect(enc.WriteField(HeaderField{Name: "foo", Value: "bar"})).To(Succeed())
			_, err := decoder.DecodeFull(buf.Bytes())
			Expect(err).ToNot(HaveOccurred())
			Expect(emitFuncCalled).To(BeFalse())
			_, err = decoder.Write(buf.Bytes())
			Expect(err).ToNot(HaveOccurred())
			Expect(emitFuncCalled).To(BeTrue())
		})
	})
})
