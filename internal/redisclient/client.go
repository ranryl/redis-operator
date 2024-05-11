package redisclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	redisv1beta1 "github.com/ranryl/redis-operator/api/v1beta1"
	"github.com/redis/go-redis/v9"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	redisRoleMaster = "role:master"
)

type RedisClient struct {
	k8sclient *kubernetes.Clientset
	k8sconfig *rest.Config
}

func NewRedisClient() (*RedisClient, error) {
	k8sconfig, err := GenerateK8sConfig()
	if err != nil {
		return nil, err
	}
	k8sclient, err := GenerateK8sClient(k8sconfig)
	if err != nil {
		return nil, err
	}
	return &RedisClient{
		k8sclient: k8sclient,
		k8sconfig: k8sconfig,
	}, nil
}

func (r *RedisClient) IsMaster(ip, namespace string) (bool, error) {
	cmd := []string{"redis-cli", "info", "replication"}
	info, err := r.ExecuteCmd(cmd, namespace, ip)
	if err != nil {
		return false, err
	}
	log.Log.Info(info)
	return strings.Contains(info, redisRoleMaster), nil
}
func (r *RedisClient) GetClusterSlots(connIP, port, nodeID string) (int, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(connIP, port),
		Password: "",
	})
	slots, err := client.ClusterSlots(context.Background()).Result()
	if err != nil {
		return 0, err
	}
	for _, slot := range slots {
		for _, node := range slot.Nodes {
			if node.ID == nodeID {
				return slot.End - slot.Start + 1, nil
			}
		}
	}
	return 0, errors.New("not found")
}
func (r *RedisClient) GetClusterSize(ip, port, password, namespace string) (int, int, error) {
	cmd := []string{"redis-cli", "cluster", "info"}
	info, err := r.ExecuteCmd(cmd, namespace, ip)
	if err != nil {
		return 0, 0, err
	}
	infoArr := strings.Split(info, "\r\n")
	infoMap := make(map[string]string, 0)
	for _, row := range infoArr {
		keys := strings.Split(row, ":")
		if len(keys) >= 2 {
			infoMap[keys[0]] = keys[1]
		}
	}
	clusterSize, err := strconv.Atoi(infoMap["cluster_size"])
	if err != nil {
		return clusterSize, 0, err
	}
	clusterCount, err := strconv.Atoi(infoMap["cluster_known_nodes"])
	if err != nil {
		return clusterSize, clusterCount, err
	}
	return clusterSize, clusterCount, nil
}
func (r *RedisClient) CreateCluster(ips []string, port, password string, rc *redisv1beta1.RedisCluster) (string, error) {
	cmd := []string{"redis-cli", "--cluster", "create"}
	if len(ips) <= 0 {
		return "", errors.New("cluster ips lens must great than 0")
	}
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	for i := 0; i < len(ips); i++ {
		cmd = append(cmd, net.JoinHostPort(ips[i]+podSufix, port))
	}
	cmd = append(cmd, "--cluster-replicas", strconv.Itoa(int(rc.Spec.Replicas)))
	cmd = append(cmd, "--cluster-yes")
	// cmd = append(cmd, "-a")
	// cmd = append(cmd, password)
	return r.ExecuteCmd(cmd, rc.Namespace, ips[0])
}
func (r *RedisClient) AddMaster(ip, connIP, port, password string, rc *redisv1beta1.RedisCluster) (string, error) {
	cmd := []string{"redis-cli", "--cluster", "add-node"}
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	cmd = append(cmd, net.JoinHostPort(ip+podSufix, port))
	cmd = append(cmd, net.JoinHostPort(connIP+podSufix, port))
	// cmd = append(cmd, "-a")
	// cmd = append(cmd, password)
	return r.ExecuteCmd(cmd, rc.Namespace, connIP)
}
func (r *RedisClient) GetNodeID(ip, port, password string, rc *redisv1beta1.RedisCluster) (string, error) {
	cmd := []string{"redis-cli", "cluster", "myid"}
	nodeId, err := r.ExecuteCmd(cmd, rc.Namespace, ip)
	return strings.Trim(nodeId, "\n"), err
}
func (r *RedisClient) Check(ip, port string, rc *redisv1beta1.RedisCluster) (string, error) {
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	cmd := []string{"redis-cli", "--cluster", "check", net.JoinHostPort(ip+podSufix, port)}
	result, err := r.ExecuteCmd(cmd, rc.Namespace, ip)
	if err != nil {
		return result, err
	}
	if !strings.Contains(result, "[ERR]") {
		return result, nil
	}
	return result, errors.New("redis cluster status failed")
}
func (r *RedisClient) Fix(ip, port, password string, rc *redisv1beta1.RedisCluster) (string, error) {
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	cmd := []string{"redis-cli", "--cluster", "fix"}
	cmd = append(cmd, net.JoinHostPort(ip+podSufix, port))
	return r.ExecuteCmd(cmd, rc.Namespace, ip)
}
func (r *RedisClient) Reshard(ip, port, password, from, to, slotsNum string, rc *redisv1beta1.RedisCluster) (string, error) {
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	cmd := []string{"redis-cli", "--cluster", "reshard"}
	cmd = append(cmd, net.JoinHostPort(ip+podSufix, port))
	cmd = append(cmd, "--cluster-from", from)
	cmd = append(cmd, "--cluster-to", to)
	cmd = append(cmd, "--cluster-slots", slotsNum)
	cmd = append(cmd, "--cluster-yes")
	return r.ExecuteCmd(cmd, rc.Namespace, ip)
}
func (r *RedisClient) Rebalance(ip, port, password string, rc *redisv1beta1.RedisCluster) (string, error) {
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	cmd := []string{"redis-cli", "--cluster", "rebalance"}
	cmd = append(cmd, net.JoinHostPort(ip+podSufix, port))
	return r.ExecuteCmd(cmd, rc.Namespace, ip)
}
func (r *RedisClient) AddSlave(ip, anyIP, port, password, masterID string, rc *redisv1beta1.RedisCluster) (string, error) {
	podSufix := "." + rc.Name + "." + rc.Namespace + ".svc.cluster.local"
	cmd := []string{"redis-cli", "--cluster", "add-node"}
	cmd = append(cmd, net.JoinHostPort(ip+podSufix, port))
	cmd = append(cmd, net.JoinHostPort(anyIP+podSufix, port))
	cmd = append(cmd, "--cluster-slave", "--cluster-master-id")
	cmd = append(cmd, masterID)
	return r.ExecuteCmd(cmd, rc.Namespace, anyIP)
}
func (r *RedisClient) DelNode(anyIP, port, id, password string, rc *redisv1beta1.RedisCluster) (string, error) {
	cmd := []string{"redis-cli", "--cluster", "del-node"}
	cmd = append(cmd, net.JoinHostPort(anyIP, port))
	cmd = append(cmd, id)
	// cmd = append(cmd, "-a")
	// cmd = append(cmd, password)
	return r.ExecuteCmd(cmd, rc.Namespace, anyIP)
}
func (r *RedisClient) ExecuteCmd(cmd []string, namespace, podName string) (stdout string, stderr error) {
	req := r.k8sclient.CoreV1().RESTClient().Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec").VersionedParams(&corev1.PodExecOptions{
		Container: "rediscluster-sample",
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(r.k8sconfig, "POST", req.URL())
	if err != nil {
		log.Log.Error(err, "Failed to init executor")
		return "", err
	}
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})
	if err != nil {
		return execOut.String(), fmt.Errorf("execute command with error: %w, stderr: %s", err, execErr.String())
	}
	return execOut.String(), nil
}

func GenerateK8sClient(kubeConfig *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(kubeConfig)
}
func GenerateK8sConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
}
