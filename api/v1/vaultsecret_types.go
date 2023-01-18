/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VaultSecretSpec defines the desired state of VaultSecret
type VaultSecretSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Addr             string              `json:"addr,omitempty" yaml:"addr"`
	Separator        string              `json:"separator,omitempty" yaml:"separator"`
	Paths            []VaultSecretPath   `json:"paths" yaml:"paths"`
	TargetSecretName string              `json:"targetSecretName,omitempty" yaml:"targetSecretName"`
	TargetFormat     string              `json:"targetFormat,omitempty" yaml:"targetFormat"`
	ReconcilePeriod  string              `json:"reconcilePeriod,omitempty" yaml:"reconcilePeriod,omitempty"`
	Auth             VaultSecretAuthSpec `json:"auth,omitempty" yaml:"auth"`
}

func (in *VaultSecretSpec) GetSeparator() string {
	if in.Separator == "" {
		return "_"
	}
	return in.Separator
}

// VaultSecretAuthSpec defines the desired state of VaultSecretAuth
type VaultSecretAuthSpec struct {
	ServiceAccountRef *VaultSecretAuthServiceAccountRefSpec `json:"serviceAccountRef,omitempty" yaml:"serviceAccountRef"`
	Token             string                                `json:"token,omitempty" yaml:"token"`
}

// VaultSecretPath defines the desired state of VaultSecretPath
type VaultSecretPath struct {
	Path   string `json:"path" yaml:"path"`
	Prefix string `json:"prefix,omitempty" yaml:"prefix"`
}

// VaultSecretStatus defines the observed state of VaultSecret
type VaultSecretStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LastUpdated string `json:"lastUpdated" yaml:"lastUpdated"`
}

// VaultSecretAuthServiceAccountRefSpec defines the desired state of VaultSecretAuthTokenRef
type VaultSecretAuthServiceAccountRefSpec struct {
	Name     string `json:"name,omitempty" yaml:"name"`
	AuthPath string `json:"authPath,omitempty" yaml:"authPath"`
	Role     string `json:"role,omitempty" yaml:"role"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VaultSecret is the Schema for the vaultsecrets API
type VaultSecret struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   VaultSecretSpec   `json:"spec,omitempty" yaml:"spec"`
	Status VaultSecretStatus `json:"status,omitempty" yaml:"status"`
}

//+kubebuilder:object:root=true

// VaultSecretList contains a list of VaultSecret
type VaultSecretList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata"`
	Items           []VaultSecret `json:"items" yaml:"items"`
}

func init() {
	SchemeBuilder.Register(&VaultSecret{}, &VaultSecretList{})
}
