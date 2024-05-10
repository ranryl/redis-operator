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
	"reflect"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cacheranryliov1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
)

// RedisReplicationReconciler reconciles a RedisReplication object
type RedisReplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cache.ranryl.io.ranryl.com,resources=redisreplications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.ranryl.io.ranryl.com,resources=redisreplications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.ranryl.io.ranryl.com,resources=redisreplications/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisReplication object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *RedisReplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	instance := &cacheranryliov1beta1.RedisReplication{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if _, found := instance.ObjectMeta.Annotations["rediscluster.io/skip-reconcile"]; found {
		log.Log.Info("Found annotations skip-reconcile, so skipping reconcile")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	masterName := instance.Name + "-master"
	masterConfig := r.NewMasterConfig(masterName, instance)
	if masterConfig != nil {
		masterCm := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: masterName}, masterCm); err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, masterConfig); err != nil {
				log.Log.Error(err, "create master cm err:")
			}
		} else {
			if err := r.Update(ctx, masterConfig); err != nil {
				log.Log.Error(err, "update master cm err:")
			}
		}
	}
	masterSts := &appsv1.StatefulSet{}
	newMasterSts := r.NewMaster(masterName, instance)
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: masterName}, masterSts); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, newMasterSts); err != nil {
			log.Log.Error(err, "create master sts err:")
			return ctrl.Result{}, err
		}
	} else {
		oldSpec := &appsv1.StatefulSet{}
		lastApplied := "kubectl.kubernetes.io/last-applied-configuration"
		if err := json.Unmarshal([]byte(masterSts.Annotations[lastApplied]), oldSpec); err != nil {
			log.Log.Info(err.Error())
			return ctrl.Result{}, err
		}
		if !reflect.DeepEqual(newMasterSts.Spec, *oldSpec) {
			masterSts.Spec = newMasterSts.Spec
			if err := r.Client.Update(ctx, masterSts); err != nil {
				log.Log.Info(err.Error())
				return ctrl.Result{}, err
			}
		}
	}
	masterService := &corev1.Service{}
	newMasterService := r.NewMasterService(masterName, instance)
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: masterName}, masterService); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, newMasterService); err != nil {
			log.Log.Error(err, "create master service err:")
		}
	} else {
		if err := r.Update(ctx, newMasterService); err != nil {
			log.Log.Error(err, "update master service err:")
		}
	}

	masterPodName := masterName + "-0." + masterName + "." + req.Namespace + ".svc.cluster.local"
	slaveName := instance.Name + "-slave"
	slaveConfig := r.NewSlaveConfig(slaveName, instance, masterPodName)
	slaveCm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: slaveName}, slaveCm); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, slaveConfig); err != nil {
			log.Log.Error(err, "create slave cm err:")
		}
	} else {
		if err := r.Update(ctx, slaveConfig); err != nil {
			log.Log.Error(err, "update slave cm err:")
		}
	}
	slaveSts := &appsv1.StatefulSet{}
	newSlaveSts := r.NewSlave(slaveName, instance)
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: slaveName}, slaveSts); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, newSlaveSts); err != nil {
			log.Log.Error(err, "create slave sts err:")
			return ctrl.Result{}, err
		}
	} else {
		oldSpec := &appsv1.StatefulSet{}
		lastApplied := "kubectl.kubernetes.io/last-applied-configuration"
		if err := json.Unmarshal([]byte(slaveSts.Annotations[lastApplied]), oldSpec); err != nil {
			log.Log.Info(err.Error())
			return ctrl.Result{}, err
		}
		if !reflect.DeepEqual(newSlaveSts.Spec, *oldSpec) {

			slaveSts.Spec = newSlaveSts.Spec
			if err := r.Client.Update(ctx, slaveSts); err != nil {
				log.Log.Info(err.Error())
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}
func (r *RedisReplicationReconciler) NewMaster(name string, app *cacheranryliov1beta1.RedisReplication) *appsv1.StatefulSet {
	sts := r.NewStatefulSet("master", app)
	sts.Spec.Replicas = &app.Spec.MasterReplica
	sts.ObjectMeta.Name = name
	sts.Spec.ServiceName = name
	if app.Spec.RedisConfig != "" {
		sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		})
		sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: app.Spec.ConfigPath,
			SubPath:   "redis.conf",
		})
		sts.Spec.Template.Spec.Containers[0].Args = append(sts.Spec.Template.Spec.Containers[0].Args, app.Spec.ConfigPath)
	}
	return sts
}
func (r *RedisReplicationReconciler) NewMasterConfig(name string, app *cacheranryliov1beta1.RedisReplication) *corev1.ConfigMap {
	if app.Spec.RedisConfig == "" {
		return nil
	}
	data := make(map[string]string)
	data["redis.conf"] = app.Spec.RedisConfig
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   app.Namespace,
			Labels:      app.Labels,
			Annotations: app.Annotations,
		},
		Data: data,
	}
}
func (r *RedisReplicationReconciler) NewMasterService(name string, app *cacheranryliov1beta1.RedisReplication) *corev1.Service {
	labels := app.Labels
	labels["app"] = "master"
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Serivce",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   app.Namespace,
			Labels:      labels,
			Annotations: app.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "tcp",
					Port: app.Spec.Port,
				},
			},
			Type:      corev1.ServiceTypeClusterIP,
			Selector:  labels,
			ClusterIP: "None",
		},
	}
}
func (r *RedisReplicationReconciler) NewSlave(name string, app *cacheranryliov1beta1.RedisReplication) *appsv1.StatefulSet {
	sts := r.NewStatefulSet("slave", app)
	sts.Spec.Replicas = &app.Spec.SlaveReplica
	sts.ObjectMeta.Name = name
	sts.Spec.ServiceName = name
	sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		},
	})
	sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      name,
		MountPath: app.Spec.ConfigPath,
		SubPath:   "redis.conf",
	})
	sts.Spec.Template.Spec.Containers[0].Args = append(sts.Spec.Template.Spec.Containers[0].Args, app.Spec.ConfigPath)
	return sts
}
func (r *RedisReplicationReconciler) NewSlaveConfig(name string, app *cacheranryliov1beta1.RedisReplication, masterPodName string) *corev1.ConfigMap {
	if app.Spec.SlaveConfig == "" {
		return nil
	}
	data := make(map[string]string)
	app.Spec.SlaveConfig += "\nreplicaof " + masterPodName + " " + strconv.Itoa(int(app.Spec.Port))
	data["redis.conf"] = app.Spec.SlaveConfig
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   app.Namespace,
			Labels:      app.Labels,
			Annotations: app.Annotations,
		},
		Data: data,
	}
}
func (r *RedisReplicationReconciler) NewStatefulSet(appName string, app *cacheranryliov1beta1.RedisReplication) *appsv1.StatefulSet {
	labels := app.Labels
	labels["app"] = appName
	selector := &metav1.LabelSelector{MatchLabels: labels}
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
					Group:   cacheranryliov1beta1.GroupVersion.Group,
					Version: cacheranryliov1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: app.Name,
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

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cacheranryliov1beta1.RedisReplication{}).
		Complete(r)
}
