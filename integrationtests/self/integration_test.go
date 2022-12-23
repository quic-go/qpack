package self

import (
	"bytes"
	"fmt"
	"math/rand"

	"github.com/quic-go/qpack"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Self Tests", func() {
	getEncoder := func() (*qpack.Encoder, *bytes.Buffer) {
		output := &bytes.Buffer{}
		return qpack.NewEncoder(output), output
	}

	randomString := func(l int) string {
		const charset = "abcdefghijklmnopqrstuvwxyz" +
			"ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		s := make([]byte, l)
		for i := range s {
			s[i] = charset[rand.Intn(len(charset))]
		}
		return string(s)
	}

	It("encodes and decodes a single header field", func() {
		hf := qpack.HeaderField{
			Name:  randomString(15),
			Value: randomString(15),
		}
		encoder, output := getEncoder()
		Expect(encoder.WriteField(hf)).To(Succeed())
		headerFields, err := qpack.NewDecoder(nil).DecodeFull(output.Bytes())
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal([]qpack.HeaderField{hf}))
	})

	It("encodes and decodes multiple header fields", func() {
		hfs := []qpack.HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "lorem", Value: "ipsum"},
			{Name: randomString(15), Value: randomString(20)},
		}
		encoder, output := getEncoder()
		for _, hf := range hfs {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}
		headerFields, err := qpack.NewDecoder(nil).DecodeFull(output.Bytes())
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal(hfs))
	})

	It("encodes and decodes multiple requests", func() {
		hfs1 := []qpack.HeaderField{{Name: "foo", Value: "bar"}}
		hfs2 := []qpack.HeaderField{
			{Name: "lorem", Value: "ipsum"},
			{Name: randomString(15), Value: randomString(20)},
		}
		encoder, output := getEncoder()
		for _, hf := range hfs1 {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}
		req1 := append([]byte{}, output.Bytes()...)
		output.Reset()
		for _, hf := range hfs2 {
			Expect(encoder.WriteField(hf)).To(Succeed())
		}
		req2 := append([]byte{}, output.Bytes()...)

		var headerFields []qpack.HeaderField
		decoder := qpack.NewDecoder(func(hf qpack.HeaderField) { headerFields = append(headerFields, hf) })
		_, err := decoder.Write(req1)
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal(hfs1))
		headerFields = nil
		_, err = decoder.Write(req2)
		Expect(err).ToNot(HaveOccurred())
		Expect(headerFields).To(Equal(hfs2))
	})

	// replace one character by a random character at a random position
	replaceRandomCharacter := func(s string) string {
		pos := rand.Intn(len(s))
		new := s[:pos]
		for {
			if c := randomString(1); c != string(s[pos]) {
				new += c
				break
			}
		}
		new += s[pos+1:]
		return new
	}

	check := func(encoded []byte, hf qpack.HeaderField) {
		headerFields, err := qpack.NewDecoder(nil).DecodeFull(encoded)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, headerFields).To(HaveLen(1))
		ExpectWithOffset(1, headerFields[0]).To(Equal(hf))
	}

	// use an entry with a value, for example "set-cookie"
	It("uses the static table for field names, for fields without values", func() {
		var hf qpack.HeaderField
		for {
			if entry := staticTable[rand.Intn(len(staticTable))]; len(entry.Value) == 0 {
				hf = qpack.HeaderField{Name: entry.Name}
				break
			}
		}
		encoder, output := getEncoder()
		Expect(encoder.WriteField(hf)).To(Succeed())
		encodedLen := output.Len()
		check(output.Bytes(), hf)
		encoder, output = getEncoder()
		oldName := hf.Name
		hf.Name = replaceRandomCharacter(hf.Name)
		Expect(encoder.WriteField(hf)).To(Succeed())
		fmt.Fprintf(GinkgoWriter, "Encoding field name:\n\t%s: %d bytes\n\t%s: %d bytes\n", oldName, encodedLen, hf.Name, output.Len())
		Expect(output.Len()).To(BeNumerically(">", encodedLen))
	})

	// use an entry with a value, for example "set-cookie",
	// but now use a custom value
	It("uses the static table for field names, for fields without values", func() {
		var hf qpack.HeaderField
		for {
			if entry := staticTable[rand.Intn(len(staticTable))]; len(entry.Value) == 0 {
				hf = qpack.HeaderField{
					Name:  entry.Name,
					Value: randomString(5),
				}
				break
			}
		}
		encoder, output := getEncoder()
		Expect(encoder.WriteField(hf)).To(Succeed())
		encodedLen := output.Len()
		check(output.Bytes(), hf)
		encoder, output = getEncoder()
		oldName := hf.Name
		hf.Name = replaceRandomCharacter(hf.Name)
		Expect(encoder.WriteField(hf)).To(Succeed())
		fmt.Fprintf(GinkgoWriter, "Encoding field name:\n\t%s: %d bytes\n\t%s: %d bytes\n", oldName, encodedLen, hf.Name, output.Len())
		Expect(output.Len()).To(BeNumerically(">", encodedLen))
	})

	// use an entry with a value, for example
	//   cache-control -> Value: "max-age=0"
	// but encode a different value
	//   cache-control -> xyz
	It("uses the static table for field names, for fields with values", func() {
		var hf qpack.HeaderField
		for {
			// Only use values with at least 2 characters.
			// This makes sure that Huffman enocding doesn't compress them as much as encoding it using the static table would.
			if entry := staticTable[rand.Intn(len(staticTable))]; len(entry.Value) > 1 {
				hf = qpack.HeaderField{
					Name:  entry.Name,
					Value: randomString(20),
				}
				break
			}
		}
		encoder, output := getEncoder()
		Expect(encoder.WriteField(hf)).To(Succeed())
		encodedLen := output.Len()
		check(output.Bytes(), hf)
		encoder, output = getEncoder()
		oldName := hf.Name
		hf.Name = replaceRandomCharacter(hf.Name)
		Expect(encoder.WriteField(hf)).To(Succeed())
		fmt.Fprintf(GinkgoWriter, "Encoding field name:\n\t%s: %d bytes\n\t%s: %d bytes\n", oldName, encodedLen, hf.Name, output.Len())
		Expect(output.Len()).To(BeNumerically(">", encodedLen))
	})

	It("uses the static table for field values", func() {
		var hf qpack.HeaderField
		for {
			// Only use values with at least 2 characters.
			// This makes sure that Huffman enocding doesn't compress them as much as encoding it using the static table would.
			if entry := staticTable[rand.Intn(len(staticTable))]; len(entry.Value) > 1 {
				hf = qpack.HeaderField{
					Name:  entry.Name,
					Value: entry.Value,
				}
				break
			}
		}
		encoder, output := getEncoder()
		Expect(encoder.WriteField(hf)).To(Succeed())
		encodedLen := output.Len()
		check(output.Bytes(), hf)
		encoder, output = getEncoder()
		oldValue := hf.Value
		hf.Value = replaceRandomCharacter(hf.Value)
		Expect(encoder.WriteField(hf)).To(Succeed())
		fmt.Fprintf(GinkgoWriter,
			"Encoding field value:\n\t%s: %s -> %d bytes\n\t%s: %s -> %d bytes\n",
			hf.Name, oldValue, encodedLen,
			hf.Name, hf.Value, output.Len(),
		)
		Expect(output.Len()).To(BeNumerically(">", encodedLen))
	})
})
