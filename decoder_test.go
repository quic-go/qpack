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
			name: "invalid static table index",
			input: func() []byte {
				data := appendVarInt(nil, 6, 10000)
				data[0] ^= 0x80 | 0x40
				return insertPrefix(data)
			}(),
			expected: "invalid indexed representation index 10000",
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
	// When referencing a dynamic table entry that doesn't exist,
	// we should get an invalid index error
	data := appendVarInt(nil, 4, 49)
	data[0] ^= 0x40 // don't set the static flag (0x10)
	data = appendVarInt(data, 7, 6)
	data = append(data, []byte("foobar")...)
	dec := NewDecoder()
	decode := dec.Decode(insertPrefix(data))
	_, err := decode()
	// With dynamic table support, we get invalid index since the table is empty
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid indexed representation index")
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
			name: "errors when a non-existent dynamic table entry is referenced",
			input: func() []byte {
				data := appendVarInt(nil, 6, 20)
				data[0] ^= 0x80 // don't set the static flag (0x40)
				return insertPrefix(data)
			}(),
			expected: "invalid indexed representation index 20",
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

// Tests for dynamic table support

func TestDynamicTableBasicOperations(t *testing.T) {
	dt := newDynamicTable(1024)

	// Insert an entry
	dt.insert(HeaderField{Name: "foo", Value: "bar"})
	require.Equal(t, uint64(1), dt.insertCount)

	// Retrieve by relative index
	hf, ok := dt.atRelative(0)
	require.True(t, ok)
	require.Equal(t, "foo", hf.Name)
	require.Equal(t, "bar", hf.Value)

	// Retrieve by absolute index
	hf, ok = dt.atAbsolute(0)
	require.True(t, ok)
	require.Equal(t, "foo", hf.Name)

	// Insert another entry
	dt.insert(HeaderField{Name: "baz", Value: "qux"})
	require.Equal(t, uint64(2), dt.insertCount)

	// New entry is at relative 0, old is at relative 1
	hf, ok = dt.atRelative(0)
	require.True(t, ok)
	require.Equal(t, "baz", hf.Name)

	hf, ok = dt.atRelative(1)
	require.True(t, ok)
	require.Equal(t, "foo", hf.Name)
}

func TestDynamicTableCapacity(t *testing.T) {
	// Small capacity that can only hold one entry
	dt := newDynamicTable(64)

	// First entry
	dt.insert(HeaderField{Name: "a", Value: "b"})
	require.Equal(t, 1, len(dt.entries))

	// Second entry should evict first
	dt.insert(HeaderField{Name: "c", Value: "d"})
	require.Equal(t, 1, len(dt.entries))

	hf, ok := dt.atRelative(0)
	require.True(t, ok)
	require.Equal(t, "c", hf.Name)
}

func TestProcessEncoderInstructions(t *testing.T) {
	dec := NewDecoderWithCapacity(4096)

	// Build an "Insert Without Name Reference" instruction
	// 01 N xxxxx - name length, then name, then value length, then value
	instruction := appendVarInt(nil, 5, 3) // name length 3
	instruction[0] |= 0x40                 // set the instruction type bit
	instruction = append(instruction, []byte("foo")...)
	instruction = appendVarInt(instruction, 7, 3) // value length 3
	instruction = append(instruction, []byte("bar")...)

	err := dec.ProcessEncoderInstructions(instruction)
	require.NoError(t, err)

	// Check that the entry was added
	require.Equal(t, uint64(1), dec.InsertCount())
}

func TestProcessEncoderInstructionsWithStaticNameRef(t *testing.T) {
	dec := NewDecoderWithCapacity(4096)

	// Build an "Insert With Name Reference (static)" instruction
	// 11 xxxxxx - static table index, then value length, then value
	// Use index 1 which is ":path"
	instruction := appendVarInt(nil, 6, 1) // index 1
	instruction[0] |= 0xc0                 // set bits for static name ref
	instruction = appendVarInt(instruction, 7, 4)
	instruction = append(instruction, []byte("/foo")...)

	err := dec.ProcessEncoderInstructions(instruction)
	require.NoError(t, err)

	require.Equal(t, uint64(1), dec.InsertCount())
}

func TestSetDynamicTableCapacity(t *testing.T) {
	dec := NewDecoderWithCapacity(4096)

	// Build a "Set Dynamic Table Capacity" instruction
	// 001 xxxxx - capacity
	instruction := appendVarInt(nil, 5, 1024)
	instruction[0] |= 0x20

	err := dec.ProcessEncoderInstructions(instruction)
	require.NoError(t, err)

	// Trying to set capacity above max should fail
	instruction = appendVarInt(nil, 5, 8192)
	instruction[0] |= 0x20
	err = dec.ProcessEncoderInstructions(instruction)
	require.ErrorIs(t, err, errTableCapacityLimit)
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
