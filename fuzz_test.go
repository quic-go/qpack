package qpack

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzDecode(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder := NewDecoder()
		decode := decoder.Decode(data)
		var fields []HeaderField
		for {
			hf, err := decode()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = err.Error()
				return
			}
			fields = append(fields, hf)
		}
		if len(fields) == 0 {
			return
		}

		buf := &bytes.Buffer{}
		encoder := NewEncoder(buf)
		for _, hf := range fields {
			if err := encoder.WriteField(hf); err != nil {
				t.Fatalf("encoding field: %v", err)
			}
		}
		require.NoError(t, encoder.Close())

		decoder2 := NewDecoder()
		decode2 := decoder2.Decode(buf.Bytes())
		var encodedFields []HeaderField
		for {
			hf, err := decode2()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("decoding re-encoded data: %v", err)
			}
			encodedFields = append(encodedFields, hf)
		}
		require.Equal(t, fields, encodedFields)
	})
}
