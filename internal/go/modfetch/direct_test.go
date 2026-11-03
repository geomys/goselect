package modfetch

import (
	"slices"
	"sync"
	"testing"

	"geomys.org/goup/internal/go/cfg"
)

func TestProxyListDirect(t *testing.T) {
	defer func(goproxy string) {
		cfg.GOPROXY = goproxy
		proxyOnce.Once = sync.Once{}
	}(cfg.GOPROXY)

	for _, tt := range []struct {
		goproxy string
		want    []string
	}{
		{"direct", []string{"https://proxy.golang.org"}},
		{"https://proxy.golang.org,direct", []string{"https://proxy.golang.org"}},
		{"https://corp.example,direct", []string{"https://corp.example", "https://proxy.golang.org"}},
		{"https://corp.example|direct,https://ignored.example", []string{"https://corp.example", "https://proxy.golang.org"}},
		{"direct,https://ignored.example", []string{"https://proxy.golang.org"}},
		{"https://corp.example,off", []string{"https://corp.example", "off"}},
	} {
		cfg.GOPROXY = tt.goproxy
		proxyOnce.Once = sync.Once{}
		proxyOnce.list, proxyOnce.err = nil, nil
		list, err := proxyList()
		if err != nil {
			t.Errorf("GOPROXY=%s: %v", tt.goproxy, err)
			continue
		}
		var got []string
		for _, p := range list {
			got = append(got, p.url)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("GOPROXY=%s: got %q, want %q", tt.goproxy, got, tt.want)
		}
	}
}
