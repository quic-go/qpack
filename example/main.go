package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/quic-go/qpack"
)

func main() {
	file, err := os.Open("example/fb-req-hq.out.0.0.0")
	if err != nil {
		panic(err)
	}

	dec := qpack.NewDecoder(emitFunc)
	for {
		in, err := decodeInput(file)
		if err != nil {
			panic(err)
		}
		fmt.Printf("\nRequest on stream %d:\n", in.streamID)
		dec.Write(in.data)
	}
}

func emitFunc(hf qpack.HeaderField) {
	fmt.Printf("%#v\n", hf)
}

type input struct {
	streamID uint64
	data     []byte
}

func decodeInput(r io.Reader) (*input, error) {
	prefix := make([]byte, 12)
	if _, err := io.ReadFull(r, prefix); err != nil {
		return nil, errors.New("insufficient data for prefix")
	}
	streamID := binary.BigEndian.Uint64(prefix[:8])
	length := binary.BigEndian.Uint32(prefix[8:12])
	if length > (1 << 15) {
		return nil, errors.New("input too long")
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, errors.New("incomplete data")
	}
	return &input{
		streamID: streamID,
		data:     data,
	}, nil
}
