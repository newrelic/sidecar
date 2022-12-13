package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"
	"gopkg.in/relistan/rubberneck.v1"
)

type ListenerUrlsConfig struct {
	Urls []string `envconfig:"URLS"`
}

type HAproxyConfig struct {
	ReloadCmd    string `envconfig:"RELOAD_COMMAND"`
	VerifyCmd    string `envconfig:"VERIFY_COMMAND"`
	BindIP       string `envconfig:"BIND_IP" default:"192.168.168.168"`
	TemplateFile string `envconfig:"TEMPLATE_FILE" default:"views/haproxy.cfg"`
	ConfigFile   string `envconfig:"CONFIG_FILE" default:"/etc/haproxy.cfg"`
	PidFile      string `envconfig:"PID_FILE" default:"/var/run/haproxy.pid"`
	Disable      bool   `envconfig:"DISABLE"`
	User         string `envconfig:"USER" default:"haproxy"`
	Group        string `envconfig:"GROUP" default:""`
	UseHostnames bool   `envconfig:"USE_HOSTNAMES"`
}

type EnvoyConfig struct {
	UseGRPCAPI   bool   `envconfig:"USE_GRPC_API" default:"true"`
	BindIP       string `envconfig:"BIND_IP" default:"192.168.168.168"`
	UseHostnames bool   `envconfig:"USE_HOSTNAMES"`
	GRPCPort     string `envconfig:"GRPC_PORT" default:"7776"`
}

type ServicesConfig struct {
	NameMatch    string `envconfig:"NAME_MATCH"`
	ServiceNamer string `envconfig:"NAMER" default:"docker_label"`
	NameLabel    string `envconfig:"NAME_LABEL" default:"ServiceName"`
}

type SidecarConfig struct {
	ExcludeIPs             []string      `envconfig:"EXCLUDE_IPS" default:"192.168.168.168"`
	Discovery              []string      `envconfig:"DISCOVERY" default:"docker"`
	StatsAddr              string        `envconfig:"STATS_ADDR"`
	PushPullInterval       time.Duration `envconfig:"PUSH_PULL_INTERVAL" default:"20s"`
	GossipMessages         int           `envconfig:"GOSSIP_MESSAGES" default:"15"`
	GossipInterval         time.Duration `envconfig:"GOSSIP_INTERVAL" default:"200ms"`
	HandoffQueueDepth      int           `envconfig:"HANDOFF_QUEUE_DEPTH" default:"1024"`
	LoggingFormat          string        `envconfig:"LOGGING_FORMAT"`
	LoggingLevel           string        `envconfig:"LOGGING_LEVEL" default:"info"`
	DefaultCheckEndpoint   string        `envconfig:"DEFAULT_CHECK_ENDPOINT" default:"/version"`
	Seeds                  []string      `envconfig:"SEEDS"`
	ClusterName            string        `envconfig:"CLUSTER_NAME" default:"default"`
	AdvertiseIP            string        `envconfig:"ADVERTISE_IP"`
	BindPort               int           `envconfig:"BIND_PORT" default:"7946"`
	Debug                  bool          `envconfig:"DEBUG" default:"false"`
	DiscoverySleepInterval time.Duration `envconfig:"DISCOVERY_SLEEP_INTERVAL" default:"1s"`
}

type DockerConfig struct {
	DockerURL string `envconfig:"URL" default:"unix:///var/run/docker.sock"`
}

type StaticConfig struct {
	ConfigFile string `envconfig:"CONFIG_FILE" default:"static.json"`
}

type K8sAPIConfig struct {
	KubeAPIIP        string        `envconfig:"KUBE_API_IP" default:"127.0.0.1"`
	KubeAPIPort      int           `envconfig:"KUBE_API_PORT" default:"8080"`
	Namespace        string        `envconfig:"NAMESPACE" default:"default"`
	KubeTimeout      time.Duration `envconfig:"KUBE_TIMEOUT" default:"3s"`
	CredsPath        string        `envconfig:"CREDS_PATH" default:"/var/run/secrets/kubernetes.io/serviceaccount"`
	AnnounceAllNodes bool          `envconfig:"ANNOUNCE_ALL_NODES" default:"false"`
}

type Config struct {
	Sidecar         SidecarConfig      // SIDECAR_
	DockerDiscovery DockerConfig       // DOCKER_
	StaticDiscovery StaticConfig       // STATIC_
	K8sAPIDiscovery K8sAPIConfig       // K8S_
	Services        ServicesConfig     // SERVICES_
	HAproxy         HAproxyConfig      // HAPROXY_
	Envoy           EnvoyConfig        // ENVOY_
	Listeners       ListenerUrlsConfig // LISTENERS_
}

func ParseConfig() *Config {
	var config Config

	errs := []error{
		envconfig.Process("sidecar", &config.Sidecar),
		envconfig.Process("docker", &config.DockerDiscovery),
		envconfig.Process("static", &config.StaticDiscovery),
		envconfig.Process("k8s", &config.K8sAPIDiscovery),
		envconfig.Process("services", &config.Services),
		envconfig.Process("haproxy", &config.HAproxy),
		envconfig.Process("envoy", &config.Envoy),
		envconfig.Process("listeners", &config.Listeners),
	}

	for _, err := range errs {
		if err != nil {
			rubberneck.Print(config)
			log.Fatalf("Can't parse environment config: %s", err)
		}
	}

	return &config
}
