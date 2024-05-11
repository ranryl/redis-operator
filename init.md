kubebuilder init --domain ranryl.io --repo github.com/ranryl/redis-operator
kubebuilder create api --group redis --version v1beta1 --kind Redis
kubebuilder create api --group redis --version v1beta1 --kind RedisReplication
kubebuilder create api --group redis --version v1beta1 --kind RedisSentinel
kubebuilder create api --group redis --version v1beta1 --kind RedisCluster