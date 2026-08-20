package tunnelpeer

import (
	"maps"
	"slices"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/tunnel"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileAggregatesEndpointSlices(t *testing.T) {
	peerA := endpointSlice("slice-a", discoveryv1.AddressTypeIPv4, 8080,
		endpoint("peer-a", "10.0.0.1"))
	peerB := endpointSlice("slice-b", discoveryv1.AddressTypeIPv4, 8081,
		endpoint("peer-b", "10.0.0.2"))
	irrelevant := endpointSlice("other-service", discoveryv1.AddressTypeIPv4, 8082,
		endpoint("other", "10.0.0.3"))
	irrelevant.Labels[discoveryv1.LabelServiceName] = "other-service"
	reconciler := newRecordingReconciler()
	handler := New("self", reconciler)

	client := newClient(t, peerA, peerB, irrelevant)
	if err := handler.Reconcile(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Namespace: "obot-system",
		Name:      "obot",
	}, nil); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"peer-a": "ws://10.0.0.1:8080" + tunnel.PeerConnectPath,
		"peer-b": "ws://10.0.0.2:8081" + tunnel.PeerConnectPath,
	}
	if got := reconciler.last(); !maps.Equal(got, want) {
		t.Fatalf("peers = %#v, want %#v", got, want)
	}
}

func TestReconcileUpdatesAndRemovesPeers(t *testing.T) {
	peerA := endpointSlice("slice-a", discoveryv1.AddressTypeIPv4, 8080,
		endpoint("peer-a", "10.0.0.1"))
	peerB := endpointSlice("slice-b", discoveryv1.AddressTypeIPv4, 8080,
		endpoint("peer-b", "10.0.0.2"))
	reconciler := newRecordingReconciler()
	handler := New("self", reconciler)
	client := newClient(t, peerA, peerB)

	reconcile := func(object kclient.Object) {
		t.Helper()
		if err := handler.Reconcile(router.Request{
			Ctx:       t.Context(),
			Client:    client,
			Object:    object,
			Namespace: "obot-system",
			Name:      "obot",
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	reconcile(peerA)

	peerB.Endpoints[0].Addresses = []string{"10.0.0.22"}
	if err := client.Update(t.Context(), peerB); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(t.Context(), peerA); err != nil {
		t.Fatal(err)
	}
	reconcile(nil)

	want := map[string]string{
		"peer-b": "ws://10.0.0.22:8080" + tunnel.PeerConnectPath,
	}
	if got := reconciler.last(); !maps.Equal(got, want) {
		t.Fatalf("peers after update and removal = %#v, want %#v", got, want)
	}

	if err := client.Delete(t.Context(), peerB); err != nil {
		t.Fatal(err)
	}
	reconcile(nil)
	if got := reconciler.last(); len(got) != 0 {
		t.Fatalf("peers after deleting last slice = %#v, want empty snapshot", got)
	}
	if got := reconciler.callCount(); got != 3 {
		t.Fatalf("ReconcilePeers calls = %d, want 3", got)
	}
}

func TestSelectPeersFiltersInvalidEndpoints(t *testing.T) {
	readyFalse := false
	terminatingTrue := true
	valid := endpoint("valid", "10.0.0.1")
	missingTarget := endpoint("unused", "10.0.0.2")
	missingTarget.TargetRef = nil
	wrongKind := endpoint("wrong-kind", "10.0.0.3")
	wrongKind.TargetRef.Kind = "Service"
	emptyUID := endpoint("empty-uid", "10.0.0.4")
	emptyUID.TargetRef.UID = ""
	wrongNamespace := endpoint("wrong-namespace", "10.0.0.5")
	wrongNamespace.TargetRef.Namespace = "elsewhere"
	unready := endpoint("unready", "10.0.0.6")
	unready.Conditions.Ready = &readyFalse
	terminating := endpoint("terminating", "10.0.0.7")
	terminating.Conditions.Terminating = &terminatingTrue
	self := endpoint("self", "10.0.0.8")
	mismatchedAddressType := endpoint("mismatched", "2001:db8::9")
	invalidAddress := endpoint("invalid-address", "not-an-ip")

	slices := []discoveryv1.EndpointSlice{
		*endpointSlice("endpoints", discoveryv1.AddressTypeIPv4, 8080,
			valid, missingTarget, wrongKind, emptyUID, wrongNamespace, unready,
			terminating, self, mismatchedAddressType, invalidAddress),
		*endpointSliceWithPorts("wrong-port-name", discoveryv1.AddressTypeIPv4,
			[]discoveryv1.EndpointPort{{Name: new("metrics"), Port: new(int32(8080))}},
			endpoint("wrong-port-name", "10.0.1.1")),
		*endpointSliceWithPorts("udp-port", discoveryv1.AddressTypeIPv4,
			[]discoveryv1.EndpointPort{{Name: new("http"), Protocol: new(corev1.ProtocolUDP), Port: new(int32(8080))}},
			endpoint("udp-port", "10.0.1.2")),
		*endpointSliceWithPorts("invalid-port", discoveryv1.AddressTypeIPv4,
			[]discoveryv1.EndpointPort{{Name: new("http"), Port: new(int32(70000))}},
			endpoint("invalid-port", "10.0.1.3")),
	}

	want := map[string]string{
		"valid": "ws://10.0.0.1:8080" + tunnel.PeerConnectPath,
	}
	if got := selectPeers(slices, "obot-system", "self"); !maps.Equal(got, want) {
		t.Fatalf("peers = %#v, want %#v", got, want)
	}
}

func TestSelectPeersIPv6AndDeterministicDuplicates(t *testing.T) {
	slicesByName := map[string]discoveryv1.EndpointSlice{
		"ipv6": *endpointSlice("ipv6", discoveryv1.AddressTypeIPv6, 8080,
			endpoint("ipv6-peer", "2001:0db8::1")),
		"duplicate-ipv6": *endpointSlice("duplicate-ipv6", discoveryv1.AddressTypeIPv6, 7000,
			endpoint("duplicate", "2001:db8::2")),
		"duplicate-high-address": *endpointSlice("duplicate-high-address", discoveryv1.AddressTypeIPv4, 6000,
			endpoint("duplicate", "10.0.0.9")),
		"duplicate-high-port": *endpointSlice("duplicate-high-port", discoveryv1.AddressTypeIPv4, 9000,
			endpoint("duplicate", "10.0.0.2")),
		"duplicate-selected": *endpointSlice("duplicate-selected", discoveryv1.AddressTypeIPv4, 5000,
			endpoint("duplicate", "10.0.0.2")),
	}
	names := make([]string, 0, len(slicesByName))
	for name := range slicesByName {
		names = append(names, name)
	}
	slices.Sort(names)

	forward := make([]discoveryv1.EndpointSlice, 0, len(names))
	for _, name := range names {
		forward = append(forward, slicesByName[name])
	}
	reverse := slices.Clone(forward)
	slices.Reverse(reverse)

	want := map[string]string{
		"ipv6-peer": "ws://[2001:db8::1]:8080" + tunnel.PeerConnectPath,
		"duplicate": "ws://10.0.0.2:5000" + tunnel.PeerConnectPath,
	}
	for _, endpointSlices := range [][]discoveryv1.EndpointSlice{forward, reverse} {
		if got := selectPeers(endpointSlices, "obot-system", "self"); !maps.Equal(got, want) {
			t.Fatalf("peers = %#v, want %#v", got, want)
		}
	}
}

type recordingReconciler struct {
	snapshots []map[string]string
}

func newRecordingReconciler() *recordingReconciler {
	return &recordingReconciler{}
}

func (r *recordingReconciler) ReconcilePeers(peers map[string]string) {
	r.snapshots = append(r.snapshots, maps.Clone(peers))
}

func (r *recordingReconciler) last() map[string]string {
	if len(r.snapshots) == 0 {
		return nil
	}
	return r.snapshots[len(r.snapshots)-1]
}

func (r *recordingReconciler) callCount() int {
	return len(r.snapshots)
}

func newClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func endpointSlice(name string, addressType discoveryv1.AddressType, port int32, endpoints ...discoveryv1.Endpoint) *discoveryv1.EndpointSlice {
	return endpointSliceWithPorts(name, addressType, []discoveryv1.EndpointPort{{
		Name:     new("http"),
		Protocol: new(corev1.ProtocolTCP),
		Port:     new(port),
	}}, endpoints...)
}

func endpointSliceWithPorts(name string, addressType discoveryv1.AddressType, ports []discoveryv1.EndpointPort, endpoints ...discoveryv1.Endpoint) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		Name:      name,
		Namespace: "obot-system",
		Labels: map[string]string{
			discoveryv1.LabelServiceName: "obot",
		},
		AddressType: addressType,
		Ports:       ports,
		Endpoints:   endpoints,
	}
}

func endpoint(peerID string, addresses ...string) discoveryv1.Endpoint {
	return discoveryv1.Endpoint{
		Addresses: addresses,
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: "obot-system",
			Name:      "obot-" + peerID,
			UID:       types.UID(peerID),
		},
	}
}
