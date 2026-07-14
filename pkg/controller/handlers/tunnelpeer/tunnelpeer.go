package tunnelpeer

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/tunnel"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PeerReconciler applies a complete snapshot of the currently available
// tunnel peers.
type PeerReconciler interface {
	ReconcilePeers(map[string]string)
}

// Handler reconciles tunnel peers from Kubernetes EndpointSlices.
type Handler struct {
	selfID     string
	reconciler PeerReconciler
}

// New creates an EndpointSlice handler for tunnel peer discovery.
func New(id string, reconciler PeerReconciler) *Handler {
	return &Handler{
		selfID:     strings.TrimSpace(id),
		reconciler: reconciler,
	}
}

// Reconcile reads every EndpointSlice for the configured Service and sends a
// complete peer snapshot to the reconciler. The event object is deliberately
// not used, so removed-object events also produce the current snapshot.
func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	var endpointSlices discoveryv1.EndpointSliceList
	if err := req.Client.List(req.Ctx, &endpointSlices,
		kclient.InNamespace(req.Namespace),
		kclient.MatchingLabels{discoveryv1.LabelServiceName: req.Name},
	); err != nil {
		return fmt.Errorf("list tunnel peer EndpointSlices for Service %s/%s: %w", req.Namespace, req.Name, err)
	}

	h.reconciler.ReconcilePeers(selectPeers(endpointSlices.Items, req.Namespace, h.selfID))
	return nil
}

func selectPeers(endpointSlices []discoveryv1.EndpointSlice, namespace, selfID string) map[string]string {
	selected := make(map[string]peerCandidate)

	for i := range endpointSlices {
		endpointSlice := &endpointSlices[i]
		if endpointSlice.Namespace != namespace {
			continue
		}

		port, ok := httpPort(endpointSlice.Ports)
		if !ok {
			continue
		}

		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready ||
				endpoint.Conditions.Terminating != nil && *endpoint.Conditions.Terminating ||
				endpoint.TargetRef == nil || endpoint.TargetRef.Kind != "Pod" ||
				endpoint.TargetRef.UID == "" || endpoint.TargetRef.Namespace != endpointSlice.Namespace {
				continue
			}

			peerID := string(endpoint.TargetRef.UID)
			if peerID == selfID {
				continue
			}

			for _, endpointAddress := range endpoint.Addresses {
				address, ok := canonicalAddress(endpointAddress, endpointSlice.AddressType)
				if !ok {
					continue
				}

				candidate := peerCandidate{address: address, port: port}
				if current, exists := selected[peerID]; !exists || candidate.less(current) {
					selected[peerID] = candidate
				}
			}
		}
	}

	peers := make(map[string]string, len(selected))
	for peerID, candidate := range selected {
		peers[peerID] = "ws://" + net.JoinHostPort(candidate.address, strconv.Itoa(int(candidate.port))) + tunnel.PeerConnectPath
	}
	return peers
}

type peerCandidate struct {
	address string
	port    int32
}

func (c peerCandidate) less(other peerCandidate) bool {
	cIPv4 := net.ParseIP(c.address).To4() != nil
	otherIPv4 := net.ParseIP(other.address).To4() != nil
	if cIPv4 != otherIPv4 {
		return cIPv4
	}
	if c.address != other.address {
		return c.address < other.address
	}
	return c.port < other.port
}

func httpPort(ports []discoveryv1.EndpointPort) (int32, bool) {
	var selected int32
	for _, port := range ports {
		if port.Name == nil || *port.Name != "http" ||
			(port.Protocol != nil && *port.Protocol != corev1.ProtocolTCP) ||
			port.Port == nil || *port.Port <= 0 || *port.Port > 65535 {
			continue
		}
		if selected == 0 || *port.Port < selected {
			selected = *port.Port
		}
	}
	return selected, selected != 0
}

func canonicalAddress(address string, addressType discoveryv1.AddressType) (string, bool) {
	ip := net.ParseIP(strings.TrimSpace(address))
	if ip == nil {
		return "", false
	}

	switch addressType {
	case discoveryv1.AddressTypeIPv4:
		ip = ip.To4()
		if ip == nil {
			return "", false
		}
	case discoveryv1.AddressTypeIPv6:
		if ip.To4() != nil {
			return "", false
		}
	case discoveryv1.AddressTypeFQDN:
		return "", false
	default:
		return "", false
	}

	return ip.String(), true
}
