package qpack

import (
	"bytes"
	"fmt"
	"io"
	"reflect"

	"github.com/quic-go/qpack"
)

func Fuzz(data []byte) int {
	decoder := qpack.NewDecoder()
	decode := decoder.Decode(data)
	var fields []qpack.HeaderField
	for {
		hf, err := decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0
		}
		fields = append(fields, hf)
	}
	if len(fields) == 0 {
		return 0
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

	decoder2 := qpack.NewDecoder()
	decode2 := decoder2.Decode(buf.Bytes())
	var encodedFields []qpack.HeaderField
	for {
		hf, err := decode2()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Fields: %#v\n", fields)
			panic(err)
		}
		encodedFields = append(encodedFields, hf)
	}
	if !reflect.DeepEqual(fields, encodedFields) {
		fmt.Printf("%#v vs %#v", fields, encodedFields)
		panic("unequal")
	}
	return 0
}
