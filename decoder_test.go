package qpack

import (
	"bytes"
	"testing"

	"golang.org/x/net/http2/hpack"

	"github.com/stretchr/testify/require"
)

type recordingDecoder struct {
	*Decoder
	headerFields []HeaderField
}

func newRecordingDecoder() *recordingDecoder {
	decoder := &recordingDecoder{}
	decoder.Decoder = NewDecoder(func(hf HeaderField) { decoder.headerFields = append(decoder.headerFields, hf) })
	return decoder
}

func (decoder *recordingDecoder) Fields() []HeaderField { return decoder.headerFields }

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
			expected: "decoding error: expected Required Insert Count to be zero",
		},
		{
			name:     "non-zero delta base", // we don't support dynamic table updates
			input:    append(appendVarInt(nil, 8, 0), appendVarInt(nil, 7, 1)...),
			expected: "decoding error: expected Base to be zero",
		},
		{
			name:     "unknown type byte",
			input:    insertPrefix([]byte{0x10}),
			expected: "unexpected type byte: 0x10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDecoder(nil).Write(tt.input)
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
	decoder := newRecordingDecoder()
	data := appendVarInt(nil, 4, 49)
	data[0] ^= 0x40 // don't set the static flag (0x10)
	data = appendVarInt(data, 7, 6)
	data = append(data, []byte("foobar")...)
	_, err := decoder.Write(insertPrefix(data))
	require.ErrorIs(t, err, errNoDynamicTable)
}

func doPartialWrites(t *testing.T, decoder *recordingDecoder, data []byte) {
	t.Helper()
	for i := 0; i < len(data)-1; i++ {
		n, err := decoder.Write([]byte{data[i]})
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	n, err := decoder.Write([]byte{data[len(data)-1]})
	require.NoError(t, err)
	require.NotZero(t, n)
}

func TestDecoderIndexedHeaderFields(t *testing.T) {
	decoder := newRecordingDecoder()
	doPartialWrites(t, decoder, indexedField.Data)
	require.Equal(t, indexedField.Expected, decoder.Fields())
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
			expected: "decoding error: invalid indexed representation index 10000",
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
			decoder := newRecordingDecoder()
			_, err := decoder.Write(tt.input)
			require.EqualError(t, err, tt.expected)
			require.Empty(t, decoder.Fields())
		})
	}
}

func TestDecoderLiteralHeaderFieldWithNameReferenceAndHuffmanEncoding(t *testing.T) {
	decoder := newRecordingDecoder()
	doPartialWrites(t, decoder, literalFieldWithHuffmanEncoding.Data)
	require.Equal(t, literalFieldWithHuffmanEncoding.Expected, decoder.Fields())
}

func TestDecoderLiteralHeaderFieldWithoutNameReference(t *testing.T) {
	decoder := newRecordingDecoder()
	doPartialWrites(t, decoder, literalFieldWithoutNameReference.Data)
	require.Equal(t, literalFieldWithoutNameReference.Expected, decoder.Fields())
}

func TestDecodeFull(t *testing.T) {
	// decode nothing
	data, err := NewDecoder(nil).DecodeFull([]byte{})
	require.NoError(t, err)
	require.Empty(t, data)

	// decode a few entries
	buf := &bytes.Buffer{}
	enc := NewEncoder(buf)
	require.NoError(t, enc.WriteField(HeaderField{Name: "foo", Value: "bar"}))
	require.NoError(t, enc.WriteField(HeaderField{Name: "lorem", Value: "ipsum"}))
	data, err = NewDecoder(nil).DecodeFull(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, []HeaderField{
		{Name: "foo", Value: "bar"},
		{Name: "lorem", Value: "ipsum"},
	}, data)
}

func TestDecodeFullIncompleteData(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := NewEncoder(buf)
	require.NoError(t, enc.WriteField(HeaderField{Name: "foo", Value: "bar"}))
	_, err := NewDecoder(nil).DecodeFull(buf.Bytes()[:buf.Len()-2])
	require.EqualError(t, err, "decoding error: truncated headers")
}

func TestDecodeFullRestoresEmitFunc(t *testing.T) {
	var emitFuncCalled bool
	emitFunc := func(HeaderField) {
		emitFuncCalled = true
	}
	decoder := NewDecoder(emitFunc)
	buf := &bytes.Buffer{}
	enc := NewEncoder(buf)
	require.NoError(t, enc.WriteField(HeaderField{Name: "foo", Value: "bar"}))
	_, err := decoder.DecodeFull(buf.Bytes())
	require.NoError(t, err)
	require.False(t, emitFuncCalled)
	_, err = decoder.Write(buf.Bytes())
	require.NoError(t, err)
	require.True(t, emitFuncCalled)
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

	decoder := NewDecoder(func(HeaderField) {})
	hdr := make(map[string]string)
	for b.Loop() {
		hfs, err := decoder.DecodeFull(data)
		if err != nil {
			b.Fatal(err)
		}
		if len(hfs) != numExpected {
			b.Fatalf("expected %d header fields, got %d", numExpected, len(hfs))
		}
		// simulate what a typical HTTP/3 consumer would do with the header fields:
		// populate an http.Header with the header fields
		for _, hf := range hfs {
			hdr[hf.Name] = hf.Value
		}
		clear(hfs)
	}
}
