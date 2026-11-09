package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/klauspost/compress/zstd"
)

const maxRequestZstdWindowSize = 8 << 20

// requestBodyBuffer tracks allocations owned by the current read/decode step.
// Replaced normalized buffers refund prior raw ownership before handing off
// their own reservation to the request-scoped cleanup.
type requestBodyBuffer struct {
	data    []byte
	charged int64
}

func (b *requestBodyBuffer) release(ctx context.Context) {
	charged := b.charged
	*b = requestBodyBuffer{}
	releaseRequestBodyMemory(ctx, charged)
}

// Read bounded chunks as bytes arrive, then assemble the exact-size result.
// This avoids doubling large buffers or eagerly allocating an untrusted
// Content-Length before the corresponding bytes have arrived.
func readRequestBodyChunksWithBudget(ctx context.Context, reader io.Reader, initialCapacity int, contentLength int64) (result requestBodyBuffer, err error) {
	var reserved int64
	defer func() {
		// Chunks and the EOF probe are temporary once the final buffer is
		// assembled. On failure there is no result, so refund all allocations.
		releaseRequestBodyMemory(ctx, reserved-result.charged)
	}()
	capacity := initialCapacity
	var chunks [][]byte
	total := 0
	for {
		chunkCapacity := capacity
		if remaining := contentLength - int64(total); remaining >= 0 && remaining < int64(chunkCapacity) {
			chunkCapacity = int(remaining) + 1
		}
		if err := reserveRequestBodyMemory(ctx, int64(chunkCapacity)); err != nil {
			return requestBodyBuffer{}, err
		}
		reserved += int64(chunkCapacity)
		chunk := make([]byte, chunkCapacity)
		n := 0
		var err error
		for n < len(chunk) && err == nil {
			if err := ctx.Err(); err != nil {
				return requestBodyBuffer{}, err
			}
			var read int
			read, err = reader.Read(chunk[n:])
			n += read
		}
		if err := ctx.Err(); err != nil {
			return requestBodyBuffer{}, err
		}
		if err != nil && err != io.EOF {
			return requestBodyBuffer{}, err
		}
		if n > 0 {
			chunks = append(chunks, chunk[:n])
			total += n
		}
		if err != nil {
			if len(chunks) == 0 {
				return requestBodyBuffer{}, nil
			}
			if len(chunks) == 1 {
				return requestBodyBuffer{data: chunks[0], charged: int64(cap(chunks[0]))}, nil
			}
			if err := reserveRequestBodyMemory(ctx, int64(total)); err != nil {
				return requestBodyBuffer{}, err
			}
			reserved += int64(total)
			body := make([]byte, total)
			offset := 0
			for _, part := range chunks {
				offset += copy(body[offset:], part)
			}
			return requestBodyBuffer{data: body, charged: int64(total)}, nil
		}
		if capacity < requestBodyReadMaxInitCap {
			capacity *= 2
			if capacity > requestBodyReadMaxInitCap {
				capacity = requestBodyReadMaxInitCap
			}
		}
	}
}

func decompressRequestBodyWithBudget(ctx context.Context, encoding string, raw []byte) (requestBodyBuffer, error) {
	var reader io.Reader
	switch encoding {
	case "zstd":
		if err := reserveRequestBodyMemory(ctx, maxRequestZstdWindowSize); err != nil {
			return requestBodyBuffer{}, err
		}
		defer releaseRequestBodyMemory(ctx, maxRequestZstdWindowSize)
		dec, err := zstd.NewReader(bytes.NewReader(raw),
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxWindow(maxRequestZstdWindowSize),
			zstd.WithDecoderMaxMemory(maxDecompressedBodySize))
		if err != nil {
			return requestBodyBuffer{}, err
		}
		defer dec.Close()
		reader = dec
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return requestBodyBuffer{}, err
		}
		defer func() { _ = gr.Close() }()
		reader = gr
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return requestBodyBuffer{}, err
		}
		defer func() { _ = zr.Close() }()
		reader = zr
	default:
		return requestBodyBuffer{}, errors.New("unsupported Content-Encoding")
	}
	decoded, err := readRequestBodyChunksWithBudget(ctx, io.LimitReader(reader, maxDecompressedBodySize+1), requestBodyReadInitCap, -1)
	if err != nil {
		return requestBodyBuffer{}, err
	}
	if len(decoded.data) > maxDecompressedBodySize {
		decoded.release(ctx)
		return requestBodyBuffer{}, &http.MaxBytesError{Limit: maxDecompressedBodySize}
	}
	return decoded, nil
}
