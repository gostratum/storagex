package storagex_test

import (
	"fmt"

	"github.com/gostratum/storagex"
)

// Example demonstrates creating a default configuration and using the
// module-level DefaultConfig helper. This example is intentionally small and
// compiles as a test to show usage in package documentation.
func ExampleDefaultConfig() {
	// Create the default configuration and override the bucket name for the
	// example. In real apps you would load configuration via your config
	// loader (for example, github.com/gostratum/core/configx) and supply it
	// to the FX container.
	cfg := storagex.DefaultConfig()
	cfg.Bucket = "example-bucket"

	// Print a trivial value to keep the example deterministic.
	fmt.Println(cfg.Bucket)

	// Output:
	// example-bucket
}
