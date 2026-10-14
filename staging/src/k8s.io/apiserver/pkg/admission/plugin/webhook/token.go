package webhook

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/transport"
)

type cachingTokenSource struct {
	hook         WebhookAccessor
	groupversion string
	saName       string
	namespace    string
}

func NewWebhookTokenSource(hook WebhookAccessor, namespace, saName, groupversion string) transport.ResettableTokenSource {
	return transport.NewCachedTokenSource(&cachingTokenSource{
		hook:         hook,
		saName:       saName,
		groupversion: groupversion,
		namespace:    namespace,
	})
}

func (ts *cachingTokenSource) Token() (*oauth2.Token, error) {
	client := ts.hook.GetKubeClient()

	var cfgKind string
	t := ts.hook.GetType()
	switch t {
	case "validate":
		cfgKind = "ValidatingWebhookConfiguration"
	case "admit":
		cfgKind = "MutatingWebhookConfiguration"
	default:
		return nil, fmt.Errorf("invalid hook type: %v", t)
	}

	tkn, err := client.CoreV1().ServiceAccounts(ts.namespace).CreateToken(context.TODO(), ts.saName, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences: []string{ts.hook.GetAudience()},
			BoundObjectRef: &authenticationv1.BoundObjectReference{
				Kind:       cfgKind,
				APIVersion: "admissionregistration.k8s.io/v1",
				Name:       ts.hook.GetConfigurationName(),
				UID:        ts.hook.GetConfigurationUID(),
			},
			Attestations: map[string]authenticationv1.AttestationValue{
				authenticationv1.AttestationAdmissionReviewAPIGroups: authenticationv1.AttestationValue{ts.groupversion},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		return nil, err
	}

	exp := tkn.Status.ExpirationTimestamp
	now := time.Now()

	return &oauth2.Token{
		AccessToken: tkn.Status.Token,
		TokenType:   "Bearer",
		Expiry:      exp.Time,
		ExpiresIn:   int64(exp.Sub(now)),
	}, nil
}
