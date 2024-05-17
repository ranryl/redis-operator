/*
Copyright 2024.

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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
)

var _ = Describe("RedisCluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "rediscluster-sample"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		rediscluster := &redisv1beta1.RedisCluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind RedisCluster")
			err := k8sClient.Get(ctx, typeNamespacedName, rediscluster)
			if err != nil && errors.IsNotFound(err) {
				resource := &redisv1beta1.RedisCluster{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "redis.ranryl.io/v1beta1",
						Kind:       "RedisCluster",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: redisv1beta1.RedisClusterSpec{
						Image:    "redis:7.2.4",
						Port:     6379,
						Replicas: 3,
						Shard:    1,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, rediscluster)
					return err == nil
				}, time.Second*10, time.Second*1).Should(BeTrue())
			}
			Expect(rediscluster.Spec.Image).Should(Equal("redis:7.2.4"))
			Expect(rediscluster.Spec.Port).Should(Equal(int32(6379)))
			Expect(rediscluster.Spec.Replicas).Should(Equal(int32(3)))
			Expect(rediscluster.Spec.Shard).Should(Equal(int32(1)))

		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &redisv1beta1.RedisCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RedisCluster")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &RedisClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
