package qpack

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/quic-go/qpack"
)

func Fuzz(data []byte) int {
	if len(data) < 1 {
		return 0
	}

	chunkLen := int(data[0]) + 1
	data = data[1:]

	fields, err := qpack.NewDecoder(nil).DecodeFull(data)
	if err != nil {
		return 0
	}
	if len(fields) == 0 {
		return 0
	}

	var writtenFields []qpack.HeaderField
	decoder := qpack.NewDecoder(func(hf qpack.HeaderField) {
		writtenFields = append(writtenFields, hf)
	})
	for len(data) > 0 {
		var chunk []byte
		if chunkLen <= len(data) {
			chunk = data[:chunkLen]
			data = data[chunkLen:]
		} else {
			chunk = data
			data = nil
		}
		n, err := decoder.Write(chunk)
		if err != nil {
			return 0
		}
		if n != len(chunk) {
			panic("len error")
		}
	}
	if !reflect.DeepEqual(fields, writtenFields) {
		fmt.Printf("%#v vs %#v", fields, writtenFields)
		panic("Write() and DecodeFull() produced different results")
	}

	buf := &bytes.Buffer{}
	encoder := qpack.NewEncoder(buf)
	for _, hf := range fields {
		if err := encoder.WriteField(hf); err != nil {
			panic(err)
		}
	}
	if err := encoder.Close(); err != nil {
		panic(err)
	}

	encodedFields, err := qpack.NewDecoder(nil).DecodeFull(buf.Bytes())
	if err != nil {
		fmt.Printf("Fields: %#v\n", fields)
		panic(err)
	}
	if !reflect.DeepEqual(fields, encodedFields) {
		fmt.Printf("%#v vs %#v", fields, encodedFields)
		panic("unequal")
	}
	return 0
}
