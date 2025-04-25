// Copyright OpCore

package dialer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"
)

const (
	defaultRateLimit      = 50
	defaultCacheTTL       = 10 * time.Second
	defaultConnectTimeout = time.Second
	defaultDialerTimeout  = 5 * time.Second
	defaultKeepAlive      = 1 * time.Second
)

// Dialer - common interface for dialer.
type Dialer interface {
	// String - dialer name.
	String() string
	// Dial - connects to the address on the named network, overridden by wrappers.
	Dial(ctx context.Context) (net.Conn, error)
	// DialHost - open a connection to the specified host, used by wrappers and cannot be overridden.
	DialHost(ctx context.Context, hostname, port string) (net.Conn, error)
	// Equal check for complete matching of configs.
	Equal(config any) bool
}

//
// CommonConfig
//

// CommonConfig full config for config dialer.
type CommonConfig struct {
	tlsConfig *tls.Config
	url       url.URL
	name      string
}

// NewCommonConfig init new CommonConfig.
func NewCommonConfig(urlp *url.URL, tlsConfig *tls.Config, name string) (*CommonConfig, error) {
	switch urlp.Scheme {
	case "http":
		return &CommonConfig{url: *urlp, name: name}, nil
	case "https":
		if tlsConfig == nil {
			return nil, errors.New("tls config empty for 'https'")
		}

		return &CommonConfig{url: *urlp, name: name, tlsConfig: tlsConfig.Clone()}, nil
	default:
		return nil, fmt.Errorf("unsupported url.Scheme: %s", urlp.Scheme)
	}
}

// Equal check for complete matching of configs.
func (c *CommonConfig) Equal(cfg *CommonConfig) bool {
	if cfg == nil {
		return false
	}

	if c.name != cfg.name {
		return false
	}

	if c.url != cfg.url {
		return false
	}

	if c.tlsConfig == nil && cfg.tlsConfig == nil {
		return true
	}

	if (c.tlsConfig == nil && cfg.tlsConfig != nil) || (c.tlsConfig != nil && cfg.tlsConfig == nil) {
		return false
	}

	if c.tlsConfig.ServerName != cfg.tlsConfig.ServerName {
		return false
	}

	if c.tlsConfig.MinVersion != cfg.tlsConfig.MinVersion {
		return false
	}

	return true
}

// Name return name from config.
func (c *CommonConfig) Name() string {
	return c.name
}

// Host return host from config.
func (c *CommonConfig) Host() string {
	return c.url.Host
}

// Hostname return hostname from config.
func (c *CommonConfig) Hostname() string {
	return c.url.Hostname()
}

// Port return port from config.
func (c *CommonConfig) Port() string {
	return c.url.Port()
}

//
// DefaultDialer
//

// DefaultDialer - init RandomTLSDialer with default values.
func DefaultDialer(cfg *CommonConfig, registerer prometheus.Registerer) (Dialer, error) {
	return NewRandomTLSDialer(
		//nolint:gosec // cryptographic strength is not required
		rand.New(rand.NewSource(time.Now().UnixNano())),
		Resolve,
		&net.Dialer{Timeout: defaultDialerTimeout, KeepAlive: defaultKeepAlive},
		cfg,
	)
}

//
// RandomTLSDialer
//

// RandomTLSDialer - connecting to a random ip mapped to a host.
type RandomTLSDialer struct {
	rand     *rand.Rand
	mxRand   *sync.Mutex
	dialer   *net.Dialer
	resolver func(ctx context.Context, host string) ([]net.IP, error)
	cfg      CommonConfig
}

var _ Dialer = (*RandomTLSDialer)(nil)

// NewRandomTLSDialer - init new RandomTLSDialer.
func NewRandomTLSDialer(
	random *rand.Rand,
	resolver func(ctx context.Context, host string) ([]net.IP, error),
	dialer *net.Dialer,
	cfg *CommonConfig,
) (*RandomTLSDialer, error) {
	if cfg == nil {
		return nil, errors.New("empty common config")
	}

	return &RandomTLSDialer{
		rand:     random,
		mxRand:   new(sync.Mutex),
		dialer:   dialer,
		resolver: resolver,
		cfg:      *cfg,
	}, nil
}

// Equal check for complete matching of configs.
func (rd *RandomTLSDialer) Equal(cfg any) bool {
	cc, ok := cfg.(*CommonConfig)
	if !ok {
		return false
	}

	return rd.cfg.Equal(cc)
}

// String - dialer name.
func (rd *RandomTLSDialer) String() string {
	return fmt.Sprintf("%s_%s", rd.cfg.Name(), rd.cfg.Host())
}

// Dial - connects to the collector.
func (rd *RandomTLSDialer) Dial(ctx context.Context) (net.Conn, error) {
	return rd.DialHost(ctx, rd.cfg.Hostname(), rd.cfg.Port())
}

// DialHost - open a connection to the specified host.
func (rd *RandomTLSDialer) DialHost(ctx context.Context, hostname, port string) (net.Conn, error) {
	ips, err := rd.resolver(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q is not resolved", hostname)
	}
	rd.mxRand.Lock()
	ip := ips[rd.rand.Intn(len(ips))]
	rd.mxRand.Unlock()

	conn, err := rd.dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%s", ip, port))
	if err != nil {
		return nil, fmt.Errorf("dial to %q (%s): %w", hostname, ip, err)
	}

	if rd.cfg.tlsConfig == nil {
		return conn, nil
	}

	tlsConn := tls.Client(conn, rd.cfg.tlsConfig)
	// in some situations, providers cut off TLS traffic, apparently due to DPI,
	// so we immediately check that we can install handshake and
	// accordingly, we can use a proxy.
	ctx, cancel := context.WithTimeout(ctx, rd.dialer.Timeout)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("handshake to %s: %w", rd.cfg.tlsConfig.ServerName, err)
	}
	cancel()
	return tlsConn, nil
}

//
// Utils
//

// resolverCache cache map with resolving name to ips.
var resolverCache = new(sync.Map)

// addrsTTL cache value with ttl and ip.
type addrsTTL struct {
	ts    *time.Timer
	addrs []net.IP
}

// getFromCache return ips for hostname from cache(if exist).
func getFromCache(host string) ([]net.IP, bool) {
	addrs, ok := resolverCache.Load(host)
	if !ok {
		return nil, false
	}

	return addrs.(*addrsTTL).addrs, true
}

// addToCache add ips for hostname to cache.
func addToCache(host string, addrs []net.IP) {
	val := &addrsTTL{addrs: addrs}

	_, loaded := resolverCache.LoadOrStore(host, val)
	if loaded {
		return
	}

	val.ts = time.AfterFunc(
		defaultCacheTTL,
		func() { removeFromCache(host) },
	)
}

// removeFromCache remove hostname from cache.
func removeFromCache(host string) {
	resolverCache.Delete(host)
}

// rl rate limmiter for resolving hostname.
var rl = rate.NewLimiter(rate.Limit(defaultRateLimit), defaultRateLimit)

// Resolve - host resolution using the standard mechanism and in parallel in Google and Yandex.
func Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// try the standard resolver
	ips, ok := getFromCache(host)
	if ok {
		return ips, nil
	}

	if err := rl.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	defaultCtx, defaultCancel := context.WithTimeout(ctx, defaultConnectTimeout)
	addrs, _ := net.DefaultResolver.LookupIPAddr(defaultCtx, host)
	defaultCancel()
	if len(addrs) > 0 {
		ips = make([]net.IP, len(addrs))
		for i, ia := range addrs {
			ips[i] = ia.IP
		}
		addToCache(host, ips)
		return ips, nil
	}

	ips, ok = getFromCache(host)
	if ok {
		return ips, nil
	}

	ips, err := fallbackResolve(ctx, host)
	if err != nil {
		return nil, err
	}

	if len(addrs) > 0 {
		addToCache(host, ips)
	}

	return ips, err
}

// namedResolver - resolver with name.
type namedResolver struct {
	*net.Resolver
	name string
}

// customResolver - custom resolver.
func customResolver(server string) namedResolver {
	address := fmt.Sprintf("%s:53", server)
	return namedResolver{
		Resolver: &net.Resolver{
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, network, address)
			},
		},
		name: server,
	}
}

// list of public DNS servers.
var resolvers = [...]namedResolver{
	customResolver("8.8.8.8"),   // dns google
	customResolver("77.88.8.8"), // dns yandex
	customResolver("77.88.8.1"), // secondary dns yandex
}

// fallbackResolve - fallback resolve with public DNS servers.
func fallbackResolve(ctx context.Context, host string) (ips []net.IP, err error) {
	// trying public DNS servers
	type Result struct {
		err error
		ips []net.IPAddr
	}

	counter := atomic.NewInt64(0)
	results := make(chan Result, len(resolvers))
	ctx, cancel := context.WithTimeout(ctx, defaultConnectTimeout)
	for _, resolver := range resolvers {
		go func(resolver namedResolver) {
			ips, rErr := resolver.LookupIPAddr(ctx, host)
			if rErr != nil {
				rErr = fmt.Errorf("%s: %w", resolver.name, rErr)
			}
			results <- Result{rErr, ips}
			if int(counter.Add(1)) == len(resolvers) {
				close(results)
			}
		}(resolver)
	}
	for res := range results {
		if len(res.ips) > 0 {
			ips = make([]net.IP, len(res.ips))
			for i, ia := range res.ips {
				ips[i] = ia.IP
			}
			cancel()
			return ips, nil
		}

		err = errors.Join(err, res.err)
	}
	cancel()
	return nil, err
}
