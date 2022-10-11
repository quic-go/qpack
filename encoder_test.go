package qpack

import (
	"bytes"
	"io"

	"golang.org/x/net/http2/hpack"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// errWriter wraps bytes.Buffer and optionally fails on every write
// useful for testing misbehaving writers
type errWriter struct {
	bytes.Buffer
	fail bool
}

func (ew *errWriter) Write(b []byte) (int, error) {
	if ew.fail {
		return 0, io.ErrClosedPipe
	}
	return ew.Buffer.Write(b)
}

var _ = Describe("Encoder", func() {
	var (
		encoder *Encoder
		output  *errWriter
	)

	BeforeEach(func() {
		output = &errWriter{}
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
		Expect(data[0] & 0x8).ToNot(BeZero())                         // Huffman encoding
		nameLen, data, err := readVarInt(3, data)
		Expect(err).ToNot(HaveOccurred())
		l := hpack.HuffmanEncodeLength(hf.Name)
		Expect(nameLen).To(BeEquivalentTo(l))
		Expect(hpack.HuffmanDecodeToString(data[:l])).To(Equal(hf.Name))
		valueLen, data, err := readVarInt(7, data[l:])
		Expect(err).ToNot(HaveOccurred())
		l = hpack.HuffmanEncodeLength(hf.Value)
		Expect(valueLen).To(BeEquivalentTo(l))
		Expect(hpack.HuffmanDecodeToString(data[:l])).To(Equal(hf.Value))
		return data[l:]
	}

	// Reads one indexed field line representation from data and verifies it matches hf.
	// Returns the leftover bytes from data.
	checkIndexedHeaderField := func(data []byte, hf HeaderField) []byte {
		Expect(data[0] >> 7).To(Equal(uint8(1))) // 1Txxxxxx
		index, data, err := readVarInt(6, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(staticTableEntries[index]).To(Equal(hf))
		return data
	}

	checkHeaderFieldWithNameRef := func(data []byte, hf HeaderField) []byte {
		// read name reference
		Expect(data[0] >> 6).To(Equal(uint8(1))) // 01NTxxxx
		index, data, err := readVarInt(4, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(staticTableEntries[index].Name).To(Equal(hf.Name))
		// read literal value
		valueLen, data, err := readVarInt(7, data)
		Expect(err).ToNot(HaveOccurred())
		l := hpack.HuffmanEncodeLength(hf.Value)
		Expect(valueLen).To(BeEquivalentTo(l))
		Expect(hpack.HuffmanDecodeToString(data[:l])).To(Equal(hf.Value))
		return data[l:]
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

	It("encodes fails to encode when writer errs", func() {
		hf := HeaderField{Name: "foobar", Value: "lorem ipsum"}
		output.fail = true
		Expect(encoder.WriteField(hf)).To(MatchError("io: read/write on closed pipe"))
	})

	It("encodes multiple fields", func() {
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

	It("encodes all the fields of the static table", func() {
		for _, hf := range staticTableEntries {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}

		data, requiredInsertCount, deltaBase := readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())

		for _, hf := range staticTableEntries {
			data = checkIndexedHeaderField(data, hf)
		}
		Expect(data).To(BeEmpty())
	})

	It("encodes fields with name reference in the static table", func() {
		hf1 := HeaderField{Name: ":status", Value: "666"}
		hf2 := HeaderField{Name: "server", Value: "lorem ipsum"}
		hf3 := HeaderField{Name: ":method", Value: ""}
		Expect(encoder.WriteField(hf1)).To(Succeed())
		Expect(encoder.WriteField(hf2)).To(Succeed())
		Expect(encoder.WriteField(hf3)).To(Succeed())

		data, requiredInsertCount, deltaBase := readPrefix(output.Bytes())
		Expect(requiredInsertCount).To(BeZero())
		Expect(deltaBase).To(BeZero())

		data = checkHeaderFieldWithNameRef(data, hf1)
		data = checkHeaderFieldWithNameRef(data, hf2)
		data = checkHeaderFieldWithNameRef(data, hf3)
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
