package qpack

import (
	"io"
	"testing"

	"golang.org/x/net/http2/hpack"

	"github.com/stretchr/testify/require"
)

func insertPrefix(data []byte) []byte {
	prefix := appendVarInt(nil, 8, 0)
	prefix = appendVarInt(prefix, 7, 0)
	return append(prefix, data...)
}

func TestDecoderInvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "non-zero required insert count", // we don't support dynamic table updates
			input:    append(appendVarInt(nil, 8, 1), appendVarInt(nil, 7, 0)...),
			expected: "expected Required Insert Count to be zero",
		},
		{
			name:     "non-zero delta base", // we don't support dynamic table updates
			input:    append(appendVarInt(nil, 8, 0), appendVarInt(nil, 7, 1)...),
			expected: "expected Base to be zero",
		},
		{
			name:     "unknown type byte",
			input:    insertPrefix([]byte{0x10}),
			expected: "unexpected type byte: 0x10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := NewDecoder()
			decode := dec.Decode(tt.input)
			_, err := decode()
			require.EqualError(t, err, tt.expected)
		})
	}
}

const (
	loremIpsum1 = "lorem ipsum dolor sit amet"
	loremIpsum2 = "consectetur adipiscing elit"
)

type testcase struct {
	Data     []byte
	Expected []HeaderField
}

var (
	literalFieldWithoutNameReference = testcase{
		Data: func() []byte {
			data := appendVarInt(nil, 3, 3)
			data[0] ^= 0x20
			data = append(data, []byte("foo")...)
			data = appendVarInt(data, 7, uint64(len(loremIpsum1)))
			data = append(data, []byte(loremIpsum1)...)
			data2 := appendVarInt(nil, 3, 3)
			data2[0] ^= 0x20
			data2 = append(data2, []byte("bar")...)
			data2 = appendVarInt(data2, 7, uint64(len(loremIpsum2)))
			data2 = append(data2, []byte(loremIpsum2)...)
			return insertPrefix(append(data, data2...))
		}(),
		Expected: []HeaderField{
			{Name: "foo", Value: loremIpsum1},
			{Name: "bar", Value: loremIpsum2},
		},
	}
	literalFieldWithNameReference = testcase{
		Data: func() []byte {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 | 0x10
			data = appendVarInt(data, 7, uint64(len(loremIpsum1)))
			data = append(data, []byte(loremIpsum1)...)
			data2 := appendVarInt(nil, 4, 82)
			data2[0] ^= 0x40 | 0x10
			data2[0] |= 0x20 // set the N-bit
			data2 = appendVarInt(data2, 7, uint64(len(loremIpsum2)))
			data2 = append(data2, []byte(loremIpsum2)...)
			return insertPrefix(append(data, data2...))
		}(),
		Expected: []HeaderField{
			{Name: "content-type", Value: loremIpsum1},
			{Name: "access-control-request-method", Value: loremIpsum2},
		},
	}
	literalFieldWithHuffmanEncoding = testcase{
		Data: func() []byte {
			data := appendVarInt(nil, 4, 49)
			data[0] ^= 0x40 | 0x10
			data2 := appendVarInt(nil, 7, hpack.HuffmanEncodeLength(loremIpsum1))
			data2[0] ^= 0x80
			data = hpack.AppendHuffmanString(append(data, data2...), loremIpsum1)
			data3 := appendVarInt(nil, 4, 82)
			data3[0] ^= 0x40 | 0x10
			data4 := appendVarInt(nil, 7, hpack.HuffmanEncodeLength(loremIpsum2))
			data4[0] ^= 0x80
			data5 := hpack.AppendHuffmanString(append(data3, data4...), loremIpsum2)
			return insertPrefix(append(data, data5...))
		}(),
		Expected: []HeaderField{
			{Name: "content-type", Value: loremIpsum1},
			{Name: "access-control-request-method", Value: loremIpsum2},
		},
	}
	indexedField = testcase{
		Data: func() []byte {
			data := appendVarInt(nil, 6, 20)
			data[0] ^= 0x80 | 0x40
			data2 := appendVarInt(nil, 6, 42)
			data2[0] ^= 0x80 | 0x40
			return insertPrefix(append(data, data2...))
		}(),
		Expected: []HeaderField{
			staticTableEntries[20],
			staticTableEntries[42],
		},
	}
)

func TestDecoderLiteralHeaderFieldDynamicTable(t *testing.T) {
	data := appendVarInt(nil, 4, 49)
	data[0] ^= 0x40 // don't set the static flag (0x10)
	data = appendVarInt(data, 7, 6)
	data = append(data, []byte("foobar")...)
	dec := NewDecoder()
	decode := dec.Decode(insertPrefix(data))
	_, err := decode()
	require.ErrorIs(t, err, errNoDynamicTable)
}

func decodeAll(t *testing.T, decode func() (HeaderField, error)) []HeaderField {
	t.Helper()
	var hfs []HeaderField
	for {
		hf, err := decode()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		hfs = append(hfs, hf)
	}
	return hfs
}

func TestDecoderIndexedHeaderFields(t *testing.T) {
	dec := NewDecoder()
	decodeFn := dec.Decode(indexedField.Data)
	require.Equal(t, indexedField.Expected, decodeAll(t, decodeFn))
}

func TestDecoderInvalidIndexedHeaderFields(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name: "errors when a non-existent static table entry is referenced",
			input: func() []byte {
				data := appendVarInt(nil, 6, 10000)
				data[0] ^= 0x80 | 0x40
				return insertPrefix(data)
			}(),
			expected: "invalid indexed representation index 10000",
		},
		{
			name: "rejects an indexed header field that references the dynamic table",
			input: func() []byte {
				data := appendVarInt(nil, 6, 20)
				data[0] ^= 0x80 // don't set the static flag (0x40)
				return insertPrefix(data)
			}(),
			expected: errNoDynamicTable.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := NewDecoder()
			decodeFn := dec.Decode(tt.input)
			_, err := decodeFn()
			require.EqualError(t, err, tt.expected)
		})
	}
}

func TestDecoderLiteralHeaderFieldWithNameReferenceAndHuffmanEncoding(t *testing.T) {
	dec := NewDecoder()
	decodeFn := dec.Decode(literalFieldWithHuffmanEncoding.Data)
	require.Equal(t, literalFieldWithHuffmanEncoding.Expected, decodeAll(t, decodeFn))
}

func TestDecoderLiteralHeaderFieldWithoutNameReference(t *testing.T) {
	dec := NewDecoder()
	decodeFn := dec.Decode(literalFieldWithoutNameReference.Data)
	require.Equal(t, literalFieldWithoutNameReference.Expected, decodeAll(t, decodeFn))
}

func TestDecoderEOF(t *testing.T) {
	t.Run("literal field without name reference", func(t *testing.T) {
		testDecoderEOF(t,
			literalFieldWithoutNameReference.Data,
			len(literalFieldWithoutNameReference.Expected),
		)
	})

	t.Run("literal field with name reference", func(t *testing.T) {
		testDecoderEOF(t,
			literalFieldWithNameReference.Data,
			len(literalFieldWithNameReference.Expected),
		)
	})

	t.Run("literal field with Huffman encoding", func(t *testing.T) {
		testDecoderEOF(t,
			literalFieldWithHuffmanEncoding.Data,
			len(literalFieldWithHuffmanEncoding.Expected),
		)
	})

	t.Run("indexed field", func(t *testing.T) {
		testDecoderEOF(t,
			indexedField.Data,
			len(indexedField.Expected),
		)
	})
}

func testDecoderEOF(t *testing.T, data []byte, numExpected int) {
	for i := range data {
		dec := NewDecoder()
		decodeFn := dec.Decode(data[:i])
		var hfs []HeaderField
		for {
			hf, err := decodeFn()
			// the data might have been cut right after a header field,
			// which is a valid header
			if err == io.EOF {
				require.Less(t, len(hfs), numExpected)
				break
			}
			if err != nil {
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
				break
			}
			hfs = append(hfs, hf)
		}
	}
}

func BenchmarkDecoder(b *testing.B) {
	b.Run("literal field without name reference", func(b *testing.B) {
		benchmarkDecoder(b,
			literalFieldWithoutNameReference.Data,
			len(literalFieldWithoutNameReference.Expected),
		)
	})

	b.Run("literal field with name reference", func(b *testing.B) {
		benchmarkDecoder(b,
			literalFieldWithNameReference.Data,
			len(literalFieldWithNameReference.Expected),
		)
	})

	b.Run("literal field with Huffman encoding", func(b *testing.B) {
		benchmarkDecoder(b,
			literalFieldWithHuffmanEncoding.Data,
			len(literalFieldWithHuffmanEncoding.Expected),
		)
	})

	b.Run("indexed field", func(b *testing.B) {
		benchmarkDecoder(b,
			indexedField.Data,
			len(indexedField.Expected),
		)
	})
}

func benchmarkDecoder(b *testing.B, data []byte, numExpected int) {
	b.ReportAllocs()

	decoder := NewDecoder()
	hdr := make(map[string]string)
	for b.Loop() {
		decodeFn := decoder.Decode(data)
		for {
			hf, err := decodeFn()
			if err != nil {
				if err == io.EOF {
					break
				}
				b.Fatalf("unexpected error: %v", err)
			}
			// simulate what a typical HTTP/3 consumer would do with the header fields:
			// populate an http.Header with the header fields
			hdr[hf.Name] = hf.Value
		}
		if len(hdr) != numExpected {
			b.Fatalf("expected %d header fields, got %d", numExpected, len(hdr))
		}
		clear(hdr)
	}
}
