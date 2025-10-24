/*
Copyright 2024 The Kubernetes Authors.

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

package machine

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

var randsrc *rand.Rand

func init() {
	randsrc = rand.New(rand.NewSource(time.Now().UnixNano()))
}

var _ = Describe("Machine DefaultProvider.IsDeleting method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	It("returns false when Machine is nil", func() {
		Expect(provider.IsDeleting(nil)).To(BeFalse())
	})

	It("return false when Machine deletion timestamp is zero", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(provider.IsDeleting(machine)).To(BeFalse())
	})

	It("return true when Machine deletion timestamp is not zero", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		timestamp := metav1.NewTime(time.Now())
		machine.SetDeletionTimestamp(&timestamp)
		Expect(provider.IsDeleting(machine)).To(BeTrue())
	})
})

var _ = Describe("Machine DefaultProvider.Get method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.Machine{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineList := &capiv1beta1.MachineList{}
			Expect(cl.List(context.Background(), machineList, client.InNamespace(testNamespace))).To(Succeed())
			return machineList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns nil when there are no Machines present in API", func() {
		machine, err := provider.GetByProviderID(context.Background(), "")
		Expect(err).ToNot(HaveOccurred())
		Expect(machine).To(BeNil())
	})

	It("returns nil when there is no Machine with the requested provider ID", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machine, err := provider.GetByProviderID(context.Background(), "clusterapi://the-wrong-provider-id")
		Expect(err).ToNot(HaveOccurred())
		Expect(machine).To(BeNil())
	})

	It("returns the expected Machine when it is present in the API", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())
		machine = newMachine("karpenter-2", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		providerID := *machine.Spec.ProviderID
		machine, err := provider.GetByProviderID(context.Background(), providerID)
		Expect(err).ToNot(HaveOccurred())
		Expect(machine).Should(HaveField("Name", "karpenter-2"))
	})

	It("returns the expected Machine when it is present in the API and not a NodePool member", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())
		machine = newMachine("karpenter-2", "karpenter-cluster", false)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		providerID := *machine.Spec.ProviderID
		machine, err := provider.GetByProviderID(context.Background(), providerID)
		Expect(err).ToNot(HaveOccurred())
		Expect(machine).Should(HaveField("Name", "karpenter-2"))
	})
})

var _ = Describe("Machine DefaultProvider.List method", func() {
	var provider Provider

	NodePoolMemberLabelSelector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      providers.NodePoolMemberLabel,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.Machine{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineList := &capiv1beta1.MachineList{}
			Expect(cl.List(context.Background(), machineList, client.InNamespace(testNamespace))).To(Succeed())
			return machineList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns an empty list when no Machines are present in API", func() {
		machines, err := provider.List(context.Background(), &NodePoolMemberLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(0))
	})

	It("returns a list of correct length when there are only karpenter member Machines", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machines, err := provider.List(context.Background(), &NodePoolMemberLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(1))
	})

	It("returns a list of correct length when there are mixed member Machines", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machine = newMachine("clusterapi-1", "workload-cluster", false)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machines, err := provider.List(context.Background(), &NodePoolMemberLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(1))
	})

	It("returns an empty list when no member Machines are present", func() {
		machine := newMachine("clusterapi-1", "workload-cluster", false)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machines, err := provider.List(context.Background(), &NodePoolMemberLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(0))
	})
})

var _ = Describe("Machine DefaultProvider.AddDeleteAnnotation method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.Machine{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineList := &capiv1beta1.MachineList{}
			Expect(cl.List(context.Background(), machineList, client.InNamespace(testNamespace))).To(Succeed())
			return machineList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns an error when Machine is nil", func() {
		err := provider.AddDeleteAnnotation(context.Background(), nil)
		Expect(err).To(MatchError(fmt.Errorf("cannot add deletion annotation to Machine, nil value")))
	})

	It("returns an error when the Machine does not exist", func() {
		machine := newMachine("non-existent", "fake-cluster", false)
		err := provider.AddDeleteAnnotation(context.Background(), machine)
		Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("unable to add deletion annotation to Machine %q", machine.Name))))
	})

	It("adds the deletion annotation", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		err := provider.AddDeleteAnnotation(context.Background(), machine)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() map[string]string {
			m, err := provider.GetByProviderID(context.Background(), *machine.Spec.ProviderID)
			Expect(err).ToNot(HaveOccurred())
			return m.GetAnnotations()
		}).Should(HaveKey(capiv1beta1.DeleteMachineAnnotation))
	})
})

var _ = Describe("Machine DefaultProvider.RemoveDeleteAnnotation method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.Machine{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineList := &capiv1beta1.MachineList{}
			Expect(cl.List(context.Background(), machineList, client.InNamespace(testNamespace))).To(Succeed())
			return machineList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns an error when Machine is nil", func() {
		err := provider.RemoveDeleteAnnotation(context.Background(), nil)
		Expect(err).To(MatchError(fmt.Errorf("cannot remove deletion annotation from Machine, nil value")))
	})

	It("returns an error when the Machine does not exist", func() {
		machine := newMachine("non-existent", "fake-cluster", false)
		annotations := map[string]string{
			capiv1beta1.DeleteMachineAnnotation: time.Now().String(),
		}
		machine.SetAnnotations(annotations)
		err := provider.RemoveDeleteAnnotation(context.Background(), machine)
		Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("unable to remove deletion annotation from Machine %q", machine.Name))))
	})

	It("removes the deletion annotation", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		annotations := map[string]string{
			capiv1beta1.DeleteMachineAnnotation: time.Now().String(),
		}
		machine.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		err := provider.RemoveDeleteAnnotation(context.Background(), machine)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() map[string]string {
			m, err := provider.GetByProviderID(context.Background(), *machine.Spec.ProviderID)
			Expect(err).ToNot(HaveOccurred())
			return m.GetAnnotations()
		}).ShouldNot(HaveKey(capiv1beta1.DeleteMachineAnnotation))
	})
})

func newMachine(machineName string, clusterName string, karpenterMember bool) *capiv1beta1.Machine {
	machine := &capiv1beta1.Machine{}
	machine.SetName(machineName)
	machine.SetNamespace(testNamespace)
	if karpenterMember {
		labels := map[string]string{}
		labels[providers.NodePoolMemberLabel] = ""
		machine.SetLabels(labels)
	}
	machine.Spec.ClusterName = clusterName
	providerID := fmt.Sprintf("clusterapi://mock-%d\n", randsrc.Uint32())
	machine.Spec.ProviderID = &providerID
	return machine
}
