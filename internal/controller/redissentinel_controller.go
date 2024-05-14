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

	redisv1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
)

// RedisSentinelReconciler reconciles a RedisSentinel object
type RedisSentinelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redissentinels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redissentinels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redissentinels/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisSentinel object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *RedisSentinelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	instance := &redisv1beta1.RedisSentinel{}
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
		if masterSts.Annotations[LastApplied] != "" {
			oldSpec := &appsv1.StatefulSet{}
			if err := json.Unmarshal([]byte(masterSts.Annotations[LastApplied]), oldSpec); err != nil {
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
		if slaveSts.Annotations[LastApplied] != "" {
			oldSpec := &appsv1.StatefulSet{}
			if err := json.Unmarshal([]byte(slaveSts.Annotations[LastApplied]), oldSpec); err != nil {
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
	}

	sentinelName := instance.Name + "-sentinel"
	sentinelConfig := r.NewSentinelConfig(sentinelName, instance, masterPodName)
	sentinelCm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: sentinelName}, sentinelCm); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, sentinelConfig); err != nil {
			log.Log.Error(err, "create sentinel cm err:")
		}
	} else {
		if err := r.Update(ctx, sentinelConfig); err != nil {
			log.Log.Error(err, "update sentinel cm err:")
		}
	}
	sentinelDep := &appsv1.Deployment{}
	newSentinelDep := r.NewSentinel(sentinelName, instance)
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: sentinelName}, sentinelDep); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, newSentinelDep); err != nil {
			log.Log.Error(err, "create sentile deployment err:")
			return ctrl.Result{}, err
		}
	} else {
		if sentinelDep.Annotations[LastApplied] != "" {
			oldSpec := &appsv1.StatefulSet{}
			if err := json.Unmarshal([]byte(sentinelDep.Annotations[LastApplied]), oldSpec); err != nil {
				log.Log.Info(err.Error())
				return ctrl.Result{}, err
			}
			if !reflect.DeepEqual(newSentinelDep.Spec, *oldSpec) {
				sentinelDep.Spec = newSentinelDep.Spec
				if err := r.Client.Update(ctx, sentinelDep); err != nil {
					log.Log.Info(err.Error())
					return ctrl.Result{}, err
				}
			}
		}
	}
	sentinelService := &corev1.Service{}
	newSentinelService := r.NewSentinelService(sentinelName, instance)
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: sentinelName}, sentinelService); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, newSentinelService); err != nil {
			log.Log.Error(err, "create sentinel service err:")
		}
	} else {
		if err := r.Update(ctx, newSentinelService); err != nil {
			log.Log.Error(err, "update sentinel service err:")
		}
	}
	return ctrl.Result{}, nil
}
func (r *RedisSentinelReconciler) NewMaster(name string, app *redisv1beta1.RedisSentinel) *appsv1.StatefulSet {
	sts := r.NewStatefulSet("master", name, app)
	sts.Spec.Replicas = &app.Spec.MasterReplica
	if app.Spec.RedisConfig != "" {
		sts.Spec.Template.Spec.Containers[0].Args = append(sts.Spec.Template.Spec.Containers[0].Args, ConfigPath)
		configVolumeMount := corev1.VolumeMount{
			Name:      name,
			MountPath: ConfigPath,
			SubPath:   "redis.conf",
		}
		sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, configVolumeMount)
		configVolume := corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		}
		sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, configVolume)
	}
	return sts
}
func (r *RedisSentinelReconciler) NewMasterConfig(name string, app *redisv1beta1.RedisSentinel) *corev1.ConfigMap {
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
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Data: data,
	}
}
func (r *RedisSentinelReconciler) NewMasterService(name string, app *redisv1beta1.RedisSentinel) *corev1.Service {
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
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
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
func (r *RedisSentinelReconciler) NewSlave(name string, app *redisv1beta1.RedisSentinel) *appsv1.StatefulSet {
	sts := r.NewStatefulSet("slave", name, app)
	sts.Spec.Replicas = &app.Spec.SlaveReplica
	if app.Spec.SlaveConfig != "" {
		sts.Spec.Template.Spec.Containers[0].Args = append(sts.Spec.Template.Spec.Containers[0].Args, ConfigPath)
		configVolumeMount := corev1.VolumeMount{
			Name:      name,
			MountPath: ConfigPath,
			SubPath:   "redis.conf",
		}
		sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, configVolumeMount)
		configVolume := corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		}
		sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, configVolume)
	}
	return sts
}
func (r *RedisSentinelReconciler) NewSlaveConfig(name string, app *redisv1beta1.RedisSentinel, masterPodName string) *corev1.ConfigMap {
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
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Data: data,
	}
}
func (r *RedisSentinelReconciler) NewSentinel(name string, app *redisv1beta1.RedisSentinel) *appsv1.Deployment {
	deploy := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
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
		Spec: appsv1.DeploymentSpec{
			Replicas: &app.Spec.SentinelReplica,
			Selector: &metav1.LabelSelector{MatchLabels: app.Labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: app.Labels,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "busybox",
							Image: "busybox",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      name,
									MountPath: "/etc/redis-sentinel.conf",
									SubPath:   "redis-sentinel.conf",
								},
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
							Command: []string{"/bin/sh"},
							Args:    []string{"-c", "cat /etc/redis-sentinel.conf > " + SentinelConfigPath},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            name,
							Image:           app.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "tcp",
									ContainerPort: app.Spec.Port,
								},
							},
							Args: []string{
								SentinelConfigPath,
								"--sentinel",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
							Resources:      app.Spec.Resources,
							LivenessProbe:  app.Spec.LivenessProbe,
							ReadinessProbe: app.Spec.ReadinessProbe,
							StartupProbe:   app.Spec.StartupProbe,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: name,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: name,
									},
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					NodeSelector:      app.Spec.NodeSelector,
					Affinity:          app.Spec.Affinity,
					Tolerations:       app.Spec.Tolerations,
					HostNetwork:       app.Spec.HostNetwork,
					PriorityClassName: app.Spec.PriorityClassName,
				},
			},
		},
	}
	deploy.Spec.Template.Labels["app"] = "sentinel"
	return &deploy
}
func (r *RedisSentinelReconciler) NewSentinelConfig(name string, app *redisv1beta1.RedisSentinel, masterPodName string) *corev1.ConfigMap {
	if app.Spec.SentinelConfig == "" {
		return nil
	}
	data := make(map[string]string)
	app.Spec.SentinelConfig += "\nsentinel monitor mymaster " + masterPodName + " " + strconv.Itoa(int(app.Spec.Port)) + " " + strconv.Itoa(2)
	data["redis-sentinel.conf"] = app.Spec.SentinelConfig
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
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Data: data,
	}
}
func (r *RedisSentinelReconciler) NewSentinelService(name string, app *redisv1beta1.RedisSentinel) *corev1.Service {
	labels := app.Labels
	labels["app"] = "sentinel"
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
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   redisv1beta1.GroupVersion.Group,
					Version: redisv1beta1.GroupVersion.Version,
					Kind:    app.Kind,
				}),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "tcp",
					Port: app.Spec.Port,
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
		},
	}
}

func (r *RedisSentinelReconciler) NewStatefulSet(labelAppName, name string, app *redisv1beta1.RedisSentinel) *appsv1.StatefulSet {
	labels := app.Labels
	labels["app"] = labelAppName
	selector := &metav1.LabelSelector{MatchLabels: labels}
	sts := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
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
			ServiceName: name,
			Selector:    selector,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            name,
							Image:           app.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "tcp",
									ContainerPort: app.Spec.Port,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: DataPath,
								},
							},
							Resources:      app.Spec.Resources,
							LivenessProbe:  app.Spec.LivenessProbe,
							ReadinessProbe: app.Spec.ReadinessProbe,
							StartupProbe:   app.Spec.StartupProbe,
						},
					},
					NodeSelector:      app.Spec.NodeSelector,
					Affinity:          app.Spec.Affinity,
					Tolerations:       app.Spec.Tolerations,
					HostNetwork:       app.Spec.HostNetwork,
					PriorityClassName: app.Spec.PriorityClassName,
				},
			},
		},
	}
	if app.Spec.StorageSpec != nil {
		sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data",
					Namespace: app.Namespace,
				},
				Spec: *app.Spec.StorageSpec,
			},
		}
	} else {
		sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
	return sts
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisSentinelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1beta1.RedisSentinel{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
