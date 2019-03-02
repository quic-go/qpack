package interop

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marten-seemann/qpack"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInterop(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Interop Suite")
}

type request struct {
	headers []qpack.HeaderField
}

type qif struct {
	requests []request
}

var qifs map[string]qif

func readQIFs() {
	qifDir := currentDir() + "/qifs/qifs"
	Expect(qifDir).To(BeADirectory())
	filepath.Walk(qifDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		_, filename := filepath.Split(path)
		ext := filepath.Ext(filename)
		name := filename[:len(filename)-len(ext)]
		file, err := os.Open(path)
		Expect(err).ToNot(HaveOccurred())
		requests := parseRequests(file)
		qifs[name] = qif{requests: requests}
		return nil
	})
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

// Done means that we reached the end of the file
// The headers returned with this call will be empty and should be ignored.
func parseRequest(lr *bufio.Reader) ([]qpack.HeaderField, bool /* done reading */) {
	var headers []qpack.HeaderField
	for {
		line, isPrefix, err := lr.ReadLine()
		if err == io.EOF {
			return headers, true
		}
		Expect(err).ToNot(HaveOccurred())
		Expect(isPrefix).To(BeFalse())
		if len(line) == 0 {
			break
		}
		split := strings.Split(string(line), "\t")
		Expect(split).To(Or(HaveLen(1), HaveLen(2)))
		name := split[0]
		var val string
		if len(split) == 2 {
			val = split[1]
		}
		headers = append(headers, qpack.HeaderField{Name: name, Value: val})
	}
	return headers, false
}

var _ = BeforeSuite(func() {
	qifs = make(map[string]qif)
	readQIFs()
})
