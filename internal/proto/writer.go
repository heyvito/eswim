package proto

func newWriter(buf []byte) *Writer {
	return &Writer{buffer: buf, cursor: 0}
}

type Writer struct {
	buffer []byte
	cursor int
}

func (w *Writer) u8(b uint8) *Writer {
	w.buffer[w.cursor] = b
	w.cursor++
	return w
}

func (w *Writer) u16(val uint16) *Writer {
	u16Marshal(w.buffer[w.cursor:], val)
	w.cursor += 2
	return w
}

func (w *Writer) u32(val uint32) *Writer {
	u32Marshal(w.buffer[w.cursor:], val)
	w.cursor += 4
	return w
}

func (w *Writer) bytes(value []byte) *Writer {
	copy(w.buffer[w.cursor:], value)
	w.cursor += len(value)
	return w
}

func (w *Writer) encoder(obj Encoder) *Writer {
	obj.Encode(w.buffer[w.cursor:])
	w.cursor += obj.RequiredSize()
	return w
}

func (w *Writer) encoderNoChain(obj Encoder) {
	w.encoder(obj)
}

func (w *Writer) ip(addr IP) *Writer {
	return w.bytes(addr.Bytes())
}
