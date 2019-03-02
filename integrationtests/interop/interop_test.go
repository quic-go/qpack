package interop

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/marten-seemann/qpack"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func currentDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Failed to get current frame")
	}
	return path.Dir(filename)
}

var _ = Describe("Interop", func() {

	// find all encoded files with a dynamic table size of 0
	findFiles := func() []string {
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

	parseInput := func(r io.Reader) (uint64 /* stream ID */, []byte) {
		prefix := make([]byte, 12)
		_, err := io.ReadFull(r, prefix)
		Expect(err).ToNot(HaveOccurred())
		streamID := binary.BigEndian.Uint64(prefix[:8])
		length := binary.BigEndian.Uint32(prefix[8:12])
		if length > (1 << 15) { // DoS prevention
			Fail("input too long")
		}
		data := make([]byte, int(length))
		_, err = io.ReadFull(r, data)
		Expect(err).ToNot(HaveOccurred())
		return streamID, data
	}

	filenames := findFiles()
	for i := range filenames {
		path := filenames[i]
		fpath, filename := filepath.Split(path)
		prettyPath := path[len(filepath.Dir(filepath.Dir(filepath.Dir(fpath))))+1:]

		It(fmt.Sprintf("using %s", prettyPath), func() {
			qif, ok := qifs[strings.Split(filename, ".")[0]]
			Expect(ok).To(BeTrue())

			file, err := os.Open(path)
			var headers []qpack.HeaderField
			decoder := qpack.NewDecoder(func(hf qpack.HeaderField) {
				headers = append(headers, hf)
			})
			var numRequests, numHeaderFields int
			Expect(qif.requests).ToNot(BeEmpty())
			for _, req := range qif.requests {
				Expect(err).ToNot(HaveOccurred())
				_, data := parseInput(file)
				_, err = decoder.Write(data)
				Expect(err).ToNot(HaveOccurred())
				Expect(headers).To(Equal(req.headers))
				numRequests++
				numHeaderFields += len(headers)
				headers = nil
				decoder.Close()
			}
			fmt.Fprintf(GinkgoWriter, "Decoded %d requests containing %d header fields.\n", len(qif.requests), numHeaderFields)
		})
	}
})
