package qpack

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func randomString(l int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	s := make([]byte, l)
	for i := range s {
		s[i] = charset[rand.IntN(len(charset))]
	}
	return string(s)
}

func getEncoder() (*Encoder, *bytes.Buffer) {
	output := &bytes.Buffer{}
	return NewEncoder(output), output
}

func TestEncodeDecode(t *testing.T) {
	hfs := []HeaderField{
		{Name: "foo", Value: "bar"},
		{Name: "lorem", Value: "ipsum"},
		{Name: randomString(15), Value: randomString(20)},
	}
	encoder, output := getEncoder()
	for _, hf := range hfs {
		require.NoError(t, encoder.WriteField(hf))
	}
	headerFields := decodeAll(t, NewDecoder().Decode(output.Bytes()))
	require.Equal(t, hfs, headerFields)
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

func check(t *testing.T, encoded []byte, hf HeaderField) {
	t.Helper()

	headerFields := decodeAll(t, NewDecoder().Decode(encoded))
	require.Len(t, headerFields, 1)
	require.Equal(t, hf, headerFields[0])
}

func TestStaticTableForFieldNamesWithoutValues(t *testing.T) {
	for i := range 10 {
		t.Run(fmt.Sprintf("run %d", i), func(t *testing.T) {
			testStaticTableForFieldNamesWithoutValues(t)
		})
	}
}

func testStaticTableForFieldNamesWithoutValues(t *testing.T) {
	var hf HeaderField
	for {
		if entry := staticTableEntries[rand.IntN(len(staticTableEntries))]; len(entry.Value) == 0 {
			hf = HeaderField{Name: entry.Name}
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

func TestStaticTableForFieldNamesWithCustomValues(t *testing.T) {
	for i := range 10 {
		t.Run(fmt.Sprintf("run %d", i), func(t *testing.T) {
			testStaticTableForFieldNamesWithCustomValues(t)
		})
	}
}

func testStaticTableForFieldNamesWithCustomValues(t *testing.T) {
	var hf HeaderField
	for {
		if entry := staticTableEntries[rand.IntN(len(staticTableEntries))]; len(entry.Value) == 0 {
			hf = HeaderField{
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
	for i := range 10 {
		t.Run(fmt.Sprintf("run %d", i), func(t *testing.T) {
			testStaticTableForFieldNamesWithValues(t)
		})
	}
}

func testStaticTableForFieldNamesWithValues(t *testing.T) {
	var hf HeaderField
	for {
		// Only use values with at least 2 characters.
		// This makes sure that Huffman encoding doesn't compress them as much as encoding it using the static table would.
		if entry := staticTableEntries[rand.IntN(len(staticTableEntries))]; len(entry.Value) > 1 {
			hf = HeaderField{
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
	for i := range 10 {
		t.Run(fmt.Sprintf("run %d", i), func(t *testing.T) {
			testStaticTableForFieldValues(t)
		})
	}
}

func testStaticTableForFieldValues(t *testing.T) {
	var hf HeaderField
	for {
		// Only use values with at least 2 characters.
		// This makes sure that Huffman encoding doesn't compress them as much as encoding it using the static table would.
		if entry := staticTableEntries[rand.IntN(len(staticTableEntries))]; len(entry.Value) > 1 {
			hf = HeaderField{
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
