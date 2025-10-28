package storagex_test

import (
	"testing"

	"github.com/gostratum/storagex"
	"github.com/gostratum/storagex/internal/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// Test that the storagex Module wires an Instrumenter even when no metrics
// or tracer modules are provided (observability is optional).
func TestModuleProvidesInstrumenterWithoutObservability(t *testing.T) {
	// Do not include full storagex.Module() here because testutil.TestModule
	// already provides a test *storagex.Config which would conflict with
	// storagex.NewConfig. Instead only provide the observability provider
	// and rely on the test module for config/keybuilder.
	app := fxtest.New(t,
		fx.Options(
			testutil.TestModule,
			fx.Provide(storagex.NewObservabilityInstrumenter),
			fx.Invoke(func(i *storagex.Instrumenter) {
				require.NotNil(t, i)
			}),
		),
	)

	defer app.RequireStart().RequireStop()
}
