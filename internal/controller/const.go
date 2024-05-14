package controller

const (
	MinClusterShards   = 3
	RetryTimes         = 3
	LastApplied        = "kubectl.kubernetes.io/last-applied-configuration"
	ConfigPath         = "/usr/local/etc/redis/redis.conf"
	SentinelConfigPath = "/data/redis-sentinel.conf"
	DataPath           = "/data"
)
