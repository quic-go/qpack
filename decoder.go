package qpack

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/net/http2/hpack"
)

// An invalidIndexError is returned when decoding encounters an invalid index
// (e.g., an index that is out of bounds for the static table).
type invalidIndexError int

func (e invalidIndexError) Error() string {
	return fmt.Sprintf("invalid indexed representation index %d", int(e))
}

var (
	errNoDynamicTable     = errors.New("no dynamic table")
	errInvalidTableIndex  = errors.New("invalid dynamic table index")
	errTableCapacityLimit = errors.New("dynamic table capacity exceeded")
)

// DynamicTable represents the QPACK dynamic table.
// It's a FIFO queue where new entries are added at the front (index 0)
// and old entries are evicted from the back when capacity is exceeded.
type DynamicTable struct {
	entries     []HeaderField
	size        uint64 // current size in bytes
	capacity    uint64 // maximum capacity in bytes
	insertCount uint64 // total number of entries ever inserted (for absolute indexing)
}

// NewDynamicTable creates a new dynamic table with the given capacity.
func NewDynamicTable(capacity uint64) *DynamicTable {
	return &DynamicTable{
		entries:  make([]HeaderField, 0, 64),
		capacity: capacity,
	}
}

// SetCapacity sets the maximum capacity of the dynamic table.
// If the new capacity is smaller, entries are evicted.
func (dt *DynamicTable) SetCapacity(capacity uint64) {
	dt.capacity = capacity
	dt.evict()
}

// Insert adds a new entry to the dynamic table.
// Returns the absolute index of the inserted entry.
func (dt *DynamicTable) Insert(hf HeaderField) uint64 {
	entrySize := headerFieldSize(hf)

	// Evict entries if needed to make room
	for dt.size+entrySize > dt.capacity && len(dt.entries) > 0 {
		// Remove from back (oldest entry)
		last := dt.entries[len(dt.entries)-1]
		dt.entries = dt.entries[:len(dt.entries)-1]
		dt.size -= headerFieldSize(last)
	}

	// If entry is too large for table, don't insert but still increment count
	if entrySize > dt.capacity {
		dt.insertCount++
		return dt.insertCount - 1
	}

	// Insert at front (newest entry)
	dt.entries = append([]HeaderField{hf}, dt.entries...)
	dt.size += entrySize
	dt.insertCount++

	return dt.insertCount - 1
}

// evict removes entries until size <= capacity
func (dt *DynamicTable) evict() {
	for dt.size > dt.capacity && len(dt.entries) > 0 {
		last := dt.entries[len(dt.entries)-1]
		dt.entries = dt.entries[:len(dt.entries)-1]
		dt.size -= headerFieldSize(last)
	}
}

// AtRelative returns the entry at a relative index (0 = most recent).
func (dt *DynamicTable) AtRelative(relIndex uint64) (HeaderField, bool) {
	if relIndex >= uint64(len(dt.entries)) {
		return HeaderField{}, false
	}
	return dt.entries[relIndex], true
}

// AtAbsolute returns the entry at an absolute index.
// Absolute index = insertCount at time of insertion.
func (dt *DynamicTable) AtAbsolute(absIndex uint64) (HeaderField, bool) {
	if absIndex >= dt.insertCount {
		return HeaderField{}, false
	}
	// Convert absolute to relative
	// relIndex = insertCount - 1 - absIndex
	relIndex := dt.insertCount - 1 - absIndex
	return dt.AtRelative(relIndex)
}

// AtPostBase returns the entry at a post-base index.
// Post-base indices are used for entries inserted after the base.
func (dt *DynamicTable) AtPostBase(base, postBaseIndex uint64) (HeaderField, bool) {
	absIndex := base + postBaseIndex
	return dt.AtAbsolute(absIndex)
}

// InsertCount returns the total number of entries ever inserted.
func (dt *DynamicTable) InsertCount() uint64 {
	return dt.insertCount
}

// Len returns the current number of entries in the table.
func (dt *DynamicTable) Len() int {
	return len(dt.entries)
}

// headerFieldSize returns the size of a header field as defined by QPACK.
// Size = len(name) + len(value) + 32
func headerFieldSize(hf HeaderField) uint64 {
	return uint64(len(hf.Name) + len(hf.Value) + 32)
}

// A Decoder decodes QPACK header blocks.
// A Decoder can be reused to decode multiple header blocks on different streams
// on the same connection (e.g., headers then trailers).
type Decoder struct {
	dynamicTable *DynamicTable
	mu           sync.RWMutex

	// maxTableCapacity is the maximum capacity we'll accept
	maxTableCapacity uint64

	// cond is used to signal when new entries are inserted into the dynamic table
	// This allows the decoder to block until the required insert count is reached
	cond *sync.Cond

	// blockingTimeout is the maximum time to wait for encoder instructions
	// Default is 5 seconds
	blockingTimeout time.Duration
}

// DecodeFunc is a function that decodes the next header field from a header block.
// It should be called repeatedly until it returns io.EOF.
// It returns io.EOF when all header fields have been decoded.
// Any error other than io.EOF indicates a decoding error.
type DecodeFunc func() (HeaderField, error)

// NewDecoder returns a new Decoder with a dynamic table.
func NewDecoder() *Decoder {
	d := &Decoder{
		dynamicTable:     NewDynamicTable(0),
		maxTableCapacity: 65536, // Default max capacity
		blockingTimeout:  5 * time.Second,
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

// NewDecoderWithCapacity returns a new Decoder with the given max table capacity.
func NewDecoderWithCapacity(maxCapacity uint64) *Decoder {
	d := &Decoder{
		dynamicTable:     NewDynamicTable(maxCapacity),
		maxTableCapacity: maxCapacity,
		blockingTimeout:  5 * time.Second,
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

// SetDynamicTableCapacity processes a Set Dynamic Table Capacity instruction.
func (d *Decoder) SetDynamicTableCapacity(capacity uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if capacity > d.maxTableCapacity {
		return errTableCapacityLimit
	}
	d.dynamicTable.SetCapacity(capacity)
	return nil
}

// InsertWithNameReference processes an Insert With Name Reference instruction.
// The name comes from either the static or dynamic table.
func (d *Decoder) InsertWithNameReference(isStatic bool, nameIndex uint64, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var name string
	if isStatic {
		if nameIndex >= uint64(len(staticTableEntries)) {
			return invalidIndexError(nameIndex)
		}
		name = staticTableEntries[nameIndex].Name
	} else {
		hf, ok := d.dynamicTable.AtRelative(nameIndex)
		if !ok {
			return errInvalidTableIndex
		}
		name = hf.Name
	}

	d.dynamicTable.Insert(HeaderField{Name: name, Value: value})
	return nil
}

// InsertWithoutNameReference processes an Insert Without Name Reference instruction.
func (d *Decoder) InsertWithoutNameReference(name, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.dynamicTable.Insert(HeaderField{Name: name, Value: value})
	return nil
}

// Duplicate processes a Duplicate instruction.
func (d *Decoder) Duplicate(relIndex uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	hf, ok := d.dynamicTable.AtRelative(relIndex)
	if !ok {
		return errInvalidTableIndex
	}
	d.dynamicTable.Insert(hf)
	return nil
}

// ProcessEncoderInstructions processes instructions from the encoder stream.
// This should be called with data received on the encoder stream.
// After processing, it broadcasts to wake any decoders waiting for entries.
func (d *Decoder) ProcessEncoderInstructions(data []byte) error {
	for len(data) > 0 {
		b := data[0]
		var err error

		switch {
		case b&0x80 > 0:
			// Insert With Name Reference
			// 1Txxxxxx - T=1 for static, T=0 for dynamic
			if b&0x40 > 0 {
				data, err = d.processInsertWithStaticNameRef(data)
			} else {
				data, err = d.processInsertWithDynamicNameRef(data)
			}
		case b&0x40 > 0:
			// Insert Without Name Reference
			// 01xxxxxx
			data, err = d.processInsertWithoutNameRef(data)
		case b&0x20 > 0:
			// Set Dynamic Table Capacity
			// 001xxxxx
			data, err = d.processSetCapacity(data)
		default:
			// Duplicate
			// 000xxxxx
			data, err = d.processDuplicate(data)
		}

		if err != nil {
			return err
		}
	}

	// Broadcast to wake any decoders waiting for entries
	d.cond.Broadcast()
	return nil
}

func (d *Decoder) processInsertWithStaticNameRef(buf []byte) ([]byte, error) {
	// 1 1 xxxxxx - static table reference
	nameIndex, rest, err := readVarInt(6, buf)
	if err != nil {
		return nil, err
	}

	// Read value
	if len(rest) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	usesHuffman := rest[0]&0x80 > 0
	value, rest, err := d.readString(rest, 7, usesHuffman)
	if err != nil {
		return nil, err
	}

	if err := d.InsertWithNameReference(true, nameIndex, value); err != nil {
		return nil, err
	}
	return rest, nil
}

func (d *Decoder) processInsertWithDynamicNameRef(buf []byte) ([]byte, error) {
	// 1 0 xxxxxx - dynamic table reference
	nameIndex, rest, err := readVarInt(6, buf)
	if err != nil {
		return nil, err
	}

	// Read value
	if len(rest) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	usesHuffman := rest[0]&0x80 > 0
	value, rest, err := d.readString(rest, 7, usesHuffman)
	if err != nil {
		return nil, err
	}

	if err := d.InsertWithNameReference(false, nameIndex, value); err != nil {
		return nil, err
	}
	return rest, nil
}

func (d *Decoder) processInsertWithoutNameRef(buf []byte) ([]byte, error) {
	// 01 N xxxxx
	usesHuffmanName := buf[0]&0x20 > 0
	name, rest, err := d.readString(buf, 5, usesHuffmanName)
	if err != nil {
		return nil, err
	}

	if len(rest) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	usesHuffmanValue := rest[0]&0x80 > 0
	value, rest, err := d.readString(rest, 7, usesHuffmanValue)
	if err != nil {
		return nil, err
	}

	if err := d.InsertWithoutNameReference(name, value); err != nil {
		return nil, err
	}
	return rest, nil
}

func (d *Decoder) processSetCapacity(buf []byte) ([]byte, error) {
	// 001 xxxxx
	capacity, rest, err := readVarInt(5, buf)
	if err != nil {
		return nil, err
	}

	if err := d.SetDynamicTableCapacity(capacity); err != nil {
		return nil, err
	}
	return rest, nil
}

func (d *Decoder) processDuplicate(buf []byte) ([]byte, error) {
	// 000 xxxxx
	relIndex, rest, err := readVarInt(5, buf)
	if err != nil {
		return nil, err
	}

	if err := d.Duplicate(relIndex); err != nil {
		return nil, err
	}
	return rest, nil
}

// InsertCount returns the current insert count of the dynamic table.
func (d *Decoder) InsertCount() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dynamicTable.InsertCount()
}

// Decode returns a function that decodes header fields from the given header block.
// It does not copy the slice; the caller must ensure it remains valid during decoding.
func (d *Decoder) Decode(p []byte) DecodeFunc {
	var readRequiredInsertCount bool
	var readDeltaBase bool
	var base uint64
	var requiredInsertCount uint64

	return func() (HeaderField, error) {
		if !readRequiredInsertCount {
			encodedInsertCount, rest, err := readVarInt(8, p)
			if err != nil {
				return HeaderField{}, err
			}
			p = rest
			readRequiredInsertCount = true

			// Decode Required Insert Count
			// If encoded is 0, required insert count is 0
			if encodedInsertCount == 0 {
				requiredInsertCount = 0
				base = 0
			} else {
				// Decode per RFC 9204 Section 4.5.1.1
				d.mu.Lock()
				insertCount := d.dynamicTable.InsertCount()

				maxEntries := d.maxTableCapacity / 32
				if maxEntries == 0 {
					maxEntries = 1
				}
				fullRange := 2 * maxEntries
				if encodedInsertCount > fullRange {
					d.mu.Unlock()
					return HeaderField{}, errors.New("invalid encoded insert count")
				}

				maxValue := insertCount + maxEntries
				maxWrapped := (maxValue / fullRange) * fullRange
				requiredInsertCount = maxWrapped + encodedInsertCount - 1

				// Handle wrap-around
				if requiredInsertCount > maxValue {
					if requiredInsertCount <= fullRange {
						d.mu.Unlock()
						return HeaderField{}, errors.New("invalid required insert count")
					}
					requiredInsertCount -= fullRange
				}

				// Wait for encoder stream to provide required entries (RFC 9204 Section 2.1.2)
				// Block until requiredInsertCount <= insertCount or timeout
				deadline := time.Now().Add(d.blockingTimeout)
				for requiredInsertCount > d.dynamicTable.InsertCount() {
					if time.Now().After(deadline) {
						currentCount := d.dynamicTable.InsertCount()
						d.mu.Unlock()
						return HeaderField{}, fmt.Errorf("timeout waiting for encoder stream: required insert count %d exceeds current %d", requiredInsertCount, currentCount)
					}
					// Wait for signal from ProcessEncoderInstructions
					// Use a timed wait by spawning a goroutine that will broadcast after a short interval
					done := make(chan struct{})
					go func() {
						time.Sleep(10 * time.Millisecond)
						d.cond.Broadcast()
						close(done)
					}()
					d.cond.Wait()
					<-done
				}
				d.mu.Unlock()

				// Set base for post-base references
				base = requiredInsertCount
			}
		}

		if !readDeltaBase {
			// Read Sign bit and Delta Base
			if len(p) == 0 {
				return HeaderField{}, io.ErrUnexpectedEOF
			}
			sign := p[0]&0x80 > 0
			deltaBase, rest, err := readVarInt(7, p)
			if err != nil {
				return HeaderField{}, err
			}
			p = rest
			readDeltaBase = true

			// Calculate base
			if sign {
				// Negative: Base = ReqInsertCount - DeltaBase - 1
				if deltaBase+1 > requiredInsertCount {
					return HeaderField{}, errors.New("invalid delta base")
				}
				base = requiredInsertCount - deltaBase - 1
			} else {
				// Positive: Base = ReqInsertCount + DeltaBase
				base = requiredInsertCount + deltaBase
			}
		}

		if len(p) == 0 {
			return HeaderField{}, io.EOF
		}

		b := p[0]
		var hf HeaderField
		var rest []byte
		var err error
		switch {
		case (b & 0x80) > 0: // 1xxxxxxx - Indexed Field Line
			hf, rest, err = d.parseIndexedHeaderField(p, base)
		case (b & 0xc0) == 0x40: // 01xxxxxx - Literal Field Line With Name Reference
			hf, rest, err = d.parseLiteralHeaderField(p, base)
		case (b & 0xe0) == 0x20: // 001xxxxx - Literal Field Line With Literal Name
			hf, rest, err = d.parseLiteralHeaderFieldWithoutNameReference(p)
		case (b & 0xf0) == 0x10: // 0001xxxx - Indexed Field Line With Post-Base Index
			hf, rest, err = d.parseIndexedFieldLinePostBase(p, base)
		case (b & 0xf0) == 0x00: // 0000xxxx - Literal Field Line With Post-Base Name Reference
			hf, rest, err = d.parseLiteralFieldLinePostBase(p, base)
		default:
			err = fmt.Errorf("unexpected type byte: %#x", b)
		}
		p = rest
		if err != nil {
			return HeaderField{}, err
		}
		return hf, nil
	}
}

func (d *Decoder) parseIndexedHeaderField(buf []byte, base uint64) (_ HeaderField, rest []byte, _ error) {
	// 1 T xxxxxx
	isStatic := buf[0]&0x40 > 0
	index, rest, err := readVarInt(6, buf)
	if err != nil {
		return HeaderField{}, buf, err
	}

	var hf HeaderField
	var ok bool
	if isStatic {
		hf, ok = d.atStatic(index)
	} else {
		// Dynamic table reference (relative to base)
		d.mu.RLock()
		absIndex := base - index - 1
		hf, ok = d.dynamicTable.AtAbsolute(absIndex)
		d.mu.RUnlock()
	}

	if !ok {
		return HeaderField{}, buf, invalidIndexError(index)
	}
	return hf, rest, nil
}

func (d *Decoder) parseIndexedFieldLinePostBase(buf []byte, base uint64) (_ HeaderField, rest []byte, _ error) {
	// 0001 xxxx - Post-Base Index
	index, rest, err := readVarInt(4, buf)
	if err != nil {
		return HeaderField{}, buf, err
	}

	d.mu.RLock()
	hf, ok := d.dynamicTable.AtPostBase(base, index)
	d.mu.RUnlock()

	if !ok {
		return HeaderField{}, buf, invalidIndexError(index)
	}
	return hf, rest, nil
}

func (d *Decoder) parseLiteralHeaderField(buf []byte, base uint64) (_ HeaderField, rest []byte, _ error) {
	// 01 N T xxxx
	// N = never index, T = static/dynamic
	isStatic := buf[0]&0x10 > 0
	index, rest, err := readVarInt(4, buf)
	if err != nil {
		return HeaderField{}, buf, err
	}

	var hf HeaderField
	var ok bool
	if isStatic {
		hf, ok = d.atStatic(index)
	} else {
		// Dynamic table reference
		d.mu.RLock()
		absIndex := base - index - 1
		hf, ok = d.dynamicTable.AtAbsolute(absIndex)
		d.mu.RUnlock()
	}

	if !ok {
		return HeaderField{}, buf, invalidIndexError(index)
	}

	buf = rest
	if len(buf) == 0 {
		return HeaderField{}, buf, io.ErrUnexpectedEOF
	}
	usesHuffman := buf[0]&0x80 > 0
	val, rest, err := d.readString(rest, 7, usesHuffman)
	if err != nil {
		return HeaderField{}, rest, err
	}
	hf.Value = val
	return hf, rest, nil
}

func (d *Decoder) parseLiteralFieldLinePostBase(buf []byte, base uint64) (_ HeaderField, rest []byte, _ error) {
	// 0000 N xxx - Post-Base Name Reference
	index, rest, err := readVarInt(3, buf)
	if err != nil {
		return HeaderField{}, buf, err
	}

	d.mu.RLock()
	hf, ok := d.dynamicTable.AtPostBase(base, index)
	d.mu.RUnlock()

	if !ok {
		return HeaderField{}, buf, invalidIndexError(index)
	}

	buf = rest
	if len(buf) == 0 {
		return HeaderField{}, buf, io.ErrUnexpectedEOF
	}
	usesHuffman := buf[0]&0x80 > 0
	val, rest, err := d.readString(rest, 7, usesHuffman)
	if err != nil {
		return HeaderField{}, rest, err
	}
	hf.Value = val
	return hf, rest, nil
}

func (d *Decoder) parseLiteralHeaderFieldWithoutNameReference(buf []byte) (_ HeaderField, rest []byte, _ error) {
	usesHuffmanForName := buf[0]&0x8 > 0
	name, rest, err := d.readString(buf, 3, usesHuffmanForName)
	if err != nil {
		return HeaderField{}, rest, err
	}
	buf = rest
	if len(buf) == 0 {
		return HeaderField{}, rest, io.ErrUnexpectedEOF
	}
	usesHuffmanForVal := buf[0]&0x80 > 0
	val, rest, err := d.readString(buf, 7, usesHuffmanForVal)
	if err != nil {
		return HeaderField{}, rest, err
	}
	return HeaderField{Name: name, Value: val}, rest, nil
}

func (d *Decoder) readString(buf []byte, n uint8, usesHuffman bool) (string, []byte, error) {
	l, buf, err := readVarInt(n, buf)
	if err != nil {
		return "", nil, err
	}
	if uint64(len(buf)) < l {
		return "", nil, io.ErrUnexpectedEOF
	}
	var val string
	if usesHuffman {
		val, err = hpack.HuffmanDecodeToString(buf[:l])
		if err != nil {
			return "", nil, err
		}
	} else {
		val = string(buf[:l])
	}
	buf = buf[l:]
	return val, buf, nil
}

func (d *Decoder) atStatic(i uint64) (hf HeaderField, ok bool) {
	if i >= uint64(len(staticTableEntries)) {
		return
	}
	return staticTableEntries[i], true
}

// at is for backwards compatibility - looks up in static table only
func (d *Decoder) at(i uint64) (hf HeaderField, ok bool) {
	return d.atStatic(i)
}
