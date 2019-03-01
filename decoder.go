package qpack

import (
	"errors"
	"fmt"

	"golang.org/x/net/http2/hpack"
)

// A decodingError is something the spec defines as a decoding error.
type decodingError struct {
	err error
}

func (de decodingError) Error() string {
	return fmt.Sprintf("decoding error: %v", de.err)
}

// An invalidIndexError is returned when an encoder references a table
// entry before the static table or after the end of the dynamic table.
type invalidIndexError int

func (e invalidIndexError) Error() string {
	return fmt.Sprintf("invalid indexed representation index %d", int(e))
}

// A Decoder is the decoding context for incremental processing of
// header blocks.
type Decoder struct {
	emitFunc func(f HeaderField)

	buf []byte
}

// NewDecoder returns a new decoder
// The emitFunc will be called for each valid field parsed,
// in the same goroutine as calls to Write, before Write returns.
func NewDecoder(emitFunc func(f HeaderField)) *Decoder {
	return &Decoder{emitFunc: emitFunc}
}

func (d *Decoder) Write(p []byte) (int, error) {
	// TODO: handle incomplete writes
	d.buf = p

	if err := d.decode(); err != nil {
		panic(err)
	}
	return len(p), nil
}

func (d *Decoder) decode() error {
	requiredInsertCount, rest, err := readVarInt(8, d.buf)
	if err != nil {
		return err
	}
	if requiredInsertCount != 0 {
		return decodingError{errors.New("expected Required Insert Count to be zero")}
	}
	d.buf = rest
	base, rest, err := readVarInt(7, d.buf)
	if err != nil {
		return err
	}
	if base != 0 {
		return decodingError{errors.New("expected Base to be zero")}
	}
	d.buf = rest

	for len(d.buf) > 0 {
		b := d.buf[0]
		var err error
		switch {
		case b&0x80 > 0:
			err = d.parseIndexedHeaderField()
		case b&0xc0 == 0x40:
			err = d.parseLiteralHeaderField()
		default:
			err = fmt.Errorf("unexpected type byte: %#x", d.buf[0])
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) parseIndexedHeaderField() error {
	if d.buf[0]&0x40 == 0 {
		return errors.New("no dymanic table")
	}
	index, rest, err := readVarInt(6, d.buf)
	if err != nil {
		return err
	}
	d.buf = rest
	hf, ok := d.at(index)
	if !ok {
		return decodingError{invalidIndexError(index)}
	}
	d.emitFunc(hf)
	return nil
}

func (d *Decoder) parseLiteralHeaderField() error {
	b := d.buf
	if b[0]&0x20 > 0 || b[0]&0x10 == 0 {
		return errors.New("no dynamic table")
	}
	index, rest, err := readVarInt(4, d.buf)
	if err != nil {
		return err
	}
	d.buf = rest
	hf, ok := d.at(index)
	if !ok {
		return decodingError{invalidIndexError(index)}
	}
	usesHuffman := d.buf[0]&0x80 > 0
	l, rest, err := readVarInt(7, d.buf)
	if err != nil {
		return err
	}
	d.buf = rest
	if uint64(len(d.buf)) < l {
		return errors.New("too little data")
	}
	var val string
	if usesHuffman {
		var err error
		val, err = hpack.HuffmanDecodeToString(d.buf[:l])
		if err != nil {
			return err
		}
	} else {
		val = string(d.buf[:l])
	}
	d.buf = d.buf[l:]
	hf.Value = val
	d.emitFunc(hf)
	return nil
}

func (d *Decoder) at(i uint64) (hf HeaderField, ok bool) {
	if i >= uint64(len(staticTableEntries)) {
		return
	}
	return staticTableEntries[i], true
}
