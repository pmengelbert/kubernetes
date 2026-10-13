package webhook

import (
	"context"
	"time"

	"golang.org/x/oauth2"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/transport"
)

type webhookAuthenticationTokenSource struct {
	src func() (*oauth2.Token, error)
}

func (ts *webhookAuthenticationTokenSource) Token() (*oauth2.Token, error) {
	return ts.src()
}

func NewWebhookAuthenticationTokenSource(ctx context.Context, cl kubernetes.Interface, cm *ClientManager, aud, name string, uid types.UID) transport.ResettableTokenSource {
	return transport.NewCachedTokenSource(&webhookAuthenticationTokenSource{
		src: func() (*oauth2.Token, error) {
			tokenRequest, err := cl.CoreV1().ServiceAccounts("kube-system").CreateToken(ctx, "webhook-auth", &authenticationv1.TokenRequest{
				Spec: authenticationv1.TokenRequestSpec{
					Audiences:         []string{aud},
					ExpirationSeconds: nil,
					BoundObjectRef: &authenticationv1.BoundObjectReference{
						Kind:       "ValidatingWebhookConfiguration",
						Name:       name,
						UID:        uid,
						APIVersion: "admissionregistration.k8s.io/v1",
					},
					Attestations: map[string]authenticationv1.AttestationValue{
						authenticationv1.AttestationAdmissionReviewAPIGroups: {"*"},
					},
				},
			}, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}

			now := time.Now()
			ts := tokenRequest.Status.ExpirationTimestamp.Time

			return &oauth2.Token{
				AccessToken: tokenRequest.Status.Token,
				TokenType:   "Bearer",
				Expiry:      tokenRequest.Status.ExpirationTimestamp.Time,
				ExpiresIn:   int64(ts.Sub(now)),
			}, nil
		},
	})
}

// func keyFunc(tr *authenticationv1.TokenRequest) string {
// 	if tr.Spec.ExpirationSeconds != nil {
// 		exp = *tr.Spec.ExpirationSeconds
// 	}

// 	var ref authenticationv1.BoundObjectReference
// 	if tr.Spec.BoundObjectRef != nil {
// 		ref = *tr.Spec.BoundObjectRef
// 	}
// }

// func (m *webhookAuthenticationTokenSource) GetServiceAccountToken(namespace, name string, tr *authenticationv1.TokenRequest) (*authenticationv1.TokenRequest, error) {
// 	// TODO: pass ctx to GetServiceAccountToken after switching pkg/volume to contextual logging
// 	ctx := context.TODO()
// 	key := keyFunc(tr)

// 	ctr, ok := m.get(key)

// 	if ok && !m.requiresRefresh(ctx, ctr) {
// 		return ctr, nil
// 	}

// 	tr, err := m.getToken(name, namespace, tr)
// 	if err != nil {
// 		switch {
// 		case !ok:
// 			return nil, fmt.Errorf("failed to fetch token: %v", err)
// 		case m.expired(ctr):
// 			return nil, fmt.Errorf("token %s expired and refresh failed: %v", key, err)
// 		default:
// 			logger := klog.FromContext(ctx)
// 			logger.Error(err, "Couldn't update token", "cacheKey", key)
// 			return ctr, nil
// 		}
// 	}

// 	m.set(key, tr)
// 	return tr, nil
// }

// // requiresRefresh returns true if the token is older than 80% of its total
// // ttl, or if the token is older than 24 hours.
// func (m *webhookAuthenticationTokenSource) requiresRefresh(ctx context.Context, tr *authenticationv1.TokenRequest) bool {
// 	if tr.Spec.ExpirationSeconds == nil {
// 		cpy := tr.DeepCopy()
// 		cpy.Status.Token = ""
// 		logger := klog.FromContext(ctx)
// 		logger.Error(nil, "Expiration seconds was nil for token request", "tokenRequest", cpy)
// 		return false
// 	}
// 	now := time.Now()
// 	exp := tr.Status.ExpirationTimestamp.Time
// 	iat := exp.Add(-1 * time.Duration(*tr.Spec.ExpirationSeconds) * time.Second)

// 	jitter := time.Duration(rand.Float64()*maxJitter.Seconds()) * time.Second
// 	if now.After(iat.Add(maxTTL - jitter)) {
// 		return true
// 	}
// 	// Require a refresh if within 20% of the TTL plus a jitter from the expiration time.
// 	if now.After(exp.Add(-1*time.Duration((*tr.Spec.ExpirationSeconds*20)/100)*time.Second - jitter)) {
// 		return true
// 	}
// 	return false
// }
