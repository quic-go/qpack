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

func TestDecoderRejectsInvalidInputs(t *testing.T) {
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

func doPartialWrites(t *testing.T, decoder *recordingDecoder, data []byte) {
	t.Helper()
	for i := 0; i < len(data)-1; i++ {
		n, err := decoder.Write([]byte{data[i]})
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, decoder.Fields())
	}
	n, err := decoder.Write([]byte{data[len(data)-1]})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, decoder.Fields(), 1)
}

func TestDecoderIndexedHeaderFields(t *testing.T) {
	decoder := newRecordingDecoder()
	data := appendVarInt(nil, 6, 20)
	data[0] ^= 0x80 | 0x40
	doPartialWrites(t, decoder, insertPrefix(data))
	require.Len(t, decoder.Fields(), 1)
	require.Equal(t, staticTableEntries[20], decoder.Fields()[0])
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

func TestDecoderLiteralHeaderFieldWithNameReference(t *testing.T) {
	t.Run("without the N-bit", func(t *testing.T) {
		testDecoderLiteralHeaderFieldWithNameReference(t, false)
	})
	t.Run("with the N-bit", func(t *testing.T) {
		testDecoderLiteralHeaderFieldWithNameReference(t, true)
	})
}

func testDecoderLiteralHeaderFieldWithNameReference(t *testing.T, n bool) {
	decoder := newRecordingDecoder()
	data := appendVarInt(nil, 4, 49)
	data[0] ^= 0x40 | 0x10
	if n {
		data[0] |= 0x20
	}
	data = appendVarInt(data, 7, 6)
	data = append(data, []byte("foobar")...)
	doPartialWrites(t, decoder, insertPrefix(data))
	require.Len(t, decoder.Fields(), 1)
	require.Equal(t, "content-type", decoder.Fields()[0].Name)
	require.Equal(t, "foobar", decoder.Fields()[0].Value)
}

func TestDecoderLiteralHeaderFieldWithNameReferenceAndHuffmanEncoding(t *testing.T) {
	decoder := newRecordingDecoder()
	data := appendVarInt(nil, 4, 49)
	data[0] ^= 0x40 | 0x10
	data2 := appendVarInt(nil, 7, hpack.HuffmanEncodeLength("foobar"))
	data2[0] ^= 0x80
	data = hpack.AppendHuffmanString(append(data, data2...), "foobar")
	doPartialWrites(t, decoder, insertPrefix(data))
	require.Len(t, decoder.Fields(), 1)
	require.Equal(t, "content-type", decoder.Fields()[0].Name)
	require.Equal(t, "foobar", decoder.Fields()[0].Value)
}

func TestDecoderLiteralHeaderFieldWithNameReferenceToTheDynamicTable(t *testing.T) {
	decoder := newRecordingDecoder()
	data := appendVarInt(nil, 4, 49)
	data[0] ^= 0x40 // don't set the static flag (0x10)
	data = appendVarInt(data, 7, 6)
	data = append(data, []byte("foobar")...)
	_, err := decoder.Write(insertPrefix(data))
	require.ErrorIs(t, err, errNoDynamicTable)
}

func TestDecoderLiteralHeaderFieldWithoutNameReference(t *testing.T) {
	decoder := newRecordingDecoder()
	data := appendVarInt(nil, 3, 3)
	data[0] ^= 0x20
	data = append(data, []byte("foo")...)
	data2 := appendVarInt(nil, 7, 3)
	data2 = append(data2, []byte("bar")...)
	data = append(data, data2...)
	doPartialWrites(t, decoder, insertPrefix(data))

	require.Len(t, decoder.Fields(), 1)
	require.Equal(t, "foo", decoder.Fields()[0].Name)
	require.Equal(t, "bar", decoder.Fields()[0].Value)
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
