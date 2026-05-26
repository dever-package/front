package transfer

import "io"

type ProgressFunc func(loaded int64, total int64)

type progressReader struct {
	reader io.Reader
	total  int64
	loaded int64
	fn     ProgressFunc
}

func WrapReader(reader io.Reader, total int64, fn ProgressFunc) io.Reader {
	if reader == nil || fn == nil {
		return reader
	}
	return &progressReader{
		reader: reader,
		total:  total,
		fn:     fn,
	}
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.loaded += int64(n)
		r.fn(r.loaded, r.total)
	}
	return n, err
}

func Percent(loaded int64, total int64, start int, end int) int {
	if total <= 0 {
		return -1
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > 100 {
		end = 100
	}
	if loaded < 0 {
		loaded = 0
	}
	if loaded > total {
		loaded = total
	}
	return start + int(loaded*int64(end-start)/total)
}
