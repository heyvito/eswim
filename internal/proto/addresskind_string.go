// Code generated by "stringer -type=AddressKind"; DO NOT EDIT.

package proto

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[AddressIPv4-0]
	_ = x[AddressIPv6-1]
}

const _AddressKind_name = "AddressIPv4AddressIPv6"

var _AddressKind_index = [...]uint8{0, 11, 22}

func (i AddressKind) String() string {
	if i >= AddressKind(len(_AddressKind_index)-1) {
		return "AddressKind(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _AddressKind_name[_AddressKind_index[i]:_AddressKind_index[i+1]]
}
