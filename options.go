package eswim

import (
	"fmt"
	"github.com/heyvito/gateway"
	"go.uber.org/zap"
	"math"
	"net/netip"
	"time"
)

// MustIPAddr parses a given IP address and returns a pointer to its netip.Addr
// representation. This can be used with HostAddresses struct.
// Panics in case the address cannot be parsed.
func MustIPAddr(addr string) *netip.Addr {
	a := netip.MustParseAddr(addr)
	return &a
}

var IgnoreFamilyAddr = &netip.Addr{}

// HostAddresses defines the set of IP addresses the library will use to
// operate. Those IPs are used to provide TCP and UDP listeners, and will also
// be used to announce this instance in the cluster. When an address has a nil
// value, the library tries to automatically pick an address corresponding to
// that family that contains a default gateway configured. In case a given
// family must be disabled, its value must be set to IgnoreFamilyAddr. The
// library provides the MustIPAddr function to return an address from a string.
type HostAddresses struct {
	IPv4Address *netip.Addr
	IPv6Address *netip.Addr
}

// Options represents a set of options to tuning and configuring the current
// server.
type Options struct {
	// HostAddresses represent the set of addresses the library will use to
	// communicate and advertise. See HostAddresses (struct) for more details.
	HostAddresses HostAddresses

	// IPv4MulticastAddress represents the Multicast IPv4 address to use when
	// announcing or listening for bootstrap requests. An empty string disables
	// IPv4. Defaults to a blank string.
	IPv4MulticastAddress string

	// IPv6MulticastAddress represents the Multicast IPv6 address to use when
	// announcing or listening for bootstrap requests. An empty string disables
	// IPv6. Defaults to a blank string.
	IPv6MulticastAddress string

	// MulticastPort represents the port this and other nodes will use to
	// exchange bootstrap requests. Required.
	MulticastPort uint16

	// SWIMPort represents the port this node will use on both UDP and TCP
	// protocols to exchange information with other nodes. Required.
	SWIMPort uint16

	// CryptoKey comprises a 16-byte cryptographic key shared among all other
	// nodes to secure communication between them. Under development
	// environments, this can be left empty as long as InsecureDisableCrypto is
	// set. Optional. Defaults to an empty byte slice.
	CryptoKey []byte

	// InsecureDisableCrypto disables cryptographic facilities on this instance,
	// effectively making all exchanged messages readable by third-parties. When
	// this is set, the value of CryptoKey is ignored. This MUST NOT BE USED IN
	// PRODUCTION. Defaults to false.
	InsecureDisableCrypto bool

	// IndirectPings determines how many nodes (at most) must be used to perform
	// indirect pings. Defaults to 2.
	IndirectPings int

	// ProtocolPeriod defines the period between protocol periods on this node.
	// This value must be consistent across other nodes, but a jitter is added
	// in order to prevent synchronization. Defaults to 2 seconds.
	ProtocolPeriod time.Duration

	// UseAdaptivePingTimeout determines whether the protocol should use an
	// adaptive ping timeout based on observations of RTT times of messages
	// exchanged with other nodes. The timeout will be set to three times the
	// 99th percentile value of observed RTTs. When enabled, any value defined
	// by PingTimeout is ignored. Defaults to false.
	UseAdaptivePingTimeout bool

	// PingTimeout defines the amount of time to wait until assuming that a
	// node is potentially at fault. In case a node does not answer a ping
	// before this duration expires, it will automatically be considered
	// suspect, and this information will be transmitted to other members of
	// the current cluster. Defaults to half of ProtocolPeriod.
	PingTimeout time.Duration

	// SuspectPeriods determines for how many protocol periods a given node can
	// stay as suspect. Note that a protocol period is also configured by
	// ProtocolPeriod, effectively yielding a value of ProtocolPeriod *
	// SuspectPeriods time units before notifying the cluster the node
	// transitioned to a fault state. Defaults to 20.
	SuspectPeriods uint16

	// FullSyncProtocolRounds determines how many protocol rounds should the
	// server wait between performing a full sync with another random node.
	// A recommended value is around 300 rounds, depending on the chosen
	// ProtocolPeriod. Defaults to 300. Setting this value to -1 disables this
	// mechanism (not recommended).
	FullSyncProtocolRounds int

	// UDPMaxLength determines the maximum length of any given UDP packet. For
	// most networks, a value of 1000~1200 is considered safe. Defaults to 1200.
	UDPMaxLength int

	// LogHandler represents a slog-compatible logger that will be used by this
	// library. A nil logger will emit no messages. Defaults to a noop
	// slog.Logger, which will discard all messages.
	LogHandler *zap.Logger

	// MaxEventTransmission represents a function responsible for calculating
	// how many times at most a given event should be transmitted, based on the
	// size of the current member list. After messages are transmitted that
	// amount of times, it is discarded from the local buffer. Defaults to
	// 3⌈max(log(N + 1), 1)⌉.
	MaxEventTransmission func(clusterSize int) int
}

func (o *Options) normalize() error {
	if !o.InsecureDisableCrypto && len(o.CryptoKey) != 16 {
		return fmt.Errorf("CryptoKey must have 16 bytes")
	}

	if o.ProtocolPeriod == 0 {
		o.ProtocolPeriod = 2 * time.Second
	}

	if o.PingTimeout == 0 {
		o.PingTimeout = o.ProtocolPeriod / 2
	}

	if o.UDPMaxLength == 0 {
		o.UDPMaxLength = 1200
	}

	if o.IndirectPings == 0 {
		o.IndirectPings = 2
	}

	if o.SuspectPeriods == 0 {
		o.SuspectPeriods = 20
	}

	if o.FullSyncProtocolRounds == 0 {
		o.FullSyncProtocolRounds = 300
	}

	if o.MaxEventTransmission == nil {
		o.MaxEventTransmission = func(n int) int {
			return 3 * int(math.Ceil(max(math.Log(float64(n)+1), 1)))
		}
	}

	if o.LogHandler == nil {
		o.LogHandler = zap.NewNop()
	}

	if o.HostAddresses.IPv4Address == nil || o.HostAddresses.IPv6Address == nil {
		allIPs, err := gateway.FindDefaultIPs()
		if err != nil {
			return fmt.Errorf("failed detecting IP addresses: %w", err)
		}

		if o.HostAddresses.IPv4Address == nil {
			o.HostAddresses.IPv4Address = filterIP(allIPs, netip.Addr.Is4)
		}

		if o.HostAddresses.IPv6Address == nil {
			o.HostAddresses.IPv6Address = filterIP(allIPs, netip.Addr.Is6)
		}
	}

	return nil
}

func filterIP(list []netip.Addr, filter func(netip.Addr) bool) *netip.Addr {
	var anyLocal, anyNonLocal netip.Addr

	for _, ip := range list {
		isLinkLocal := ip.IsLinkLocalUnicast() || ip.IsLinkLocalUnicast()
		if !filter(ip) {
			continue
		}

		if isLinkLocal && !anyLocal.IsValid() {
			anyLocal = ip
		} else if !anyNonLocal.IsValid() {
			anyNonLocal = ip
		}

		if anyLocal.IsValid() && anyNonLocal.IsValid() {
			break
		}
	}

	if anyNonLocal.IsValid() {
		return &anyLocal
	} else if anyNonLocal.IsValid() {
		return &anyNonLocal
	}
	return nil
}
