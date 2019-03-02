package qpack

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Encoder", func() {
	var (
		encoder *Encoder
		output  *bytes.Buffer
	)

	BeforeEach(func() {
		output = &bytes.Buffer{}
		encoder = NewEncoder(output)
	})

	readPrefix := func(data []byte) (rest []byte, requiredInsertCount uint64, deltaBase uint64) {
		var err error
		requiredInsertCount, rest, err = readVarInt(8, data)
		Expect(err).ToNot(HaveOccurred())
		deltaBase, rest, err = readVarInt(7, rest)
		Expect(err).ToNot(HaveOccurred())
		return
	}

	It("encodes", func() {
		hf := HeaderField{Name: "foobar", Value: "lorem ipsum"}
		Expect(encoder.WriteField(hf)).To(Succeed())

		data, requiredInsertCount, deltaBase := readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())

		Expect(data[0] & (0x80 ^ 0x40 ^ 0x20)).To(Equal(uint8(0x20))) // 001xxxxx
		Expect(data[0] & 0x8).To(BeZero())                            // no Huffman encoding
		nameLen, data, err := readVarInt(3, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(nameLen).To(BeEquivalentTo(6))
		Expect(string(data[:6])).To(Equal("foobar"))
		valueLen, data, err := readVarInt(7, data[6:])
		Expect(err).ToNot(HaveOccurred())
		Expect(valueLen).To(BeEquivalentTo(11))
		Expect(string(data[:11])).To(Equal("lorem ipsum"))
		Expect(data[11:]).To(BeEmpty())
	})
})
