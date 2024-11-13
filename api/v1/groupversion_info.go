// Package v1 contains API Schema definitions for the github-actions-runner v1 API group
// +kubebuilder:object:generate=true
// +groupName=github-actions-runner.kaidotdev.github.io
package v1

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

func apiGroup() string {
	defaultGroup := "github-actions-runner.kaidotdev.github.io"
	if v, ok := os.LookupEnv("VARIANT"); ok {
		return fmt.Sprintf("%s.%s", v, defaultGroup)
	}
	return defaultGroup
}

var (
	// GroupVersion is a group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: apiGroup(), Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&Runner{}, &RunnerList{})
}
