package proto

var (
	ipv4Healthy = Node{false, 1, ipV4(0x01, 0x02, 0x03, 0x04)}
	ipv4Suspect = Node{true, 2, ipV4(0x05, 0x06, 0x07, 0x08)}
	ipv6Healthy = Node{false, 5, ipV6(0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F)}
	ipv6Suspect = Node{true, 6, ipV6(0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F)}
)

var (
	ipv4HealthyBytes = hex2Bytes(`00 0001 01020304`)
	ipv4SuspectBytes = hex2Bytes(`02 0002 05060708`)
	ipv6HealthyBytes = hex2Bytes(`01 0005 000102030405060708090A0B0C0D0E0F`)
	ipv6SuspectBytes = hex2Bytes(`03 0006 101112131415161718191A1B1C1D1E1F`)
)
