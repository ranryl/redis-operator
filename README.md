# redis-operator
## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
#### 1. install use exits images
install
```
kubectl apply -f https://raw.githubusercontent.com/ranryl/redis-operator/main/dist/install.yaml
```
uninstall
```
kubectl delete -f https://raw.githubusercontent.com/ranryl/redis-operator/main/dist/install.yaml

```
#### 2. use source code build your image to installer
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/redis-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/redis-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```
#### Examples
create redis instance
```
apiVersion: redis.ranryl.io/v1beta1
kind: Redis
metadata:
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/managed-by: kustomize
  name: redis-sample
spec:
  image: "redis:7.2.4"
  port: 6379

```
create redis replication instance
```
apiVersion: redis.ranryl.io/v1beta1
kind: RedisReplication
metadata:
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/managed-by: kustomize
  name: redisreplication-sample
spec:
  image: "redis:7.2.4"
  masterReplica: 1
  slaveReplica: 2
  port: 6379
  configPath: "/usr/local/etc/redis/redis.conf"
  slaveConfig: |
    bind * -::*
    protected-mode no
    port 6379
    tcp-backlog 511
    timeout 0
    tcp-keepalive 300
    daemonize no
    pidfile /var/run/redis_6379.pid
    #replicaof 127.0.0.1 6379
```
create redis sentinel instance
```
apiVersion: redis.ranryl.io/v1beta1
kind: RedisSentinel
metadata:
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/managed-by: kustomize
  name: redissentinel-sample
spec:
  image: "redis:7.2.4"
  masterReplica: 1
  slaveReplica: 2
  sentinelReplica: 3
  port: 6379
  configPath: "/usr/local/etc/redis/redis.conf"
  sentinelConfigPath: "/data/redis-sentinel.conf"
  slaveConfig: |
    bind * -::*
    protected-mode no
    port 6379
    tcp-backlog 511
    timeout 0
    tcp-keepalive 300
    daemonize no
    pidfile /var/run/redis_6379.pid
    #replicaof 127.0.0.1 6379
  sentinelConfig: |
    port 26379
    daemonize no
    pidfile /var/run/redis-sentinel.pid
    logfile ""
    dir /tmp
    #sentinel monitor mymaster 127.0.0.1 6379 2
    sentinel down-after-milliseconds mymaster 30000
    acllog-max-len 128
    sentinel parallel-syncs mymaster 1
    sentinel failover-timeout mymaster 180000
    sentinel deny-scripts-reconfig yes
    SENTINEL resolve-hostnames yes
    SENTINEL announce-hostnames yes
```
create redis cluster
```
apiVersion: redis.ranryl.io/v1beta1
kind: RedisCluster
metadata:
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/managed-by: kustomize
  name: rediscluster-sample
spec:
  image: "redis:7.2.4"
  replicas: 1
  shard: 3
  port: 6379
  nodeSelector:
    kubernetes.io/hostname: lima-k3s
  priorityClassName: system-node-critical
  livenessProbe:
    tcpSocket:
        port: 6379
    initialDelaySeconds: 2
    periodSeconds: 5
    timeoutSeconds: 3
    failureThreshold: 3
    successThreshold: 1
  readinessProbe:
    tcpSocket:
        port: 6379
    initialDelaySeconds: 2
    periodSeconds: 5
    timeoutSeconds: 3
    failureThreshold: 3
    successThreshold: 1
  volumeMounts:
  # - mountPath: /data
  #   name: data
  #   readOnly: false
  - mountPath: /usr/local/etc/redis/redis.conf
    name: rediscluster-sample
    subPath: redis.conf
  volumes:
  - configMap:
      name: rediscluster-sample
    name: rediscluster-sample
  # volumeClaimTemplates:
  # - metadata:
  #     namespace: default
  #     name: data
  #   apiVersion: v1
  #   kind: PersistentVolumeClaim
  #   spec:
  #     accessModes:
  #     - ReadWriteOnce
  #     resources:
  #       requests:
  #         storage: 1G
  #     storageClassName: local-path
  args: 
    - /usr/local/etc/redis/redis.conf
  redisConfig: |
    bind * -::*
    protected-mode no
    port 6379
    tcp-backlog 511
    timeout 0
    tcp-keepalive 300
    daemonize no
    pidfile /var/run/redis_6379.pid
    cluster-enabled yes
```
## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=rekca/redis-operator:0.1.0
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/ranryl/redis-operator/main/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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

