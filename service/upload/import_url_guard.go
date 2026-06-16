package upload

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var importURLHTTPClient = &http.Client{
	Timeout:   importURLTimeout,
	Transport: newImportURLHTTPTransport(),
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("资源地址重定向次数过多")
		}
		return validateImportURL(req.URL)
	},
}

var importURLDialer = &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 30 * time.Second,
}

func newImportURLHTTPTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport := base.Clone()
		transport.DialContext = dialAllowedImportURLAddress
		transport.TLSHandshakeTimeout = 10 * time.Second
		transport.ResponseHeaderTimeout = 30 * time.Second
		transport.ExpectContinueTimeout = 1 * time.Second
		return transport
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialAllowedImportURLAddress,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func validateImportURL(parsed *url.URL) error {
	if parsed == nil {
		return fmt.Errorf("资源地址无效")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("资源地址仅支持 http/https")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("资源地址缺少主机")
	}
	if _, err := resolveAllowedImportURLAddrs(context.Background(), host); err != nil {
		return err
	}
	return nil
}

func dialAllowedImportURLAddress(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("资源地址无效")
	}
	addrs, err := resolveAllowedImportURLAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := importURLDialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("资源地址无可用网络地址")
}

func resolveAllowedImportURLAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	switch normalized {
	case "", "localhost", "localhost.localdomain":
		return nil, fmt.Errorf("资源地址不允许访问内网或本机")
	}
	if addr, err := netip.ParseAddr(normalized); err == nil {
		if isBlockedImportAddr(addr) {
			return nil, fmt.Errorf("资源地址不允许访问内网或本机")
		}
		return []netip.Addr{addr}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", normalized)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("资源地址解析失败")
	}
	for _, addr := range addrs {
		if isBlockedImportAddr(addr) {
			return nil, fmt.Errorf("资源地址不允许访问内网或本机")
		}
	}
	return addrs, nil
}

func isBlockedImportAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	if addr.Is4() {
		ipv4 := addr.As4()
		if ipv4[0] == 169 && ipv4[1] == 254 {
			return true
		}
	}
	return false
}
