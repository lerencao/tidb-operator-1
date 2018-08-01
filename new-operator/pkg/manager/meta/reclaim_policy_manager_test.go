// Copyright 2018 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package meta

import (
	"testing"

	"fmt"

	. "github.com/onsi/gomega"
	"github.com/pingcap/tidb-operator/new-operator/pkg/apis/pingcap.com/v1"
	"github.com/pingcap/tidb-operator/new-operator/pkg/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestReclaimPolicyManagerSync(t *testing.T) {
	g := NewGomegaWithT(t)
	type testcase struct {
		name         string
		pvcHasLabels bool
		updateErr    bool
		err          bool
		changed      bool
	}

	testFn := func(test *testcase, t *testing.T) {
		t.Log(test.name)
		tc := newTidbClusterForRPM()
		pv1 := newPV()
		pvc1 := newPVC(tc, pv1)

		if !test.pvcHasLabels {
			pvc1.Labels = nil
		}

		rpm, fakePVControl, pvcIndexer, pvIndexer := newFakeReclaimPolicyManager()
		err := pvcIndexer.Add(pvc1)
		g.Expect(err).NotTo(HaveOccurred())
		err = pvIndexer.Add(pv1)
		g.Expect(err).NotTo(HaveOccurred())

		if test.updateErr {
			fakePVControl.SetUpdatePVError(errors.NewInternalError(fmt.Errorf("API server failed")), 0)
		}

		err = rpm.Sync(tc)
		if test.err {
			g.Expect(err).To(HaveOccurred())
			pv, err := rpm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pv.Spec.PersistentVolumeReclaimPolicy).To(Equal(corev1.PersistentVolumeReclaimDelete))
		}
		if test.changed {
			g.Expect(err).NotTo(HaveOccurred())
			pv, err := rpm.pvLister.Get(pv1.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pv.Spec.PersistentVolumeReclaimPolicy).To(Equal(corev1.PersistentVolumeReclaimRetain))
		}
	}

	tests := []testcase{
		{
			name:         "normal",
			pvcHasLabels: true,
			updateErr:    false,
			err:          false,
			changed:      true,
		},
		{
			name:         "pvc don't have labels",
			pvcHasLabels: false,
			updateErr:    false,
			err:          false,
			changed:      false,
		},
		{
			name:         "update failed",
			pvcHasLabels: true,
			updateErr:    true,
			err:          true,
			changed:      false,
		},
	}

	for i := range tests {
		testFn(&tests[i], t)
	}
}

func newFakeReclaimPolicyManager() (*reclaimPolicyManager, *controller.FakePVControl, cache.Indexer, cache.Indexer) {
	kubeCli := kubefake.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeCli, 0)
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()

	pvControl := controller.NewFakePVControl(pvInformer)

	return &reclaimPolicyManager{
		pvcInformer.Lister(),
		pvInformer.Lister(),
		pvControl,
	}, pvControl, pvcInformer.Informer().GetIndexer(), pvInformer.Informer().GetIndexer()
}

func newTidbClusterForRPM() *v1.TidbCluster {
	return &v1.TidbCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TidbCluster",
			APIVersion: "pingcap.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
		},
		Spec: v1.TidbClusterSpec{
			PVReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
	}
}

func newPV() *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv-1",
			Namespace: "",
			UID:       types.UID("test"),
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
		},
	}
}

func newPVC(tc *v1.TidbCluster, pv *corev1.PersistentVolume) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pvc-1",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
			Labels: map[string]string{
				"cluster.pingcap.com/app":         "tikv",
				"cluster.pingcap.com/owner":       "tidbCluster",
				"cluster.pingcap.com/tidbCluster": tc.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "pv-1",
		},
	}
}
