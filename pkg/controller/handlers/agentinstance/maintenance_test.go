package agentinstance

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/substrate"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type maintenanceControl struct {
	pb.ControlClient
	list func(context.Context) (*pb.ListActorTemplatesResponse, error)
}

func maintenanceGateway(t *testing.T, onLock func()) *gclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://" + filepath.Join(t.TempDir(), "maintenance.db")})
	require.NoError(t, err)
	services.DB.SQLDB.SetMaxOpenConns(2)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	require.NoError(t, services.DB.DB.Callback().Raw().Before("gorm:raw").Register("test_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			onLock()
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1")
			tx.Statement.Vars = nil
		}
	}))
	return gateway
}

func (c maintenanceControl) ListActorTemplates(ctx context.Context, _ *pb.ListActorTemplatesRequest, _ ...grpc.CallOption) (*pb.ListActorTemplatesResponse, error) {
	return c.list(ctx)
}

func TestMaintenanceRechecksOrphansAfterDiscovery(t *testing.T) {
	locks := 0
	gateway := maintenanceGateway(t, func() { locks++ })
	scheme := storagescheme.Scheme
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "ate.dev", Version: "v1alpha1", Kind: "WorkerPoolList"}, &unstructured.UnstructuredList{})
	kube := fake.NewClientBuilder().WithScheme(scheme).Build()
	storage := fake.NewClientBuilder().WithScheme(scheme).Build()
	instance := &v1.AgentInstance{}
	instance.Name = "acr-resumed"
	instance.Spec.UserID = "owner"
	ref := substrate.Ref(instance)
	control := maintenanceControl{list: func(context.Context) (*pb.ListActorTemplatesResponse, error) {
		require.Zero(t, locks, "remote discovery must not acquire the provisioning lock")
		require.NoError(t, storage.Create(t.Context(), instance))
		return &pb.ListActorTemplatesResponse{ActorTemplates: []*pb.ActorTemplate{{
			Metadata:       &pb.ResourceMetadata{Atespace: ref.Atespace, Name: ref.Name, CreateTime: timestamppb.New(time.Now().Add(-time.Hour))},
			WorkerSelector: &pb.Selector{MatchLabels: map[string]string{"obot.ai/claude-environment": "env"}},
		}}}, nil
	}}
	h := Handler{Gateway: gateway, Substrate: &substrate.Client{Kube: kube, Control: control}}
	require.NoError(t, h.maintain(t.Context(), storage))
	require.Equal(t, 1, locks, "recheck under lock must protect a newly recorded session")
}

func TestMaintenanceSkipsStalePoolConfiguration(t *testing.T) {
	gateway := maintenanceGateway(t, func() {})
	environment := &gt.ClaudeEnvironment{ID: "env", EnvironmentID: "ccpool_test"}
	require.NoError(t, gateway.SaveClaudeEnvironment(t.Context(), environment, nil))
	kube := fake.NewClientBuilder().Build()
	// Change the desired state between discovery and mutation.
	kube = interceptor.NewClient(kube, interceptor.Funcs{
		Get: func(ctx context.Context, client kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			environment.Runtime.WorkerImage = "new-image"
			require.NoError(t, gateway.SaveClaudeEnvironment(ctx, environment, nil))
			return client.Get(ctx, key, obj, opts...)
		},
		Create: func(context.Context, kclient.WithWatch, kclient.Object, ...kclient.CreateOption) error {
			t.Fatal("must not apply stale worker configuration")
			return nil
		},
	})
	h := Handler{Gateway: gateway, Substrate: &substrate.Client{
		Kube: kube,
		Control: maintenanceControl{list: func(context.Context) (*pb.ListActorTemplatesResponse, error) {
			return &pb.ListActorTemplatesResponse{}, nil
		}},
	}}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	require.NoError(t, h.maintain(t.Context(), storage))
}

func TestStalledMaintenanceDiscoveryDoesNotLockProvisioning(t *testing.T) {
	locks := 0
	gateway := maintenanceGateway(t, func() { locks++ })
	h := Handler{
		Gateway: gateway,
		Substrate: &substrate.Client{
			Kube: fake.NewClientBuilder().Build(),
			Control: maintenanceControl{list: func(ctx context.Context) (*pb.ListActorTemplatesResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}},
		},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	require.ErrorIs(t, h.maintain(ctx, storage), context.DeadlineExceeded)
	require.Zero(t, locks)
}
