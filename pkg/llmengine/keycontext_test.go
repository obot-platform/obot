package llmengine

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestKeyGuardKeepsAcceptedContextAfterCallerCancellation(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	var afterLoad func(context.Context)
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayProvider); ok && err == nil && afterLoad != nil {
				afterLoad(ctx)
			}
			return err
		},
	})
	store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	e := New()
	require.NoError(t, e.Start(t.Context(), Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	_, err = e.CreateProvider(t.Context(), json.RawMessage(`{"provider":"azure"}`))
	require.NoError(t, err)

	seed, err := e.CreateKey(t.Context(), "azure", json.RawMessage(`{"name":"seed","value":"secret","azure_key_config":{"endpoint":"https://93.184.216.34"}}`))
	require.NoError(t, err)

	var lookups atomic.Int32
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			lookups.Add(1)
			return testKeyDNS(ctx, network, address)
		},
	}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			started := time.Now()
			afterLoad = func(accepted context.Context) {
				afterLoad = nil
				deadline, ok := accepted.Deadline()
				require.True(t, ok)
				require.WithinRange(t, deadline, started.Add(30*time.Second), time.Now().Add(30*time.Second))
				cancel()
				require.NoError(t, accepted.Err(), "acceptance must detach caller cancellation")
			}
			endpoint := "https://" + operation + "-key-context.invalid"
			body := testJSON(t, map[string]any{"name": operation, "value": "secret", "azure_key_config": map[string]any{"endpoint": endpoint}})
			before := lookups.Load()
			var key Key
			if operation == "create" {
				key, err = e.CreateKey(ctx, "azure", body)
			} else {
				key, err = e.UpdateKey(ctx, "azure", seed.ID, body)
			}
			require.NoError(t, err)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Greater(t, lookups.Load(), before, "the hostname guard must resolve each changed endpoint")
			provider, err := store.GetProvider(t.Context(), "azure")
			require.NoError(t, err)
			index := slices.IndexFunc(provider.Keys, func(k schemas.Key) bool { return k.ID == key.ID })
			require.NotEqual(t, -1, index)
			require.Equal(t, endpoint, provider.Keys[index].AzureKeyConfig.Endpoint.GetValue())
		})
	}
}

// Resolver injection uses an in-memory DNS exchange, so successful hostname
// validation never contacts a DNS server or opens a network listener.
func testKeyDNS(context.Context, string, string) (net.Conn, error) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		var size [2]byte
		if _, err := io.ReadFull(server, size[:]); err != nil {
			return
		}
		raw := make([]byte, binary.BigEndian.Uint16(size[:]))
		if _, err := io.ReadFull(server, raw); err != nil {
			return
		}
		var request dnsmessage.Message
		if err := request.Unpack(raw); err != nil {
			return
		}
		response := dnsmessage.Message{
			ID:                 request.ID,
			Response:           true,
			RecursionAvailable: true,
			Questions:          request.Questions,
		}
		for _, question := range request.Questions {
			if question.Type == dnsmessage.TypeA {
				response.Answers = append(response.Answers, dnsmessage.Resource{
					Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
					Body:   &dnsmessage.AResource{A: [4]byte{93, 184, 216, 34}},
				})
			}
		}
		raw, err := response.Pack()
		if err != nil {
			return
		}
		binary.BigEndian.PutUint16(size[:], uint16(len(raw)))
		_, _ = server.Write(append(size[:], raw...))
	}()
	return client, nil
}

func TestKeyGuardUsesAcceptedDeadlineWhileResolverBlocked(t *testing.T) {
	e, _ := startTestEngine(t)
	_, err := e.CreateProvider(t.Context(), json.RawMessage(`{"provider":"azure"}`))
	require.NoError(t, err)
	previousResolver := net.DefaultResolver
	t.Cleanup(func() { net.DefaultResolver = previousResolver })

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		defer func() {
			close(release)
			synctest.Wait()
		}()
		var lookups atomic.Int32
		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(context.Context, string, string) (net.Conn, error) {
				lookups.Add(1)
				// Ignore the resolver's own dial timeout: the guard's accepted
				// context must bound the entire lookup, even if DNS is stuck.
				<-release
				return nil, errors.New("test DNS released")
			},
		}
		done := make(chan error, 1)
		started := time.Now()
		go func() {
			_, err := e.CreateKey(context.WithoutCancel(t.Context()), "azure", json.RawMessage(`{"name":"blocked","value":"secret","azure_key_config":{"endpoint":"https://blocked-key-context.invalid"}}`))
			done <- err
		}()
		synctest.Wait()
		require.Positive(t, lookups.Load())
		// Time advances only inside this fake-clock bubble; no timeout is waited in real time.
		time.Sleep(30 * time.Second)
		synctest.Wait()
		select {
		case err := <-done:
			require.ErrorContains(t, err, "timeout")
			require.Equal(t, 30*time.Second, time.Since(started))
		default:
			t.Error("hostname validation outlived the accepted mutation deadline")
		}
	})
	provider, err := e.store.GetProvider(t.Context(), "azure")
	require.NoError(t, err)
	require.Empty(t, provider.Keys, "timed-out validation must not persist a key")
}
