package identityservice

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"testing/iotest"
)

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestRandomHexTokenHasLength(c *C) {
	val := randomHexToken()
	c.Assert(val, HasLen, 32)
}

func (s *UtilSuite) TestRandomHexTokenIsHex(c *C) {
	val := randomHexToken()
	for i, b := range val {
		switch {
		case (b >= 'a' && b <= 'f') || (b >= '0' && b <= '9'):
			continue
		default:
			c.Logf("char %d of %s was not in the right range",
				i, val)
			c.Fail()
		}
	}
}

func (s *UtilSuite) TestDefaultReader(c *C) {
	raw := make([]byte, 6)
	c.Assert(string(raw), Equals, "\x00\x00\x00\x00\x00\x00")
	n, err := io.ReadFull(randReader, raw)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 6)
	c.Assert(string(raw), Not(Equals), "\x00\x00\x00\x00\x00\x00")
}

func (s *UtilSuite) TestSetReader(c *C) {
	orig := randReader
	// This test will be mutating global state (randReader), ensure that we
	// restore it sanely even if tests fail
	defer func() { randReader = orig }()
	// "randomize" everything to the letter 'n'
	nRandom := bytes.NewBufferString("nnnnnnnnnnnnnnnnnnnnnnn")
	c.Assert(randReader, Equals, rand.Reader)
	cleanup := setReader(nRandom)
	c.Assert(randReader, Equals, nRandom)
	raw := make([]byte, 6)
	n, err := io.ReadFull(randReader, raw)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 6)
	c.Assert(string(raw), Equals, "nnnnnn")
	cleanup()
	c.Assert(randReader, Equals, rand.Reader)
}

// Change how we get random data, the default is to use crypto/rand
// This mostly exists to be able to test error side effects
// The return value is a function you can call to restore the previous
// randomizer
func setReader(r io.Reader) (restore func()) {
	old := randReader
	randReader = r
	return func() {
		randReader = old
	}
}

func (s *UtilSuite) TestNotEnoughRandomBytes(c *C) {
	// No error, just not enough bytes
	shortRand := bytes.NewBufferString("xx")
	cleanup := setReader(shortRand)
	defer cleanup()
	c.Assert(randomHexToken, PanicMatches, "failed to read 16 random bytes \\(read 2 bytes\\): unexpected EOF")
}

type ErrReader struct{}

func (e ErrReader) Read(b []byte) (n int, err error) {
	b[0] = 'x'
	b[1] = 'x'
	b[2] = 'x'
	return 3, fmt.Errorf("Not enough bytes")
}

func (s *UtilSuite) TestRandomBytesError(c *C) {
	// No error, just not enough bytes
	cleanup := setReader(ErrReader{})
	defer cleanup()
	c.Assert(randomHexToken, PanicMatches, "failed to read 16 random bytes \\(read 3 bytes\\): Not enough bytes")
}

func (s *UtilSuite) TestSlowBytes(c *C) {
	// Even when we have to read one byte at a time, we can still get our
	// hex token
	defer setReader(iotest.OneByteReader(rand.Reader))()
	val := randomHexToken()
	c.Assert(val, HasLen, 32)
}
