package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/pkg/agentbackend"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// metricsAPIPath is the standard metrics.k8s.io endpoint, served by
// metrics-server. It is queried through a raw REST call rather than the typed
// k8s.io/metrics client so that live usage costs no extra module dependency.
const metricsAPIPath = "/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods"

// podMetrics is the subset of PodMetrics this package reads.
type podMetricsList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Containers []struct {
			Name  string `json:"name"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}

// usageReader reports live per-pod resource consumption.
type usageReader interface {
	// PodUsage returns measured usage keyed by pod name. A nil map with a nil
	// error means measurement is unavailable, which callers treat as a reason
	// to fall back rather than as a failure.
	PodUsage(ctx context.Context, namespace, labelSelector string) (map[string]agentbackend.ResourceUtilization, error)
	// PoolVolumeUsage returns bytes used on a pool's shared volume. The bool is
	// false when the figure cannot be trusted to describe the pool alone.
	PoolVolumeUsage(ctx context.Context, node, namespace, claimName string, claimCapacity int64) (int64, bool, error)
}

type metricsServerReader struct {
	client rest.Interface
}

func newMetricsReader(config *rest.Config) (usageReader, error) {
	if config == nil {
		return nil, nil
	}
	// Copy: the metrics API is not the core group, and mutating the caller's
	// config would affect every other client built from it.
	metricsConfig := rest.CopyConfig(config)
	metricsConfig.GroupVersion = &schema.GroupVersion{}
	metricsConfig.APIPath = "/apis"
	// Required even though every call here uses DoRaw and parses the bytes
	// itself; UnversionedRESTClientFor refuses to build without one.
	metricsConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	client, err := rest.UnversionedRESTClientFor(metricsConfig)
	if err != nil {
		return nil, fmt.Errorf("build metrics client: %w", err)
	}
	return &metricsServerReader{client: client}, nil
}

func (m *metricsServerReader) PodUsage(ctx context.Context, namespace, labelSelector string) (map[string]agentbackend.ResourceUtilization, error) {
	raw, err := m.client.Get().
		AbsPath(fmt.Sprintf(metricsAPIPath, namespace)).
		Param("labelSelector", labelSelector).
		DoRaw(ctx)
	if err != nil {
		// metrics-server is optional. Reporting an error here would make
		// utilization unavailable on clusters that simply do not run it, so an
		// unreachable metrics API is reported as "no measurement".
		return nil, nil
	}

	var list podMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parse pod metrics: %w", err)
	}

	usage := make(map[string]agentbackend.ResourceUtilization, len(list.Items))
	for _, item := range list.Items {
		var total agentbackend.ResourceUtilization
		for _, container := range item.Containers {
			// Quantities arrive in whatever unit metrics-server chose, commonly
			// nanocores for CPU, so they are parsed rather than assumed.
			if cpu, err := resource.ParseQuantity(container.Usage.CPU); err == nil {
				total.CPUVCPUs += float64(cpu.MilliValue()) / 1000
			}
			if memory, err := resource.ParseQuantity(container.Usage.Memory); err == nil {
				total.MemoryBytes += memory.Value()
			}
		}
		usage[item.Metadata.Name] = total
	}
	return usage, nil
}

// statsSummary is the subset of kubelet's /stats/summary this package reads.
type statsSummary struct {
	Pods []struct {
		Volume []struct {
			UsedBytes     int64 `json:"usedBytes"`
			CapacityBytes int64 `json:"capacityBytes"`
			PVCRef        struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"pvcRef"`
		} `json:"volume"`
	} `json:"pods"`
}

// PoolVolumeUsage reports bytes used on a pool's shared volume.
//
// Kubelet is the only source for this without deploying a node agent, and it
// reports the filesystem the volume lives on. For a dedicated volume that is
// the pool; for a hostPath-backed storage class such as local-path it is the
// whole node disk, which would show every pool as permanently near-full.
//
// claimCapacity is the volume's own capacity, used to tell those apart: a
// filesystem materially larger than the claim is shared with the host, and its
// usage says nothing about the pool. Reporting nothing is better than reporting
// a number that describes someone else's disk.
func (m *metricsServerReader) PoolVolumeUsage(ctx context.Context, node, namespace, claimName string, claimCapacity int64) (int64, bool, error) {
	if node == "" || claimName == "" {
		return 0, false, nil
	}

	raw, err := m.client.Get().
		AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", node)).
		DoRaw(ctx)
	if err != nil {
		// Requires nodes/proxy, which a deployment may reasonably withhold.
		return 0, false, nil
	}

	var summary statsSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return 0, false, fmt.Errorf("parse kubelet stats summary: %w", err)
	}

	for _, pod := range summary.Pods {
		for _, volume := range pod.Volume {
			if volume.PVCRef.Name != claimName || volume.PVCRef.Namespace != namespace {
				continue
			}
			if !volumeIsDedicated(volume.CapacityBytes, claimCapacity) {
				return 0, false, nil
			}
			return volume.UsedBytes, true, nil
		}
	}
	return 0, false, nil
}

// volumeIsDedicated reports whether a filesystem's size is close enough to the
// claim's to be that claim's own. Provisioners round, so this tolerates a
// modest overshoot but rejects an order-of-magnitude difference.
func volumeIsDedicated(filesystemBytes, claimBytes int64) bool {
	if filesystemBytes <= 0 || claimBytes <= 0 {
		return false
	}
	return filesystemBytes <= claimBytes*2
}
