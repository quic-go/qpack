package qpack

import (
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

	It("rejects a literal header field wit name reference that references the dynamic table", func() {
		data := appendVarInt(nil, 4, 49)
		data[0] ^= 0x40 // don't set the static flag (0x10)
		data = appendVarInt(data, 7, 6)
		data = append(data, []byte("foobar")...)
		_, err := decoder.Write(insertPrefix(data))
		Expect(err).To(MatchError(errNoDynamicTable))
	})

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

	It("rejects unknown type bytes", func() {
		_, err := decoder.Write(insertPrefix([]byte{0x10}))
		Expect(err).To(MatchError("unexpected type byte: 0x10"))
	})
})
