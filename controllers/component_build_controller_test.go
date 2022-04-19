/*
Copyright 2021-2022 Red Hat, Inc.

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

package controllers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	triggersapi "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//+kubebuilder:scaffold:imports
)

const (
	timeout  = time.Second * 10
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

func isOwnedBy(resource []metav1.OwnerReference, component appstudiov1alpha1.Component) bool {
	if len(resource) == 0 {
		return false
	}
	if resource[0].Kind == "Component" &&
		resource[0].APIVersion == "appstudio.redhat.com/v1alpha1" &&
		resource[0].Name == component.Name {
		return true
	}
	return false
}

// Simple function to create, retrieve from k8s, and return a simple Application CR
func createAndFetchSimpleApp(name string, namespace string, display string, description string) *appstudiov1alpha1.Application {
	hasApp := &appstudiov1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appstudiov1alpha1.ApplicationSpec{
			DisplayName: display,
			Description: description,
		},
	}

	Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

	// Look up the has app resource that was created.
	// num(conditions) may still be < 1 on the first try, so retry until at least _some_ condition is set
	hasAppLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
	fetchedHasApp := &appstudiov1alpha1.Application{}
	Eventually(func() bool {
		k8sClient.Get(ctx, hasAppLookupKey, fetchedHasApp)
		return len(fetchedHasApp.Status.Conditions) > 0
	}, timeout, interval).Should(BeTrue())

	return fetchedHasApp
}

// deleteHASAppCR deletes the specified hasApp resource and verifies it was properly deleted
func deleteHASAppCR(hasAppLookupKey types.NamespacedName) {
	// Delete
	Eventually(func() error {
		f := &appstudiov1alpha1.Application{}
		k8sClient.Get(ctx, hasAppLookupKey, f)
		return k8sClient.Delete(ctx, f)
	}, timeout, interval).Should(Succeed())

	// Wait for delete to finish
	Eventually(func() error {
		f := &appstudiov1alpha1.Application{}
		return k8sClient.Get(ctx, hasAppLookupKey, f)
	}, timeout, interval).ShouldNot(Succeed())
}

// deleteHASCompCR deletes the specified hasComp resource and verifies it was properly deleted
func deleteHASCompCR(hasCompLookupKey types.NamespacedName) {
	// Delete
	Eventually(func() error {
		f := &appstudiov1alpha1.Component{}
		k8sClient.Get(ctx, hasCompLookupKey, f)
		return k8sClient.Delete(ctx, f)
	}, timeout, interval).Should(Succeed())

	// Wait for delete to finish
	Eventually(func() error {
		f := &appstudiov1alpha1.Component{}
		return k8sClient.Get(ctx, hasCompLookupKey, f)
	}, timeout, interval).ShouldNot(Succeed())
}

var _ = Describe("Component build controller", func() {
	const (
		HASAppName      = "test-application"
		HASCompName     = "test-component"
		HASAppNamespace = "default"
		DisplayName     = "petclinic"
		Description     = "Simple petclinic app"
		ComponentName   = "backend"
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)

	Context("Test build trigger", func() {
		var (
			// All related to the component resources have the same key (but different type)
			resourceKey    = types.NamespacedName{Name: HASCompName, Namespace: HASAppNamespace}
			createdHasComp *appstudiov1alpha1.Component
		)

		createSampleComponent := func() {
			hasComp := &appstudiov1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      HASCompName,
					Namespace: HASAppNamespace,
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: ComponentName,
					Application:   HASAppName,
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{
								URL: SampleRepoLink,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())
		}

		listCreatedHasCompPipelienRuns := func() *tektonapi.PipelineRunList {
			pipelineRuns := &tektonapi.PipelineRunList{}
			labelSelectors := client.ListOptions{Raw: &metav1.ListOptions{
				LabelSelector: "build.appstudio.openshift.io/component=" + createdHasComp.Name,
			}}
			err := k8sClient.List(ctx, pipelineRuns, &labelSelectors)
			Expect(err).ToNot(HaveOccurred())
			return pipelineRuns
		}

		_ = BeforeEach(func() {
			createAndFetchSimpleApp(HASAppName, HASAppNamespace, DisplayName, Description)
			createSampleComponent()

			createdHasComp = &appstudiov1alpha1.Component{}
			Eventually(func() bool {
				k8sClient.Get(ctx, resourceKey, createdHasComp)
				return createdHasComp.ResourceVersion != ""
			}, timeout, interval).Should(BeTrue())
		}, 30)

		_ = AfterEach(func() {
			deleteHASCompCR(resourceKey)
			deleteHASAppCR(types.NamespacedName{Name: HASAppName, Namespace: HASAppNamespace})
		}, 30)

		checkInitialBuildWasSubmitted := func() {
			// Check that a new TriggerTemplate created
			Eventually(func() bool {
				triggerTemplate := &triggersapi.TriggerTemplate{}
				k8sClient.Get(ctx, resourceKey, triggerTemplate)
				return triggerTemplate.ResourceVersion != "" && isOwnedBy(triggerTemplate.GetOwnerReferences(), *createdHasComp)
			}, timeout, interval).Should(BeTrue())

			// Check that build is submitted
			Eventually(func() bool {
				return len(listCreatedHasCompPipelienRuns().Items) > 0
			}, timeout, interval).Should(BeTrue())
		}

		It("should submit a new build if no trigger template found", func() {
			checkInitialBuildWasSubmitted()
		})

		It("should submit a new build if build parameter changed", func() {
			checkInitialBuildWasSubmitted()

			// Update build parameter in the TriggerTemplate
			triggerTemplate := &triggersapi.TriggerTemplate{}
			err := k8sClient.Get(ctx, resourceKey, triggerTemplate)
			Expect(err).ToNot(HaveOccurred())

			triggerTemplate.Spec.Params[0].Name = "new-param"

			err = k8sClient.Update(ctx, triggerTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Check that a new build is submitted
			Eventually(func() bool {
				return len(listCreatedHasCompPipelienRuns().Items) > 1
			}, timeout, interval).Should(BeTrue())
		})

		It("should submit a new build if build trigger resource template changed", func() {
			checkInitialBuildWasSubmitted()

			// Update TriggerResourceTemplate in the TriggerTemplate
			triggerTemplate := &triggersapi.TriggerTemplate{}
			err := k8sClient.Get(ctx, resourceKey, triggerTemplate)
			Expect(err).ToNot(HaveOccurred())

			rawTriggerResourceTemplate := triggerTemplate.Spec.ResourceTemplates[0].Raw
			var triggerResourceTemplate tektonapi.PipelineRun
			err = json.Unmarshal(rawTriggerResourceTemplate, &triggerResourceTemplate)
			Expect(err).ToNot(HaveOccurred())

			triggerResourceTemplate.GenerateName = "test-"

			rawTriggerResourceTemplate, err = json.Marshal(triggerResourceTemplate)
			Expect(err).ToNot(HaveOccurred())
			triggerTemplate.Spec.ResourceTemplates[0].Raw = rawTriggerResourceTemplate
			err = k8sClient.Update(ctx, triggerTemplate)
			Expect(err).ToNot(HaveOccurred())

			// Check that a new build is submitted
			Eventually(func() bool {
				return len(listCreatedHasCompPipelienRuns().Items) > 1
			}, timeout, interval).Should(BeTrue())
		})
	})
})

// Single functions tests

func TestGetGitProvider(t *testing.T) {
	type args struct {
		ctx    context.Context
		gitURL string
	}
	tests := []struct {
		name       string
		args       args
		wantErr    bool
		wantString string
	}{
		{
			name: "github",
			args: args{
				ctx:    context.Background(),
				gitURL: "git@github.com:redhat-appstudio/application-service.git",
			},
			wantErr:    true, //unsupported
			wantString: "",
		},
		{
			name: "github https",
			args: args{
				ctx:    context.Background(),
				gitURL: "https://github.com/redhat-appstudio/application-service.git",
			},
			wantErr:    false,
			wantString: "https://github.com",
		},
		{
			name: "bitbucket https",
			args: args{
				ctx:    context.Background(),
				gitURL: "https://sbose78@bitbucket.org/sbose78/appstudio.git",
			},
			wantErr:    false,
			wantString: "https://bitbucket.org",
		},
		{
			name: "no scheme",
			args: args{
				ctx:    context.Background(),
				gitURL: "github.com/redhat-appstudio/application-service.git",
			},
			wantErr:    true, //fully qualified URL is a must
			wantString: "",
		},
		{
			name: "invalid url",
			args: args{
				ctx:    context.Background(),
				gitURL: "not-even-a-url",
			},
			wantErr:    true, //fully qualified URL is a must
			wantString: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := getGitProvider(tt.args.gitURL); (got != tt.wantString) ||
				(tt.wantErr == true && err == nil) ||
				(tt.wantErr == false && err != nil) {
				t.Errorf("UpdateServiceAccountIfSecretNotLinked() Got Error: = %v, want %v ; Got String:  = %v , want %v", err, tt.wantErr, got, tt.wantString)
			}
		})
	}
}

func TestUpdateServiceAccountIfSecretNotLinked(t *testing.T) {
	type args struct {
		gitSecretName  string
		serviceAccount *corev1.ServiceAccount
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "present",
			args: args{
				gitSecretName: "present",
				serviceAccount: &corev1.ServiceAccount{
					Secrets: []corev1.ObjectReference{
						{
							Name: "present",
						},
					},
				},
			},
			want: false, // since it was present, this implies the SA wasn't updated.
		},
		{
			name: "not present",
			args: args{
				gitSecretName: "not-present",
				serviceAccount: &corev1.ServiceAccount{
					Secrets: []corev1.ObjectReference{
						{
							Name: "something-else",
						},
					},
				},
			},
			want: true, // since it wasn't present, this implies the SA was updated.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateServiceAccountIfSecretNotLinked(tt.args.gitSecretName, tt.args.serviceAccount); got != tt.want {
				t.Errorf("UpdateServiceAccountIfSecretNotLinked() = %v, want %v", got, tt.want)
			}
		})
	}
}
