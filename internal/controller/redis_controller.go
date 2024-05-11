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
	"encoding/json"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	redisv1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redis/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciler myredis")
	instance := &redisv1beta1.Redis{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("app kind: " + instance.Kind + ", app name: " + instance.Name)
	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	redisSts := &appsv1.StatefulSet{}
	if err := r.Client.Get(ctx, req.NamespacedName, redisSts); err != nil {
		if !errors.IsNotFound(err) {
			logger.Info(err.Error())
			return ctrl.Result{}, err
		}
		fmt.Println(redisSts)
		// 创建
		newSts := r.NewStatefulSet(instance)
		if err := r.Client.Create(ctx, newSts); err != nil {
			logger.Info(err.Error())
			return ctrl.Result{}, err
		}
		svc := r.NewService(instance)
		if err := r.Client.Create(ctx, svc); err != nil {
			logger.Info(err.Error())
			return ctrl.Result{}, err
		}
	} else {
		// 更新
		oldSpec := &redisv1beta1.RedisSpec{}
		if err := json.Unmarshal([]byte(instance.Annotations["spec"]), oldSpec); err != nil {
			logger.Info(err.Error())
			return ctrl.Result{}, err
		}
		fmt.Println(*oldSpec)
		fmt.Println(instance.Spec)
		if !reflect.DeepEqual(instance.Spec, *oldSpec) {
			newSts := r.NewStatefulSet(instance)
			currSts := &appsv1.StatefulSet{}
			if err := r.Client.Get(ctx, req.NamespacedName, currSts); err != nil {
				logger.Info(err.Error())
				return ctrl.Result{}, err
			}
			currSts.Spec = newSts.Spec
			if err := r.Client.Update(ctx, currSts); err != nil {
				logger.Info(err.Error())
				return ctrl.Result{}, err
			}

			newService := r.NewService(instance)
			currService := &corev1.Service{}
			if err := r.Client.Get(ctx, req.NamespacedName, currService); err != nil {
				logger.Info(err.Error())
				return ctrl.Result{}, err
			}
			currIP := currService.Spec.ClusterIP
			currService.Spec = newService.Spec
			currService.Spec.ClusterIP = currIP
			if err = r.Client.Update(ctx, currService); err != nil {
				logger.Info(err.Error())
				return ctrl.Result{}, err
			}
		}
	}
	data, _ := json.Marshal(instance.Spec)
	if instance.Annotations != nil {
		instance.Annotations["spec"] = string(data)
	} else {
		instance.Annotations = map[string]string{"spec": string(data)}
	}
	if err := r.Client.Update(ctx, instance); err != nil {
		logger.Info(err.Error())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
func (r *RedisReconciler) NewStatefulSet(app *redisv1beta1.Redis) *appsv1.StatefulSet {
	labels := map[string]string{"app": app.Name}
	selector := &metav1.LabelSelector{MatchLabels: labels}
	var replicas int32 = 1
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
			Namespace:   app.Namespace,
			Annotations: app.Annotations,
			Labels:      app.Labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: app.Name,
			Replicas:    &replicas,
			Selector:    selector,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            app.Name,
							Image:           app.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "tcp",
									ContainerPort: app.Spec.Port,
								},
							},
							Args:           app.Spec.Args,
							VolumeMounts:   app.Spec.VolumeMounts,
							Resources:      app.Spec.Resources,
							LivenessProbe:  app.Spec.LivenessProbe,
							ReadinessProbe: app.Spec.ReadinessProbe,
							StartupProbe:   app.Spec.StartupProbe,
						},
					},
					Volumes:           app.Spec.Volumes,
					NodeSelector:      app.Spec.NodeSelector,
					Affinity:          app.Spec.Affinity,
					Tolerations:       app.Spec.Tolerations,
					HostNetwork:       app.Spec.HostNetwork,
					PriorityClassName: app.Spec.PriorityClassName,
				},
			},
			VolumeClaimTemplates: app.Spec.VolumeClaimTemplates,
		},
	}
}
func (r *RedisReconciler) NewService(app *redisv1beta1.Redis) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolSCTP,
					Port:       app.Spec.Port,
					TargetPort: intstr.FromInt(int(app.Spec.Port)),
				},
			},
			Selector: map[string]string{
				"app": app.Name,
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1beta1.Redis{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
