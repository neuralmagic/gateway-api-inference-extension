/*
Copyright 2025 The Kubernetes Authors.

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

package datastore

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	utiltest "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/testing"
)

const roleLabel = "llmd.org/role"

var (
	prefillPod = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "prefillPod", Namespace: "test", Labels: map[string]string{roleLabel: "prefill"}}, Status: corev1.PodStatus{PodIP: "address-1"}}
	decodePod  = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "decodePod", Namespace: "test", Labels: map[string]string{roleLabel: "decode"}}, Status: corev1.PodStatus{PodIP: "address-1"}}
	bothPod    = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bothPod", Namespace: "test"}, Status: corev1.PodStatus{PodIP: "address-1"}}
	pmc        = &metrics.FakePodMetricsClient{}
	pmf        = metrics.NewPodMetricsFactory(pmc, time.Second)
)

func TestPodRole(t *testing.T) {
	pool := &v1alpha2.InferencePool{
		Spec: v1alpha2.InferencePoolSpec{
			TargetPortNumber: int32(8000),
			Selector: map[v1alpha2.LabelKey]v1alpha2.LabelValue{
				"some-key": "some-val",
			},
		},
	}

	tests := []struct {
		name      string
		pod       *corev1.Pod
		wantRoles map[string]metrics.PodRole
	}{
		{
			name:      "Add new prefill pod",
			pod:       utiltest.FromBase(prefillPod).ReadyCondition().ObjRef(),
			wantRoles: map[string]metrics.PodRole{prefillPod.Name: metrics.Prefill},
		},
		{
			name:      "Add new decode pod",
			pod:       utiltest.FromBase(decodePod).ReadyCondition().ObjRef(),
			wantRoles: map[string]metrics.PodRole{decodePod.Name: metrics.Decode},
		},
		{
			name:      "Add new pod without role label",
			pod:       utiltest.FromBase(bothPod).ReadyCondition().ObjRef(),
			wantRoles: map[string]metrics.PodRole{bothPod.Name: metrics.Both},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set up the scheme.
			scheme := runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(scheme)
			initialObjects := []client.Object{}
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(initialObjects...).
				Build()

			// Configure the initial state of the datastore.
			store := NewDatastore(t.Context(), pmf)
			_ = store.PoolSet(t.Context(), fakeClient, pool)
			store.PodUpdateOrAddIfNotExist(test.pod)

			for _, pm := range store.PodGetAll() {
				wantRole, ok := test.wantRoles[pm.GetPod().NamespacedName.Name]
				if !ok {
					t.Errorf("unexpected pod %s", pm.GetPod().NamespacedName.Name)
				}
				if wantRole != pm.GetPod().Role {
					t.Errorf("invalid pod role, got (%d) != want (%d)", pm.GetPod().Role, wantRole)
				}
			}
		})
	}
}
