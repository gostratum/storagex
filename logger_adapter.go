package storagex

import (
	"fmt"

	"github.com/gostratum/core/logx"
)

// ArgsToFields converts simple key/value pairs into logx.Field values using
// logx.Any. This helper is used during the migration from the old
// storagex logging adapter to the canonical core/logx.Logger.
//
// Example:
//
//	logger.Info("message", storagex.ArgsToFields("key", val, "other", v2)...)
func ArgsToFields(args ...any) []logx.Field {
	if len(args) == 0 {
		return nil
	}
	fields := make([]logx.Field, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		var key string
		var val any
		if i < len(args) {
			if ks, ok := args[i].(string); ok {
				key = ks
			} else {
				key = fmt.Sprintf("arg%d", i)
			}
		}
		if i+1 < len(args) {
			val = args[i+1]
		} else {
			val = nil
		}
		fields = append(fields, logx.Any(key, val))
	}
	return fields
}
