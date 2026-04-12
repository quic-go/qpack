package qpack

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzDecode(f *testing.F) {
	for _, headerFields := range [][]HeaderField{
		{ // simple GET request: all indexed fields
			{Name: ":method", Value: "GET"},
			{Name: ":scheme", Value: "https"},
			{Name: ":path", Value: "/"},
			{Name: ":authority", Value: "example.com"},
			{Name: "accept", Value: "*/*"},
			{Name: "accept-encoding", Value: "gzip, deflate, br"},
		},
		{ // POST with JSON body: indexed + literal with name reference
			{Name: ":method", Value: "POST"},
			{Name: ":scheme", Value: "https"},
			{Name: ":path", Value: "/api/v1/users"},
			{Name: ":authority", Value: "api.example.com"},
			{Name: "content-type", Value: "application/json"},
			{Name: "content-length", Value: "128"},
			{Name: "accept", Value: "application/json"},
		},
		{ // response with common headers
			{Name: ":status", Value: "200"},
			{Name: "content-type", Value: "text/html; charset=utf-8"},
			{Name: "content-encoding", Value: "gzip"},
			{Name: "vary", Value: "accept-encoding"},
			{Name: "cache-control", Value: "no-cache"},
			{Name: "strict-transport-security", Value: "max-age=31536000; includesubdomains; preload"},
		},
		{ // custom headers: literal without name reference (Huffman-encoded)
			{Name: ":method", Value: "GET"},
			{Name: ":scheme", Value: "https"},
			{Name: ":path", Value: "/"},
			{Name: "x-request-id", Value: "a]cef9-1234-5678"},
			{Name: "x-trace-id", Value: "trace-98765"},
			{Name: "user-agent", Value: "Mozilla/5.0 (X11; Linux x86_64)"},
		},
		{ // 404 response: indexed status + name-referenced values not in static table
			{Name: ":status", Value: "404"},
			{Name: "content-type", Value: "text/plain"},
			{Name: "server", Value: "custom-server/1.0"},
			{Name: "date", Value: "Mon, 01 Jan 2024 00:00:00 GMT"},
			{Name: "x-error-code", Value: "NOT_FOUND"},
		},
	} {
		buf := &bytes.Buffer{}
		encoder := NewEncoder(buf)
		for _, hf := range headerFields {
			require.NoError(f, encoder.WriteField(hf))
		}
		require.NoError(f, encoder.Close())
		f.Add(buf.Bytes())
	}

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
