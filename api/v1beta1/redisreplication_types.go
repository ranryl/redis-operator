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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisReplicationSpec defines the desired state of RedisReplication
type RedisReplicationSpec struct {
	Image                string                         `json:"image"`
	MasterReplica        int32                          `json:"masterReplica"`
	SlaveReplica         int32                          `json:"slaveReplica"`
	Port                 int32                          `json:"port,omitempty"`
	RedisConfig          string                         `json:"redisConfig,omitempty"`
	SlaveConfig          string                         `json:"slaveConfig,omitempty"`
	ConfigPath           string                         `json:"configPath,omitempty"`
	HostNetwork          bool                           `json:"hostNetwork,omitempty"`
	Resources            corev1.ResourceRequirements    `json:"resources,omitempty"`
	NodeSelector         map[string]string              `json:"nodeSelector,omitempty"`
	Affinity             *corev1.Affinity               `json:"affinity,omitempty"`
	Tolerations          []corev1.Toleration            `json:"toleration,omitempty"`
	PriorityClassName    string                         `json:"priorityClassName,omitempty"`
	LivenessProbe        *corev1.Probe                  `json:"livenessProbe,omitempty"`
	ReadinessProbe       *corev1.Probe                  `json:"readinessProbe,omitempty"`
	StartupProbe         *corev1.Probe                  `json:"startupProbe,omitempty"`
	Volumes              []corev1.Volume                `json:"volumes,omitempty"`
	VolumeMounts         []corev1.VolumeMount           `json:"volumeMounts,omitempty"`
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// RedisReplicationStatus defines the observed state of RedisReplication
type RedisReplicationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RedisReplication is the Schema for the redisreplications API
type RedisReplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisReplicationSpec   `json:"spec,omitempty"`
	Status RedisReplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedisReplicationList contains a list of RedisReplication
type RedisReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisReplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisReplication{}, &RedisReplicationList{})
}
