package webhook

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilcache "k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// serviceaccount tokens have a max expiration of 10 minutes. this ttl
	// forces a proactive refresh
	defaultTTL = 7 * time.Minute
)

// whAttrAccessor is a subset of the webhook.Accessor interface
type whAttrAccessor interface {
	GetKubeClient() kubernetes.Interface
	GetConfigurationName() string
	GetConfigurationUID() types.UID
	GetType() string
}

// tokenSourceGetter provides a cached token source from a webhook accessor and
// a struct of token attributes
type tokenSourceGetter interface {
	Getter(whAttrAccessor, TokenAttributes) oauth2.TokenSource
}

func NewTokenCache() tokenSourceGetter {
	return NewTokenCacheWithTTL(defaultTTL)
}

func NewTokenCacheWithTTL(ttl time.Duration) tokenSourceGetter {
	return &tokenCache{
		LRUExpireCache: utilcache.NewLRUExpireCache(defaultCacheSize),
		ttl:            ttl,
	}
}

type tokenCache struct {
	*utilcache.LRUExpireCache
	ttl time.Duration
}

func (tc *tokenCache) Getter(a whAttrAccessor, ta TokenAttributes) oauth2.TokenSource {
	return &cachedTokenSource{
		TokenAttributes: ta,
		accessor:        a,
		ttl:             tc.ttl,
		cacheRef:        tc.LRUExpireCache,
	}
}

// TokenAttributes is the list of attributes needed to get a token, and serves
// as the cache key for the token
type TokenAttributes struct {
	Namespace    string
	SAName       string
	GroupVersion string
	Audience     string
}

type cachedTokenSource struct {
	TokenAttributes
	ttl      time.Duration
	accessor whAttrAccessor
	cacheRef *utilcache.LRUExpireCache
}

func (ts *cachedTokenSource) Token() (*oauth2.Token, error) {
	cacheKey := ts.TokenAttributes
	cachedToken, ok := ts.cacheRef.Get(cacheKey)
	if ok {
		cachedToken, okok := cachedToken.(oauth2.Token)
		if !okok {
			return nil, fmt.Errorf("token stored in cache incorrectly")
		}

		now := time.Now()
		if now.Before(cachedToken.Expiry) {
			klog.Warning("aaaFOUND IN CACHE")
			return &cachedToken, nil
		}
		klog.Warning("EXPIRY FOOLERY exp/now", cachedToken.Expiry, now)
	}
	klog.Warning("zzzNOT FOUND IN CACHE")

	client := ts.accessor.GetKubeClient()

	var cfgKind string
	t := ts.accessor.GetType()
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
			Audiences: []string{ts.Audience},
			BoundObjectRef: &authenticationv1.BoundObjectReference{
				Kind:       cfgKind,
				APIVersion: "admissionregistration.k8s.io/v1",
				Name:       ts.accessor.GetConfigurationName(),
				UID:        ts.accessor.GetConfigurationUID(),
			},
			Attestations: map[string]authenticationv1.AttestationValue{
				authenticationv1.AttestationAdmissionReviewAPIGroups: {ts.GroupVersion},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		return nil, err
	}

	exp := tkn.Status.ExpirationTimestamp
	ret := oauth2.Token{
		AccessToken: tkn.Status.Token,
		TokenType:   "Bearer",
		Expiry:      exp.Time,
		ExpiresIn:   int64(time.Until(exp.Time)),
	}
	ts.cacheRef.Add(cacheKey, ret, ts.ttl)

	return &ret, nil
}
