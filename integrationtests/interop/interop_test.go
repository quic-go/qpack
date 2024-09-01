package interop

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/quic-go/qpack"

	"github.com/stretchr/testify/require"
)

type request struct {
	headers []qpack.HeaderField
}

type qif struct {
	requests []request
}

var qifs map[string]qif

func init() {
	qifs = make(map[string]qif)
	readQIFs()
}

func readQIFs() {
	qifDir := currentDir() + "/qifs/qifs"
	if err := filepath.Walk(qifDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		_, filename := filepath.Split(path)
		name := filename[:len(filename)-len(filepath.Ext(filename))]
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		requests := parseRequests(file)
		qifs[name] = qif{requests: requests}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}

func parseRequests(r io.Reader) []request {
	lr := bufio.NewReader(r)
	var reqs []request
	for {
		headers, done := parseRequest(lr)
		if done {
			break
		}
		reqs = append(reqs, request{headers})
	}
	return reqs
}

func parseRequest(lr *bufio.Reader) (headers []qpack.HeaderField, done bool) {
	for {
		line, isPrefix, err := lr.ReadLine()
		if err == io.EOF {
			return headers, true
		}
		if err != nil {
			return nil, true
		}
		if isPrefix {
			return nil, true
		}
		if len(line) == 0 {
			break
		}
		split := strings.Split(string(line), "\t")
		if len(split) != 1 && len(split) != 2 {
			return nil, true
		}
		name := split[0]
		var val string
		if len(split) == 2 {
			val = split[1]
		}
		headers = append(headers, qpack.HeaderField{Name: name, Value: val})
	}
	return headers, false
}

func currentDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Failed to get current frame")
	}
	return path.Dir(filename)
}

func findFiles() []string {
	var files []string
	encodedDir := currentDir() + "/qifs/encoded/qpack-06/"
	filepath.Walk(encodedDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		_, file := filepath.Split(path)
		split := strings.Split(file, ".")
		tableSize := split[len(split)-3]
		if tableSize == "0" {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func parseInput(r io.Reader) (uint64, []byte) {
	prefix := make([]byte, 12)
	if _, err := io.ReadFull(r, prefix); err != nil {
		return 0, nil
	}
	streamID := binary.BigEndian.Uint64(prefix[:8])
	length := binary.BigEndian.Uint32(prefix[8:12])
	if length > 1<<15 {
		return 0, nil
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil
	}
	return streamID, data
}

func TestInteropDecodingEncodedFiles(t *testing.T) {
	filenames := findFiles()
	for _, path := range filenames {
		fpath, filename := filepath.Split(path)
		prettyPath := path[len(filepath.Dir(filepath.Dir(filepath.Dir(fpath))))+1:]

		t.Run(fmt.Sprintf("Decoding_%s", prettyPath), func(t *testing.T) {
			qif, ok := qifs[strings.Split(filename, ".")[0]]
			require.True(t, ok)

			file, err := os.Open(path)
			require.NoError(t, err)
			defer file.Close()

			var numRequests, numHeaderFields int
			require.NotEmpty(t, qif.requests)

			var headers []qpack.HeaderField
			decoder := qpack.NewDecoder(func(hf qpack.HeaderField) { headers = append(headers, hf) })

			for _, req := range qif.requests {
				_, data := parseInput(file)
				require.NotNil(t, data)

				_, err = decoder.Write(data)
				require.NoError(t, err)

				require.Equal(t, req.headers, headers)

				numRequests++
				numHeaderFields += len(headers)
				headers = nil
				decoder.Close()
			}

			t.Logf("Decoded %d requests containing %d header fields.", len(qif.requests), numHeaderFields)
		})
	}
}
