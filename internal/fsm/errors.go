package fsm

import "fmt"

// Done is internally used by the fsm package to signal that a given FSM has
// successfully received enough data to compose its value, and such value is
// ready to be returned to the caller.
var Done = fmt.Errorf("FSM Stop Signal")
