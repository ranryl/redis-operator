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
	"strconv"
	"time"

	redisv1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
	"github.com/ranryl/redis-operator/internal/redisclient"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RedisClusterReconciler reconciles a RedisCluster object
type RedisClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	RedisClient *redisclient.RedisClient
}

//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redisclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redisclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.ranryl.io,resources=redisclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *RedisClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciler myredis cluster")
	instance := &redisv1beta1.RedisCluster{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if _, found := instance.ObjectMeta.Annotations["rediscluster.io/skip-reconcile"]; found {
		logger.Info("Found annotations skip-reconcile, so skipping reconcile")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// create cm
	oldcm := &corev1.ConfigMap{}
	cm := r.NewRedisConfig(instance)
	if err := r.Get(ctx, req.NamespacedName, oldcm); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, cm); err != nil {
				log.Log.Error(err, "create cm err:")
				return ctrl.Result{RequeueAfter: 10 * time.Second}, err
			}
		} else {
			log.Log.Error(err, "get cm err:")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
	} else {
		if err := r.Update(ctx, cm); err != nil {
			log.Log.Error(err, "update cm err:")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
	}
	stsInstance := &appsv1.StatefulSet{}
	if err := r.Get(ctx, req.NamespacedName, stsInstance); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		// create sts
		stsInstance = r.NewRedisSts(instance, req.Namespace)
		if err := r.Create(ctx, stsInstance); err != nil {
			logger.Info(err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else {
		if stsInstance.Status.AvailableReplicas != *stsInstance.Spec.Replicas {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		_, knownNodes, err := r.RedisClient.GetClusterSize(instance.Name+"-0", strconv.Itoa(int(instance.Spec.Port)), "", req.Namespace)
		if err != nil {
			log.Log.Error(err, "get cluster size: ")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		total := int(instance.Spec.Shard * (instance.Spec.Replicas + 1))
		// cluster scaler down
		if knownNodes > total {
			result, err := r.ScalerDown(knownNodes, total, req.Namespace, instance)
			if err != nil {
				log.Log.Error(err, "scaler down err")
				return result, nil
			}
			// return ctrl.Result{}, nil
		}
		if instance.Annotations[LastApplied] != "" {
			oldSpec := &redisv1beta1.RedisCluster{}
			if err := json.Unmarshal([]byte(instance.Annotations[LastApplied]), oldSpec); err != nil {
				logger.Info(err.Error())
				return ctrl.Result{}, err
			}
			if !reflect.DeepEqual(instance.Spec, *oldSpec) {
				newRedissts := r.NewRedisSts(instance, req.Namespace)
				currRedissts := &appsv1.StatefulSet{}
				if err := r.Client.Get(ctx, req.NamespacedName, currRedissts); err != nil {
					logger.Info(err.Error())
					return ctrl.Result{}, err
				}
				currRedissts.Spec = newRedissts.Spec
				if err := r.Client.Update(ctx, currRedissts); err != nil {
					logger.Info(err.Error())
					return ctrl.Result{}, err
				}
			}
		}
	}
	oldSvc := &corev1.Service{}
	newSvc := r.NewRedisService(instance)
	if err := r.Get(ctx, req.NamespacedName, oldSvc); err != nil {
		if errors.IsNotFound(err) {
			if err = r.Create(ctx, newSvc); err != nil {
				log.Log.Error(err, "create redis svc err")
				return ctrl.Result{}, err
			}
		} else {
			log.Log.Error(err, "get redis svc err")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
	} else {
		currIP := oldSvc.Spec.ClusterIP
		oldSvc.Spec = newSvc.Spec
		oldSvc.Spec.ClusterIP = currIP
		if err = r.Client.Update(ctx, oldSvc); err != nil {
			log.Log.Error(err, "update svc err")
			return ctrl.Result{}, err
		}
	}

	if err := r.Get(ctx, req.NamespacedName, stsInstance); err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if stsInstance.Status.AvailableReplicas != *stsInstance.Spec.Replicas {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	_, knownNodes, err := r.RedisClient.GetClusterSize(instance.Name+"-0", strconv.Itoa(int(instance.Spec.Port)), "", req.Namespace)
	if err != nil {
		log.Log.Error(err, "get cluster size: ")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	// create cluster
	if knownNodes == 1 {
		result, err := r.CreateRedisCluster(instance)
		if err != nil {
			log.Log.Error(err, "create redis cluster")
			return ctrl.Result{}, err
		}
		log.Log.Info(result)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	total := int(instance.Spec.Shard * (instance.Spec.Replicas + 1))
	// cluster scaler up
	if knownNodes < total {
		result, err := r.ScalerUp(knownNodes, total, instance)
		if err != nil {
			log.Log.Error(err, "scaler up err")
			return result, nil
		}
	}
	isUpdate := false
	if instance.Status.Replicas != instance.Spec.Replicas {
		instance.Status.Replicas = instance.Spec.Replicas
		isUpdate = true
	}
	if instance.Status.Shard != instance.Spec.Shard {
		instance.Status.Shard = instance.Spec.Shard
		isUpdate = true
	}
	if instance.Status.InitShard == 0 {
		instance.Status.InitShard = instance.Spec.Shard
		isUpdate = true
	}
	if isUpdate {
		if err := r.Status().Update(ctx, instance); err != nil {
			log.Log.Error(err, "update redis status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
func (r *RedisClusterReconciler) ScalerUp(knownNodes, total int, instance *redisv1beta1.RedisCluster) (ctrl.Result, error) {
	connIP := instance.Name + "-0"
	port := strconv.Itoa(int(instance.Spec.Port))
	for i := knownNodes; i < total; i++ {
		masterIP, masterPort := instance.Name+"-"+strconv.Itoa(i), port
		_, err := r.RedisClient.AddMaster(masterIP, connIP, port, "", instance)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("add master err: %w", err)
		}
		masterID, err := r.RedisClient.GetNodeID(masterIP, port, "", instance)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("get nodeID err: %w", err)
		}
		log.Log.Info("add master node: %s:%s succeed", masterIP, masterPort)
		slotsNum := strconv.Itoa(int(16384 / instance.Spec.Shard))
		for j := 0; j < RetryTimes; j++ {
			if j == RetryTimes-1 {
				info, err := r.RedisClient.Fix(masterIP, port, "", instance)
				if err != nil {
					log.Log.Error(err, "fix cluster err:")
				}
				log.Log.Info(info)
			}
			result, err := r.RedisClient.Check(masterIP, port, instance)
			log.Log.Info(result)
			if err == nil {
				break
			}
			log.Log.Error(err, "cluster check err, times: "+strconv.Itoa(j))
			time.Sleep(3 * time.Second)
		}
		i++
		for k := 0; k < int(instance.Spec.Replicas) && i < total; k++ {
			pod := instance.Name + "-" + strconv.Itoa(i)
			_, err := r.RedisClient.AddSlave(pod, masterIP, port, "", masterID, instance)
			if err != nil {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("add slave err: %w", err)
			}
			log.Log.Info("add slave node: %s:%s to master %s:%s succeed", masterIP, masterPort, pod, port)
			i++
		}
		log.Log.Info("start reshard ...")
		for j := 0; j < RetryTimes; j++ {
			if j == RetryTimes-1 {
				info, err := r.RedisClient.Fix(masterIP, port, "", instance)
				if err != nil {
					log.Log.Error(err, "fix cluster err:")
				}
				log.Log.Info(info)
			}
			result, err := r.RedisClient.Reshard(masterIP, port, "", "all", masterID, slotsNum, instance)
			if err == nil {
				log.Log.Info(result[0:100] + result[len(result)-100:])
				break
			}
			log.Log.Error(err, "reshard err, times: "+strconv.Itoa(j))
			time.Sleep(3 * time.Second)
		}
		log.Log.Info("reshard succeed")
	}
	return ctrl.Result{}, nil

}
func (r *RedisClusterReconciler) ScalerDown(knownNodes, total int, namespace string, instance *redisv1beta1.RedisCluster) (ctrl.Result, error) {
	// 缩容
	connIP := instance.Name + "-0"
	port := strconv.Itoa(int(instance.Spec.Port))
	if total < MinClusterShards {
		return ctrl.Result{}, fmt.Errorf("cluster shard must equal than %d", MinClusterShards)
	}
	masterList := make([]string, 0)
	for i := 0; i < total; i++ {
		ip := instance.Name + "-" + strconv.Itoa(i)
		isMaster, err := r.RedisClient.IsMaster(ip, namespace)
		if err != nil {
			log.Log.Error(err, "is master error:")
		}
		if isMaster {
			id, err := r.RedisClient.GetNodeID(ip, port, "", instance)
			if err != nil {
				log.Log.Error(err, "get node id error:")
			}
			if id != "" {
				masterList = append(masterList, id)
			}
		}
	}
	for i := knownNodes - 1; i >= total; i-- {
		var masterIP, masterID string
		for k := 0; k <= int(instance.Spec.Replicas) && i >= total; k++ {
			ip := instance.Name + "-" + strconv.Itoa(i)
			isMaster, err := r.RedisClient.IsMaster(ip, namespace)
			if err != nil {
				log.Log.Error(err, "is master error:")
			}
			if isMaster {
				masterIP = ip
				masterID, err = r.RedisClient.GetNodeID(ip, port, "", instance)
				if err != nil {
					log.Log.Error(err, "get node id error:")
				}
			} else {
				id, err := r.RedisClient.GetNodeID(ip, port, "", instance)
				if err != nil {
					log.Log.Error(err, "get node id error:")
				}
				info, err := r.RedisClient.DelNode(connIP, port, id, "", instance)
				log.Log.Info(info)
				if err != nil {
					log.Log.Error(err, "del node err")
					return ctrl.Result{RequeueAfter: 5 * time.Second}, err
				}
			}
			i--
		}
		if masterIP != "" {
			slots, err := r.RedisClient.GetClusterSlots(connIP, port, masterID)
			// slots, err := r.RedisClient.GetClusterSlots("127.0.0.1", "26379", masterID)
			if err != nil {
				log.Log.Error(err, "get cluster slots error:")
			}
			everySlot := slots / len(masterList)
			if slots > 0 {
				for j := 0; j < len(masterList); j++ {
					if j+1 == len(masterList) {
						everySlot = slots - everySlot*(len(masterList)-1)
					}
					info, err := r.RedisClient.Reshard(connIP, port, "", masterID, masterList[j], strconv.Itoa(everySlot), instance)
					log.Log.Info(info[:100] + info[len(info)-100:])
					if err != nil {
						log.Log.Error(err, "scale down reshard err")
						return ctrl.Result{RequeueAfter: 5 * time.Second}, err
					}
				}
			}
			info, err := r.RedisClient.DelNode(connIP, port, masterID, "", instance)
			log.Log.Info(info)
			if err != nil {
				log.Log.Error(err, "del node err")
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
			// return ctrl.Result{RequeueAfter: time.Second * 5}, fmt.Errorf("reshard err, try again")
		}
	}
	log.Log.Info("reduce instance succeed")
	return ctrl.Result{}, nil
}
func (r *RedisClusterReconciler) CreateRedisCluster(app *redisv1beta1.RedisCluster) (string, error) {
	minShard := app.Spec.Shard
	if minShard > MinClusterShards {
		minShard = MinClusterShards
	}
	minPods := minShard * (app.Spec.Replicas + 1)
	ips := []string{}
	for i := 0; i < int(minPods); i++ {
		ips = append(ips, app.Name+"-"+strconv.Itoa(i))
	}
	port := strconv.Itoa(int(app.Spec.Port))
	return r.RedisClient.CreateCluster(ips, port, "", app)
}
func (r *RedisClusterReconciler) NewRedisConfig(app *redisv1beta1.RedisCluster) *corev1.ConfigMap {
	data := make(map[string]string)
	data["redis.conf"] = app.Spec.RedisConfig
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
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
func (r *RedisClusterReconciler) NewRedisService(app *redisv1beta1.RedisCluster) *corev1.Service {
	app.Labels["app"] = app.Name
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Serivce",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
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
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "tcp",
					Port: app.Spec.Port,
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: app.Labels,
		},
	}
}
func (r *RedisClusterReconciler) NewRedisSts(app *redisv1beta1.RedisCluster, namespace string) *appsv1.StatefulSet {
	app.Labels["app"] = app.Name
	selector := &metav1.LabelSelector{MatchLabels: app.Labels}
	replicas := (app.Spec.Replicas + 1) * app.Spec.Shard
	args := app.Spec.Args
	args = append(args, ConfigPath)
	sts := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
			Namespace:   namespace,
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
					Labels: app.Labels,
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
							Args: args,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      app.Name,
									MountPath: ConfigPath,
									SubPath:   "redis.conf",
								},
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
					Volumes: []corev1.Volume{
						{
							Name: app.Name,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: app.Name,
									},
								},
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
func (r *RedisClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1beta1.RedisCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
