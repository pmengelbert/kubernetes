/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package validation contains methods to validate kinds in the
// authentication.k8s.io API group.
package validation

import (
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/authentication"
)

const MinTokenAgeSec = 10 * 60 // 10 minutes

// ValidateTokenRequest validates a TokenRequest.
func ValidateTokenRequest(tr *authentication.TokenRequest) field.ErrorList {
	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	if tr.Spec.ExpirationSeconds < MinTokenAgeSec {
		allErrs = append(allErrs, field.Invalid(specPath.Child("expirationSeconds"), tr.Spec.ExpirationSeconds, "may not specify a duration less than 10 minutes"))
	}
	if tr.Spec.ExpirationSeconds > 1<<32 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("expirationSeconds"), tr.Spec.ExpirationSeconds, "may not specify a duration larger than 2^32 seconds"))

	}
	allErrs = append(allErrs, validateAttestations(tr.Spec.Attestations, specPath)...)
	return allErrs
}

func validateAttestations(attestations map[string]authentication.AttestationValue, specPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}

	for key, values := range attestations {
		if len(values) == 0 {
			errs = append(errs, field.Invalid(specPath.Child("attestations"), key, "may not specify empty value"))
			continue
		}

		switch key {
		case authentication.AttestationAdmissionReviewAPIGroups:
			if len(values) != 1 {
				errs = append(errs, field.Invalid(specPath.Child("attestations").Child(authentication.AttestationAdmissionReviewAPIGroups), key, "must specify a single value"))
				continue
			}

			if values[0] == "" {
				errs = append(errs, field.Invalid(specPath.Child("attestations").Child(authentication.AttestationAdmissionReviewAPIGroups), key, "may not be an empty string"))
				continue
			}
		default:
			errs = append(errs, field.Invalid(specPath.Child("attestations"), key, "may not specify an unknown key"))
		}
	}

	return errs
}
