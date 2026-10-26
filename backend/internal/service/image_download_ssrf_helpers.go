package service

import (
	"context"
	"net"
	"strings"
)

// These helpers are the minimum SSRF-policy dependencies for the image
// downloader. Do not couple image uploads to the (not retained) monitoring
// feature from the upstream repository.
var imageDownloadBlockedNames = map[string]struct{}{
	"localhost":                  {},
	"localhost.localdomain":      {},
	"metadata":                   {},
	"metadata.google.internal":   {},
	"metadata.goog":              {},
	"instance-data":              {},
	"instance-data.ec2.internal": {},
}

var imageDownloadAdditionalBlockedRanges = func() []*net.IPNet {
	ranges := []string{"100.64.0.0/10", "0.0.0.0/8"}
	out := make([]*net.IPNet, 0, len(ranges))
	for _, value := range ranges {
		_, cidr, err := net.ParseCIDR(value)
		if err != nil {
			panic("invalid static image download CIDR: " + value)
		}
		out = append(out, cidr)
	}
	return out
}()

func isBlockedHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return true
	}
	_, blocked := imageDownloadBlockedNames[host]
	return blocked
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return true
	}
	for _, cidr := range imageDownloadAdditionalBlockedRanges {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// Reject the whole hostname when any resolved A/AAAA destination is private;
// proxy resolution cannot otherwise enforce DNS rebinding protections.
func isPrivateOrLoopbackHost(ctx context.Context, hostname string) (bool, error) {
	hostname = strings.TrimSuffix(strings.TrimSpace(hostname), ".")
	if isBlockedHostname(hostname) {
		return true, nil
	}
	if ip := net.ParseIP(stripIPv6Zone(hostname)); ip != nil {
		return isPrivateIP(ip), nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return false, err
	}
	if len(addresses) == 0 {
		return true, nil
	}
	for _, entry := range addresses {
		if isPrivateIP(entry.IP) {
			return true, nil
		}
	}
	return false, nil
}
