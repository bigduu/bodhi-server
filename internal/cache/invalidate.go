package cache

var globalCaches struct {
	credCache  *TTLCache
	routeCache *TTLCache
	quotaCache *TTLCache
}

func RegisterCaches(credCache, routeCache, quotaCache *TTLCache) {
	globalCaches.credCache = credCache
	globalCaches.routeCache = routeCache
	globalCaches.quotaCache = quotaCache
}

func InvalidateCredential(userID, provider string) {
	if globalCaches.credCache != nil {
		globalCaches.credCache.Delete("cred:" + userID + ":" + provider)
	}
}

func InvalidateGroupMembership(userID string) {
	if globalCaches.credCache != nil {
		globalCaches.credCache.Delete("groups:" + userID)
	}
}

func InvalidateRouting() {
	if globalCaches.routeCache != nil {
		globalCaches.routeCache.PurgeAll()
	}
}

func InvalidateQuota(userID string) {
	if globalCaches.quotaCache != nil {
		globalCaches.quotaCache.Delete("quota:" + userID)
	}
}
