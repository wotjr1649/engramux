package inject

// FenceWith is [Fence] with the nonce mint made explicit, reachable from this
// package's external test package.
//
// Gate M9's invariant is that the fenced body never carries the delimiter, and
// the only input that can violate it is a body containing a nonce the mint is
// about to return - which crypto/rand will not produce on purpose. A test that
// cannot supply the mint cannot reach the collision path, and a check nothing
// reaches is a check that can be deleted with every test still green.
//
// This file compiles into this package's test binary and into nothing else, so
// no shipped surface gains a settable random source.
var FenceWith = fence
