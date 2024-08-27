package self

import (
	"bytes"
	"math/rand/v2"
	"testing"
	_ "unsafe" // for go:linkname

	"github.com/quic-go/qpack"
	"github.com/stretchr/testify/require"
)

var staticTable []qpack.HeaderField

//go:linkname getStaticTable github.com/quic-go/qpack.getStaticTable
func getStaticTable() []qpack.HeaderField

func init() {
	staticTable = getStaticTable()
}

func randomString(l int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	s := make([]byte, l)
	for i := range s {
		s[i] = charset[rand.IntN(len(charset))]
	}
	return string(s)
}

func getEncoder() (*qpack.Encoder, *bytes.Buffer) {
	output := &bytes.Buffer{}
	return qpack.NewEncoder(output), output
}

func TestEncodingAndDecodingSingleHeaderField(t *testing.T) {
	hf := qpack.HeaderField{
		Name:  randomString(15),
		Value: randomString(15),
	}
	encoder, output := getEncoder()
	require.NoError(t, encoder.WriteField(hf))
	headerFields, err := qpack.NewDecoder(nil).DecodeFull(output.Bytes())
	require.NoError(t, err)
	require.Equal(t, []qpack.HeaderField{hf}, headerFields)
}

func TestEncodingAndDecodingMultipleHeaderFields(t *testing.T) {
	hfs := []qpack.HeaderField{
		{Name: "foo", Value: "bar"},
		{Name: "lorem", Value: "ipsum"},
		{Name: randomString(15), Value: randomString(20)},
	}
	encoder, output := getEncoder()
	for _, hf := range hfs {
		require.NoError(t, encoder.WriteField(hf))
	}
	headerFields, err := qpack.NewDecoder(nil).DecodeFull(output.Bytes())
	require.NoError(t, err)
	require.Equal(t, hfs, headerFields)
}

func TestEncodingAndDecodingMultipleRequests(t *testing.T) {
	hfs1 := []qpack.HeaderField{{Name: "foo", Value: "bar"}}
	hfs2 := []qpack.HeaderField{
		{Name: "lorem", Value: "ipsum"},
		{Name: randomString(15), Value: randomString(20)},
	}
	encoder, output := getEncoder()
	for _, hf := range hfs1 {
		require.NoError(t, encoder.WriteField(hf))
	}
	req1 := append([]byte{}, output.Bytes()...)
	output.Reset()
	for _, hf := range hfs2 {
		require.NoError(t, encoder.WriteField(hf))
	}
	req2 := append([]byte{}, output.Bytes()...)

	var headerFields []qpack.HeaderField
	decoder := qpack.NewDecoder(func(hf qpack.HeaderField) { headerFields = append(headerFields, hf) })
	_, err := decoder.Write(req1)
	require.NoError(t, err)
	require.Equal(t, hfs1, headerFields)
	headerFields = nil
	_, err = decoder.Write(req2)
	require.NoError(t, err)
	require.Equal(t, hfs2, headerFields)
}

// replace one character by a random character at a random position
func replaceRandomCharacter(s string) string {
	pos := rand.IntN(len(s))
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

func check(t *testing.T, encoded []byte, hf qpack.HeaderField) {
	t.Helper()
	headerFields, err := qpack.NewDecoder(nil).DecodeFull(encoded)
	require.NoError(t, err)
	require.Len(t, headerFields, 1)
	require.Equal(t, hf, headerFields[0])
}

func TestUsingStaticTableForFieldNamesWithoutValues(t *testing.T) {
	var hf qpack.HeaderField
	for {
		if entry := staticTable[rand.IntN(len(staticTable))]; len(entry.Value) == 0 {
			hf = qpack.HeaderField{Name: entry.Name}
			break
		}
	}
	encoder, output := getEncoder()
	require.NoError(t, encoder.WriteField(hf))
	encodedLen := output.Len()
	check(t, output.Bytes(), hf)
	encoder, output = getEncoder()
	oldName := hf.Name
	hf.Name = replaceRandomCharacter(hf.Name)
	require.NoError(t, encoder.WriteField(hf))
	t.Logf("Encoding field name:\n\t%s: %d bytes\n\t%s: %d bytes\n", oldName, encodedLen, hf.Name, output.Len())
	require.Greater(t, output.Len(), encodedLen)
}

func TestUsingStaticTableForFieldNamesWithCustomValues(t *testing.T) {
	var hf qpack.HeaderField
	for {
		if entry := staticTable[rand.IntN(len(staticTable))]; len(entry.Value) == 0 {
			hf = qpack.HeaderField{
				Name:  entry.Name,
				Value: randomString(5),
			}
			break
		}
	}
	encoder, output := getEncoder()
	require.NoError(t, encoder.WriteField(hf))
	encodedLen := output.Len()
	check(t, output.Bytes(), hf)
	encoder, output = getEncoder()
	oldName := hf.Name
	hf.Name = replaceRandomCharacter(hf.Name)
	require.NoError(t, encoder.WriteField(hf))
	t.Logf("Encoding field name:\n\t%s: %d bytes\n\t%s: %d bytes", oldName, encodedLen, hf.Name, output.Len())
	require.Greater(t, output.Len(), encodedLen)
}

func TestStaticTableForFieldNamesWithValues(t *testing.T) {
	var hf qpack.HeaderField
	for {
		// Only use values with at least 2 characters.
		// This makes sure that Huffman encoding doesn't compress them as much as encoding it using the static table would.
		if entry := staticTable[rand.IntN(len(staticTable))]; len(entry.Value) > 1 {
			hf = qpack.HeaderField{
				Name:  entry.Name,
				Value: randomString(20),
			}
			break
		}
	}
	encoder, output := getEncoder()
	require.NoError(t, encoder.WriteField(hf))
	encodedLen := output.Len()
	check(t, output.Bytes(), hf)
	encoder, output = getEncoder()
	oldName := hf.Name
	hf.Name = replaceRandomCharacter(hf.Name)
	require.NoError(t, encoder.WriteField(hf))
	t.Logf("Encoding field name:\n\t%s: %d bytes\n\t%s: %d bytes", oldName, encodedLen, hf.Name, output.Len())
	require.Greater(t, output.Len(), encodedLen)
}

func TestStaticTableForFieldValues(t *testing.T) {
	var hf qpack.HeaderField
	for {
		// Only use values with at least 2 characters.
		// This makes sure that Huffman encoding doesn't compress them as much as encoding it using the static table would.
		if entry := staticTable[rand.IntN(len(staticTable))]; len(entry.Value) > 1 {
			hf = qpack.HeaderField{
				Name:  entry.Name,
				Value: entry.Value,
			}
			break
		}
	}
	encoder, output := getEncoder()
	require.NoError(t, encoder.WriteField(hf))
	encodedLen := output.Len()
	check(t, output.Bytes(), hf)
	encoder, output = getEncoder()
	oldValue := hf.Value
	hf.Value = replaceRandomCharacter(hf.Value)
	require.NoError(t, encoder.WriteField(hf))
	t.Logf(
		"Encoding field value:\n\t%s: %s -> %d bytes\n\t%s: %s -> %d bytes",
		hf.Name, oldValue, encodedLen,
		hf.Name, hf.Value, output.Len(),
	)
	require.Greater(t, output.Len(), encodedLen)
}
