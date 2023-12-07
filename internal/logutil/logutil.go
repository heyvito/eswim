package logutil

import (
	"fmt"
	"github.com/heyvito/eswim/internal/containers"
	"go.uber.org/zap"
)

// StringerArr is a utility zap.Field that takes a name and a list of items
// that implements fmt.Stringer, including in the string slice returned the
// value returned by each item's String() method.
func StringerArr[S interface{ ~[]E }, E fmt.Stringer](name string, arr S) zap.Field {
	return zap.Strings(name, containers.StrMapper(arr))
}
