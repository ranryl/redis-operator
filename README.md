# redis-operator
kubebuilder init --domain ranryl.com --repo github.com/ranryl/redis-operator
kubebuilder create api --group cache.ranryl.io --version v1beta1 --kind Redis
kubebuilder create api --group cache.ranryl.io --version v1beta1 --kind RedisReplication
kubebuilder create api --group cache.ranryl.io --version v1beta1 --kind RedisSentinel
kubebuilder create api --group cache.ranryl.io --version v1beta1 --kind RedisCluster
