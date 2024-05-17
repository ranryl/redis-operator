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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
)

var _ = Describe("Redis Controller", func() {
	const (
		timeout   = time.Second * 10
		duration  = time.Second * 10
		interval  = time.Millisecond * 250
		namespace = "default"
	)
	Context("When reconciling a resource", func() {
		const resourceName = "redis-sample"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace, // TODO(user):Modify as needed
		}
		redis := &redisv1beta1.Redis{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Redis")
			err := k8sClient.Get(ctx, typeNamespacedName, redis)
			if err != nil && errors.IsNotFound(err) {
				resource := &redisv1beta1.Redis{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "redis.ranryl.io/v1beta1",
						Kind:       "Redis",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					// TODO(user): Specify other spec details if needed.
					Spec: redisv1beta1.RedisSpec{
						Image: "redis:7.2.4",
						Port:  6379,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, redis)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				Expect(redis.Spec.Image).Should(Equal("redis:7.2.4"))
				Expect(redis.Spec.Port).Should(Equal(int32(6379)))
				By("By creating a new statefulset")
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &redisv1beta1.Redis{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Redis")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			found := &redisv1beta1.Redis{}
			Eventually(func() error {

				return k8sClient.Get(ctx, typeNamespacedName, found)
			}, time.Minute, time.Second).Should(Succeed())

			controllerReconciler := &RedisReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				found := &appsv1.StatefulSet{}
				return k8sClient.Get(ctx, typeNamespacedName, found)
			}, time.Minute, time.Second).Should(Succeed())
		})
	})
})
