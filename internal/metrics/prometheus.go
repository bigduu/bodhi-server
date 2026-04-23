package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	proxyRequests   sync.Map // key -> *atomic.Int64
	proxyInputTok   sync.Map // key -> *atomic.Int64
	proxyOutputTok  sync.Map // key -> *atomic.Int64
	proxyDuration   sync.Map // key -> *durationBucket
	proxyCacheHits  atomic.Int64
	proxyCacheMiss  atomic.Int64
)

type durationBucket struct {
	sum   atomic.Int64 // milliseconds
	count atomic.Int64
}

func metricKey(provider, model, status string) string {
	return provider + "|" + model + "|" + status
}

func IncProxyRequests(provider, model string, statusCode int) {
	key := metricKey(provider, model, fmt.Sprintf("%d", statusCode))
	val, _ := proxyRequests.LoadOrStore(key, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
}

func IncProxyTokens(provider, model string, input, output int) {
	iKey := provider + "|" + model + "|input"
	oKey := provider + "|" + model + "|output"

	val, _ := proxyInputTok.LoadOrStore(iKey, &atomic.Int64{})
	val.(*atomic.Int64).Add(int64(input))

	val, _ = proxyOutputTok.LoadOrStore(oKey, &atomic.Int64{})
	val.(*atomic.Int64).Add(int64(output))
}

func ObserveDuration(provider string, ms int) {
	b, _ := proxyDuration.LoadOrStore(provider, &durationBucket{})
	db := b.(*durationBucket)
	db.sum.Add(int64(ms))
	db.count.Add(1)
}

func IncCacheHit()  { proxyCacheHits.Add(1) }
func IncCacheMiss() { proxyCacheMiss.Add(1) }

func ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	var sb strings.Builder
	sb.WriteString("# HELP proxy_requests_total Total proxy requests by provider, model, status\n")
	sb.WriteString("# TYPE proxy_requests_total counter\n")

	proxyRequests.Range(func(key, val interface{}) bool {
		parts := strings.Split(key.(string), "|")
		sb.WriteString(fmt.Sprintf("proxy_requests_total{provider=%q,model=%q,status=%q} %d\n",
			parts[0], parts[1], parts[2], val.(*atomic.Int64).Load()))
		return true
	})

	sb.WriteString("\n# HELP proxy_tokens_input_total Total input tokens by provider and model\n")
	sb.WriteString("# TYPE proxy_tokens_input_total counter\n")
	proxyInputTok.Range(func(key, val interface{}) bool {
		parts := strings.Split(key.(string), "|")
		sb.WriteString(fmt.Sprintf("proxy_tokens_input_total{provider=%q,model=%q} %d\n",
			parts[0], parts[1], val.(*atomic.Int64).Load()))
		return true
	})

	sb.WriteString("\n# HELP proxy_tokens_output_total Total output tokens by provider and model\n")
	sb.WriteString("# TYPE proxy_tokens_output_total counter\n")
	proxyOutputTok.Range(func(key, val interface{}) bool {
		parts := strings.Split(key.(string), "|")
		sb.WriteString(fmt.Sprintf("proxy_tokens_output_total{provider=%q,model=%q} %d\n",
			parts[0], parts[1], val.(*atomic.Int64).Load()))
		return true
	})

	sb.WriteString("\n# HELP proxy_duration_milliseconds_total Total request duration in ms by provider\n")
	sb.WriteString("# TYPE proxy_duration_milliseconds_total counter\n")
	proxyDuration.Range(func(key, val interface{}) bool {
		db := val.(*durationBucket)
		sb.WriteString(fmt.Sprintf("proxy_duration_milliseconds_total{provider=%q} %d\n",
			key.(string), db.sum.Load()))
		return true
	})

	sb.WriteString("\n# HELP proxy_duration_count_total Number of observed durations by provider\n")
	sb.WriteString("# TYPE proxy_duration_count_total counter\n")
	proxyDuration.Range(func(key, val interface{}) bool {
		db := val.(*durationBucket)
		sb.WriteString(fmt.Sprintf("proxy_duration_count_total{provider=%q} %d\n",
			key.(string), db.count.Load()))
		return true
	})

	sb.WriteString(fmt.Sprintf("\nproxy_cache_hits_total %d\n", proxyCacheHits.Load()))
	sb.WriteString(fmt.Sprintf("proxy_cache_misses_total %d\n", proxyCacheMiss.Load()))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(sb.String()))
}
