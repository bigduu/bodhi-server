package proxy

import (
	"net/http"
	"strings"
)

func copyProviderRequestHeaders(dst, src http.Header, provider string) {
	hop := connectionHeaders(src)
	for name, values := range src {
		name = strings.ToLower(name)
		if hop[name] {
			continue
		}
		if blockedClientHeader(name) || (provider != "anthropic" &&
			(name == "anthropic-version" || name == "anthropic-beta")) {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

func blockedClientHeader(name string) bool {
	switch name {
	case "authorization", "x-api-key", "api-key", "x-goog-api-key", "x-litellm-api-key",
		"cookie", "cookie2", "host", "content-length", "accept-encoding",
		"x-user-id", "x-username", "x-api-key-id", "x-key-allowed-models", "x-key-allowed-providers",
		"forwarded", "x-real-ip", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-forwarded-port",
		"x-http-method-override", "x-http-method", "x-method-override", "x-original-url", "x-rewrite-url":
		return true
	default:
		return false
	}
}

func copyProviderResponseHeaders(dst, src http.Header, reframed bool) {
	hop := connectionHeaders(src)
	for name, values := range src {
		name = strings.ToLower(name)
		if hop[name] {
			continue
		}
		switch name {
		case "content-length", "content-encoding", "etag", "content-md5", "digest", "content-digest":
			if reframed {
				continue
			}
		case "authorization", "x-api-key", "api-key", "x-goog-api-key", "x-litellm-api-key",
			"x-user-id", "x-username", "x-api-key-id", "x-key-allowed-models", "x-key-allowed-providers",
			"set-cookie", "set-cookie2", "server", "access-control-allow-origin",
			"access-control-allow-credentials", "access-control-expose-headers":
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

func connectionHeaders(headers http.Header) map[string]bool {
	names := map[string]bool{
		"connection": true, "keep-alive": true, "proxy-connection": true,
		"proxy-authenticate": true, "proxy-authorization": true,
		"te": true, "trailer": true, "transfer-encoding": true, "upgrade": true,
	}
	for _, value := range headers.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			names[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	return names
}
