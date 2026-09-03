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

// BuildWithBudget is [Build] with the 500 ms budget made explicit, reachable
// from this package's external test package.
//
// Gate M10 asserts that the deadline is enforced rather than merely present,
// and a deadline that is never approached is not evidence of anything. Driving
// the budget below the cost of a real search over a real corpus is the same
// inequality as a search made slower than the budget, and it is the one a test
// can produce without a SQLite progress handler this driver does not expose.
var BuildWithBudget = build

// QueryFor is the prompt-to-query reduction, reachable from this package's
// external test package. It is the decision this session took that rev.8's M-4
// does not name, and asserting which three terms it chose is a different
// question from what those three then matched.
var QueryFor = queryFor

// Assemble is the body builder, reachable from this package's external test
// package. The alternation between the two ranked lists is what keeps P4 - one
// query reaching what only the other host knows - from being the half that
// never fits, and it needs no database to assert.
var Assemble = assemble
