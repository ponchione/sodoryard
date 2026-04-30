package id

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

var (
	mu           sync.Mutex
	lastUnixMS   uint64
	lastSequence uint64
)

// New returns a UUIDv7 string.
//
// This implementation is intentionally hand-rolled instead of using a library
// so Sodoryard can guarantee lexicographic ordering even for IDs generated
// within the same millisecond.
func New() string {
	timestamp := uint64(time.Now().UnixMilli())
	sequence := nextSequence(timestamp)

	var b [16]byte
	b[0] = byte(timestamp >> 40)
	b[1] = byte(timestamp >> 32)
	b[2] = byte(timestamp >> 24)
	b[3] = byte(timestamp >> 16)
	b[4] = byte(timestamp >> 8)
	b[5] = byte(timestamp)

	b[6] = 0x70 | byte((sequence>>58)&0x0f)
	b[7] = byte(sequence >> 50)
	b[8] = 0x80 | byte((sequence>>44)&0x3f)
	b[9] = byte(sequence >> 36)
	b[10] = byte(sequence >> 28)
	b[11] = byte(sequence >> 20)
	b[12] = byte(sequence >> 12)
	b[13] = byte(sequence >> 4)
	b[14] = byte(sequence << 4)
	b[15] = 0

	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3],
		b[4], b[5],
		b[6], b[7],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15],
	)
}

func nextSequence(timestamp uint64) uint64 {
	mu.Lock()
	defer mu.Unlock()

	if timestamp != lastUnixMS {
		lastUnixMS = timestamp
		lastSequence = randomSequence()
		return lastSequence
	}

	lastSequence++
	return lastSequence
}

func randomSequence() uint64 {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(err)
	}
	return binary.BigEndian.Uint64(seed[:]) & ((1 << 62) - 1)
}
