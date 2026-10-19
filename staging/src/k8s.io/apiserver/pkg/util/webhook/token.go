package webhook

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilcache "k8s.io/apimachinery/pkg/util/cache"
)

const (
	defaultCacheSize = 200
)

type TokenSourceGetter interface {
	Getter(ta TokenAttributes) oauth2.TokenSource
}

func NewTokenCacheWithTTL(ttl time.Duration) *tokenCache {
	return &tokenCache{
		LRUExpireCache: utilcache.NewLRUExpireCache(defaultCacheSize),
		ttl:            ttl,
	}
}

func NewTokenCache() *tokenCache {
	return NewTokenCacheWithTTL(7 * time.Minute)
}

type tokenCache struct {
	*utilcache.LRUExpireCache
	ttl time.Duration
}

func (tc *tokenCache) Getter(ta TokenAttributes) oauth2.TokenSource {
	return &cachedTokenSource{
		TokenAttributes: ta,
		cache:           tc.LRUExpireCache,
	}
}

type TokenAttributes struct {
	Hook         WebhookAccessor
	Namespace    string
	SAName       string
	GroupVersion string
}

type cachedTokenSource struct {
	TokenAttributes
	cache *utilcache.LRUExpireCache
}

func (ts *cachedTokenSource) Token() (*oauth2.Token, error) {
	cacheKey := ts.Namespace + "/" + ts.SAName + "/" + ts.GroupVersion

	cachedToken, ok := ts.cache.Get(cacheKey)
	if ok {
		cachedToken, okok := cachedToken.(oauth2.Token)
		if !okok {
			return nil, fmt.Errorf("token stored in cache incorrectly")
		}

		if !cachedToken.Expiry.After(time.Now()) {
			return &cachedToken, nil
		}
	}

	client := ts.Hook.GetKubeClient()

	var cfgKind string
	t := ts.Hook.GetType()
	switch t {
	case "validate":
		cfgKind = "ValidatingWebhookConfiguration"
	case "admit":
		cfgKind = "MutatingWebhookConfiguration"
	default:
		return nil, fmt.Errorf("invalid hook type: %v", t)
	}

	tkn, err := client.CoreV1().ServiceAccounts(ts.Namespace).CreateToken(context.TODO(), ts.SAName, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences: []string{ts.Hook.GetAudience()},
			BoundObjectRef: &authenticationv1.BoundObjectReference{
				Kind:       cfgKind,
				APIVersion: "admissionregistration.k8s.io/v1",
				Name:       ts.Hook.GetConfigurationName(),
				UID:        ts.Hook.GetConfigurationUID(),
			},
			Attestations: map[string]authenticationv1.AttestationValue{
				authenticationv1.AttestationAdmissionReviewAPIGroups: authenticationv1.AttestationValue{ts.GroupVersion},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		return nil, err
	}

	exp := tkn.Status.ExpirationTimestamp
	now := time.Now()
	ret := oauth2.Token{
		AccessToken: tkn.Status.Token,
		TokenType:   "Bearer",
		Expiry:      exp.Time,
		ExpiresIn:   int64(exp.Sub(now)),
	}
	ts.cache.Add(cacheKey, ret, time.Minute*7)

	return &ret, nil
}
