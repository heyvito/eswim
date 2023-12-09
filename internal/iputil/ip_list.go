package iputil

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
)

// NoAddressError
var NoAddressErr = fmt.Errorf("no address could be detected")

type IPList []netip.Addr

func (i IPList) Contains(ip netip.Addr) bool {
	if ip.IsLoopback() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}
	return slices.Index(i, ip) != -1
}

func LocalAddresses() (IPList, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("listing interface addresses: %w", err)
	}
	addrIPs := make(IPList, 0, len(addrs))
	for _, v := range addrs {
		if ipNet, ok := v.(*net.IPNet); ok {
			if ip := ipNet.IP.To4(); ip != nil {
				addrIPs = append(addrIPs, netip.AddrFrom4([4]byte(ip)))
			} else {
				addrIPs = append(addrIPs, netip.AddrFrom16([16]byte(ipNet.IP.To16())))
			}
		}
	}

	return addrIPs, nil
}

func ConvertNetIP(in net.IP) netip.Addr {
	if ip4 := in.To4(); ip4 != nil {
		return netip.AddrFrom4([4]byte(ip4[:4]))
	}
	return netip.AddrFrom16([16]byte(in[:16]))
}
