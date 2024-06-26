# QPACK

[![Godoc Reference](https://img.shields.io/badge/godoc-reference-blue.svg?style=flat-square)](https://godoc.org/github.com/quic-go/qpack)
[![Code Coverage](https://img.shields.io/codecov/c/github/quic-go/qpack/master.svg?style=flat-square)](https://codecov.io/gh/quic-go/qpack)

This is a minimal QPACK ([RFC 9204](https://datatracker.ietf.org/doc/html/rfc9204)) implementation in Go. It is minimal in the sense that it doesn't use the dynamic table at all, but just the static table and (Huffman encoded) string literals. Wherever possible, it reuses code from the [HPACK implementation in the Go standard library](https://github.com/golang/net/tree/master/http2/hpack).

It is interoperable with other QPACK implemetations (both encoders and decoders), however it won't achieve a high compression efficiency.

## Running the Interop Tests

Install the [QPACK interop files](https://github.com/qpackers/qifs/) by running
```bash
git submodule update --init --recursive
```

Then run the tests:
```bash
ginkgo -r integrationtests
```
