package influxdb

import "github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"

// basicAuthAfterForwardedHeaders puts Basic Auth after ContextualMiddleware.
// InfluxQL user/password then fill Authorization only when Grafana did not
// forward a user token.
func basicAuthAfterForwardedHeaders(_ httpclient.Options, existing []httpclient.Middleware) []httpclient.Middleware {
	var basic httpclient.Middleware
	contextualIdx := -1
	out := make([]httpclient.Middleware, 0, len(existing))
	for _, m := range existing {
		name := middlewareName(m)
		if name == httpclient.BasicAuthenticationMiddlewareName {
			basic = m
			continue
		}
		if name == httpclient.ContextualMiddlewareName {
			contextualIdx = len(out)
		}
		out = append(out, m)
	}
	if basic == nil {
		return existing
	}
	if contextualIdx == -1 {
		// No forwarding hook to order against; keep the SDK chain as-is.
		return existing
	}
	insertAt := contextualIdx + 1
	rebuilt := make([]httpclient.Middleware, 0, len(out)+1)
	rebuilt = append(rebuilt, out[:insertAt]...)
	rebuilt = append(rebuilt, basic)
	rebuilt = append(rebuilt, out[insertAt:]...)
	return rebuilt
}

func middlewareName(m httpclient.Middleware) string {
	n, ok := m.(httpclient.MiddlewareName)
	if !ok {
		return ""
	}
	return n.MiddlewareName()
}
