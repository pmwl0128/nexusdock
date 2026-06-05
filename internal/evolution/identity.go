package evolution

import (
	"crypto/rand"
	"fmt"
	"time"
)

type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type IDGenerator interface{ NewID() string }

type uuidGenerator struct{}

func (uuidGenerator) NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failures are exceptional; timestamp preserves uniqueness enough
		// for the caller to persist and detect collisions.
		return fmt.Sprintf("00000000-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffffffff)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func defaultClock(c Clock) Clock {
	if c == nil {
		return realClock{}
	}
	return c
}

func defaultIDs(g IDGenerator) IDGenerator {
	if g == nil {
		return uuidGenerator{}
	}
	return g
}
