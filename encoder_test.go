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

	checkHeaderField := func(data []byte, hf HeaderField) []byte {
		Expect(data[0] & (0x80 ^ 0x40 ^ 0x20)).To(Equal(uint8(0x20))) // 001xxxxx
		Expect(data[0] & 0x8).To(BeZero())                            // no Huffman encoding
		nameLen, data, err := readVarInt(3, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(nameLen).To(BeEquivalentTo(len(hf.Name)))
		Expect(string(data[:len(hf.Name)])).To(Equal(hf.Name))
		valueLen, data, err := readVarInt(7, data[len(hf.Name):])
		Expect(err).ToNot(HaveOccurred())
		Expect(valueLen).To(BeEquivalentTo(len(hf.Value)))
		Expect(string(data[:len(hf.Value)])).To(Equal(hf.Value))
		return data[len(hf.Value):]
	}

	It("encodes a single field", func() {
		hf := HeaderField{Name: "foobar", Value: "lorem ipsum"}
		Expect(encoder.WriteField(hf)).To(Succeed())

		data, requiredInsertCount, deltaBase := readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())

		data = checkHeaderField(data, hf)
		Expect(data).To(BeEmpty())
	})

	It("encodes multipe fields", func() {
		hf1 := HeaderField{Name: "foobar", Value: "lorem ipsum"}
		hf2 := HeaderField{Name: "raboof", Value: "dolor sit amet"}
		Expect(encoder.WriteField(hf1)).To(Succeed())
		Expect(encoder.WriteField(hf2)).To(Succeed())

		data, requiredInsertCount, deltaBase := readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())

		data = checkHeaderField(data, hf1)
		data = checkHeaderField(data, hf2)
		Expect(data).To(BeEmpty())
	})

	It("encodes multiple requests", func() {
		hf1 := HeaderField{Name: "foobar", Value: "lorem ipsum"}
		Expect(encoder.WriteField(hf1)).To(Succeed())
		data, requiredInsertCount, deltaBase := readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())
		data = checkHeaderField(data, hf1)
		Expect(data).To(BeEmpty())

		output.Reset()
		Expect(encoder.Close())
		hf2 := HeaderField{Name: "raboof", Value: "dolor sit amet"}
		Expect(encoder.WriteField(hf2)).To(Succeed())
		data, requiredInsertCount, deltaBase = readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())
		data = checkHeaderField(data, hf2)
		Expect(data).To(BeEmpty())
	})
})
