// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

import (
    "math/rand"
    "time"
)

// A GWACL-specific source of pseudo-randomness. We use this so we can safely
// seed it without affecting application-global state in the math package.
var random *rand.Rand

func init() {
    // Seed the pseudo-random number generator. Without this, each run
    // will get the same sequence of results from the math/rand package.
    seed := int64(time.Now().Nanosecond())
    source := rand.NewSource(seed)
    random = rand.New(source)
}
