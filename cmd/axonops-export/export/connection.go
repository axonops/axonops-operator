/*
© 2026 AxonOps Limited. All rights reserved.

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

package export

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// buildConnectionResources generates the Secret and AxonOpsConnection resources.
func buildConnectionResources(opts *Options) []Resource {
	secretName := opts.ConnectionName + "-api-key"

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: opts.Namespace,
		},
		StringData: map[string]string{
			"api-key": "<REPLACE_WITH_API_KEY>",
		},
	}

	conn := &v1alpha1.AxonOpsConnection{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.axonops.com/v1alpha1",
			Kind:       "AxonOpsConnection",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.ConnectionName,
			Namespace: opts.Namespace,
		},
		Spec: v1alpha1.AxonOpsConnectionSpec{
			OrgID: opts.OrgID,
			APIKeyRef: v1alpha1.AxonOpsSecretKeyRef{
				Name: secretName,
				Key:  "api-key",
			},
			TLSSkipVerify: opts.TLSSkipVerify,
		},
	}

	// Only set non-default values
	if opts.Host != "dash.axonops.cloud" {
		conn.Spec.Host = opts.Host
	}
	if opts.Protocol != "https" {
		conn.Spec.Protocol = opts.Protocol
	}
	// Only emit tokenType when it differs from the operator's auto-detected default,
	// so the generated manifest is minimal for cloud users and explicit for self-hosted.
	if opts.TokenType != "" && opts.TokenType != axonops.DefaultTokenType(opts.Host) {
		conn.Spec.TokenType = opts.TokenType
	}

	return []Resource{
		{Kind: "Secret", Name: secretName, Object: secret},
		{Kind: "AxonOpsConnection", Name: opts.ConnectionName, Object: conn},
	}
}
