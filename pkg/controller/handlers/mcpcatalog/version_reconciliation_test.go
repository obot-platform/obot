package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadMCPCatalogBuildsVersionFamilyAndProjectsHighestVersion(t *testing.T) {
	dir := t.TempDir()
	writeVersionManifest(t, dir, "v1.yaml", "Tool", "tool", "version: 1\n", "tool-v1")
	writeVersionManifest(t, dir, "v2.yaml", "Tool", "tool", "version: 2\n", "tool-v2")

	objs, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")
	require.NoError(t, err)
	require.Len(t, objs, 3)

	parent := catalogParent(t, objs)
	assert.Equal(t, 2, parent.Spec.DefaultVersion)
	assert.Equal(t, 2, parent.Status.LatestVersion)
	require.NotNil(t, parent.Spec.Manifest.NPXConfig)
	assert.Equal(t, "tool-v2", parent.Spec.Manifest.NPXConfig.Package)

	versions := catalogVersions(objs)
	require.Len(t, versions, 2)
	assert.Equal(t, 1, versions[0].Spec.Version)
	assert.Equal(t, 2, versions[1].Spec.Version)
	assert.True(t, versions[0].Spec.Active)
	assert.True(t, versions[1].Spec.Active)
}

func TestReadMCPCatalogTreatsMissingVersionAsHiddenZero(t *testing.T) {
	dir := t.TempDir()
	writeVersionManifest(t, dir, "entry.yaml", "Tool", "tool", "", "tool")

	objs, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")
	require.NoError(t, err)
	parent := catalogParent(t, objs)
	assert.Zero(t, parent.Spec.DefaultVersion)
	assert.Zero(t, parent.Status.LatestVersion)
	versions := catalogVersions(objs)
	require.Len(t, versions, 1)
	assert.Zero(t, versions[0].Spec.Version)
}

func TestReadMCPCatalogRejectsNonPositiveExplicitVersion(t *testing.T) {
	for _, version := range []string{"0", "-1"} {
		t.Run(version, func(t *testing.T) {
			dir := t.TempDir()
			writeVersionManifest(t, dir, "entry.yaml", "Tool", "tool", "version: "+version+"\n", "tool")

			objs, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")

			assert.ErrorContains(t, err, "explicit version must be positive")
			assert.Empty(t, objs)
		})
	}
}

func TestReadMCPCatalogRejectsInvalidVersionFamilies(t *testing.T) {
	tests := []struct {
		name      string
		second    func(*testing.T, string)
		wantError string
	}{
		{
			name: "duplicate version",
			second: func(t *testing.T, dir string) {
				t.Helper()
				writeVersionManifest(t, dir, "second.yaml", "Tool", "tool", "version: 1\n", "other")
			},
			wantError: "duplicate version 1",
		},
		{
			name: "different exact name",
			second: func(t *testing.T, dir string) {
				t.Helper()
				writeVersionManifest(t, dir, "second.yaml", "tool", "tool", "version: 2\n", "other")
			},
			wantError: "must use exact name",
		},
		{
			name: "different entry key",
			second: func(t *testing.T, dir string) {
				t.Helper()
				writeVersionManifest(t, dir, "second.yaml", "Tool", "other", "version: 2\n", "other")
			},
			wantError: "must use entry key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeVersionManifest(t, dir, "first.yaml", "Tool", "tool", "version: 1\n", "tool")
			tt.second(t, dir)

			objs, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")

			assert.ErrorContains(t, err, tt.wantError)
			assert.Empty(t, objs)
		})
	}
}

func TestReadMCPCatalogAllowsFamilyEntryKeyButRejectsAcrossFamilies(t *testing.T) {
	dir := t.TempDir()
	writeVersionManifest(t, dir, "a-tool-v1.yaml", "Tool", "shared", "version: 1\n", "tool-v1")
	writeVersionManifest(t, dir, "b-tool-v2.yaml", "Tool", "shared", "version: 2\n", "tool-v2")
	writeVersionManifest(t, dir, "c-other.yaml", "Other", "shared", "version: 1\n", "other")

	objs, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")

	assert.ErrorContains(t, err, `duplicate source entry key "shared"`)
	assert.Len(t, objs, 3)
	assert.Equal(t, "Tool", catalogParent(t, objs).Spec.Manifest.Name)
}

func TestReconcileMCPCatalogVersionsUsesLaterWholeFamilyAndPreservesDefault(t *testing.T) {
	firstDir := t.TempDir()
	writeVersionManifest(t, firstDir, "v1.yaml", "Tool", "tool", "version: 1\n", "first-v1")
	writeVersionManifest(t, firstDir, "v2.yaml", "Tool", "tool", "version: 2\n", "first-v2")
	secondDir := t.TempDir()
	writeVersionManifest(t, secondDir, "v2.yaml", "Tool", "tool", "version: 2\n", "second-v2")
	writeVersionManifest(t, secondDir, "v3.yaml", "Tool", "tool", "version: 3\n", "second-v3")

	h := &Handler{}
	first, err := h.readMCPCatalog(t.Context(), "default", firstDir, "")
	require.NoError(t, err)
	second, err := h.readMCPCatalog(t.Context(), "default", secondDir, "")
	require.NoError(t, err)
	for _, version := range catalogVersions(second) {
		if version.Spec.Version == 2 {
			version.Spec.UnsupportedTools = []string{"v2"}
		} else {
			version.Spec.UnsupportedTools = []string{"v3"}
		}
	}
	existing := catalogParent(t, first).DeepCopy()
	existing.Spec.DefaultVersion = 2
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(existing).Build()

	objs, err := h.reconcileMCPCatalogVersions(t.Context(), c, []string{firstDir, secondDir}, nil, append(first, second...))
	require.NoError(t, err)
	parent := catalogParent(t, objs)
	assert.Equal(t, secondDir, parent.Spec.SourceURL)
	assert.Equal(t, 2, parent.Spec.DefaultVersion)
	assert.Equal(t, 3, parent.Status.LatestVersion)
	require.NotNil(t, parent.Spec.Manifest.NPXConfig)
	assert.Equal(t, "second-v2", parent.Spec.Manifest.NPXConfig.Package)
	assert.Equal(t, []string{"v2"}, parent.Spec.UnsupportedTools)
	versions := catalogVersions(objs)
	require.Len(t, versions, 2)
	assert.Equal(t, []int{2, 3}, []int{versions[0].Spec.Version, versions[1].Spec.Version})
}

func TestReconcileMCPCatalogVersionsRetainsReferencedRemovedVersionInactive(t *testing.T) {
	dir := t.TempDir()
	writeVersionManifest(t, dir, "v2.yaml", "Tool", "tool", "version: 2\n", "tool-v2")
	h := &Handler{}
	desired, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	require.NoError(t, err)
	parent := catalogParent(t, desired)
	removed := &v1.MCPServerCatalogEntryVersion{
		ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(parent.Name, 1), Namespace: parent.Namespace},
		Spec: v1.MCPServerCatalogEntryVersionSpec{
			MCPServerCatalogEntryName: parent.Name,
			Version:                   1,
			Manifest:                  testVersionManifest("Tool", "tool-v1"),
			SourceURL:                 "old-source",
			Active:                    true,
		},
	}
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server", Namespace: parent.Namespace},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName:    parent.Name,
			MCPServerCatalogEntryVersion: 1,
		},
	}
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(parent.DeepCopy(), removed, server).Build()

	objs, err := h.reconcileMCPCatalogVersions(t.Context(), c, []string{dir}, nil, desired)
	require.NoError(t, err)
	versions := catalogVersions(objs)
	require.Len(t, versions, 2)
	assert.Equal(t, 1, versions[0].Spec.Version)
	assert.False(t, versions[0].Spec.Active)
	assert.Equal(t, 2, versions[1].Spec.Version)
	assert.True(t, versions[1].Spec.Active)
	assert.Equal(t, 2, catalogParent(t, objs).Status.LatestVersion)
}

func TestReconcileMCPCatalogVersionsPreservesFailedLaterSourceWinner(t *testing.T) {
	firstDir := t.TempDir()
	writeVersionManifest(t, firstDir, "v1.yaml", "Tool", "tool", "version: 1\n", "first-v1")
	secondDir := t.TempDir()
	writeVersionManifest(t, secondDir, "v2.yaml", "Tool", "tool", "version: 2\n", "second-v2")

	h := &Handler{}
	first, err := h.readMCPCatalog(t.Context(), "default", firstDir, "")
	require.NoError(t, err)
	second, err := h.readMCPCatalog(t.Context(), "default", secondDir, "")
	require.NoError(t, err)
	storedParent := catalogParent(t, second).DeepCopy()
	storedVersions := catalogVersions(second)
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).
		WithObjects(storedParent, storedVersions[0].DeepCopy()).Build()

	objs, err := h.reconcileMCPCatalogVersions(t.Context(), c, []string{firstDir, secondDir}, map[string]bool{secondDir: true}, first)
	require.NoError(t, err)
	parent := catalogParent(t, objs)
	assert.Equal(t, secondDir, parent.Spec.SourceURL)
	assert.Equal(t, 2, parent.Spec.DefaultVersion)
	require.NotNil(t, parent.Spec.Manifest.NPXConfig)
	assert.Equal(t, "second-v2", parent.Spec.Manifest.NPXConfig.Package)
	versions := catalogVersions(objs)
	require.Len(t, versions, 1)
	assert.Equal(t, 2, versions[0].Spec.Version)
	assert.True(t, versions[0].Spec.Active)
}

func TestReconcileMCPCatalogVersionsDeactivatesRemovedVersionDuringUnrelatedSourceFailure(t *testing.T) {
	dir := t.TempDir()
	writeVersionManifest(t, dir, "v2.yaml", "Tool", "tool", "version: 2\n", "tool-v2")
	h := &Handler{}
	desired, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	require.NoError(t, err)
	parent := catalogParent(t, desired)
	removed := &v1.MCPServerCatalogEntryVersion{
		ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(parent.Name, 1), Namespace: parent.Namespace},
		Spec: v1.MCPServerCatalogEntryVersionSpec{
			MCPServerCatalogEntryName: parent.Name,
			Version:                   1,
			Manifest:                  testVersionManifest("Tool", "tool-v1"),
			SourceURL:                 dir,
			Active:                    true,
		},
	}
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(parent.DeepCopy(), removed).Build()

	objs, err := h.reconcileMCPCatalogVersions(t.Context(), c, []string{dir, "failed-source"}, map[string]bool{"failed-source": true}, desired)
	require.NoError(t, err)
	versions := catalogVersions(objs)
	require.Len(t, versions, 2)
	assert.Equal(t, 1, versions[0].Spec.Version)
	assert.False(t, versions[0].Spec.Active)
	assert.Equal(t, 2, versions[1].Spec.Version)
	assert.True(t, versions[1].Spec.Active)
}

func TestResolveCompositeSourceRefsUsesProjectedDefaultVersion(t *testing.T) {
	targetDir := t.TempDir()
	writeVersionManifest(t, targetDir, "v1.yaml", "Tool", "tool", "version: 1\n", "tool-v1")
	writeVersionManifest(t, targetDir, "v2.yaml", "Tool", "tool", "version: 2\n", "tool-v2")
	compositeDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(compositeDir, "composite.yaml"), []byte(`entryKey: composite
name: Composite
shortDescription: Composite
description: Composite
icon: icon
runtime: composite
compositeConfig:
  componentServers:
    - catalogEntryID: `+sourceRef(targetDir, "tool")+`
`), 0o600))

	h := &Handler{}
	targets, err := h.readMCPCatalog(t.Context(), "default", targetDir, "")
	require.NoError(t, err)
	composites, err := h.readMCPCatalog(t.Context(), "default", compositeDir, "")
	require.NoError(t, err)
	existing := catalogParent(t, targets).DeepCopy()
	existing.Spec.DefaultVersion = 1
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(existing).Build()

	objs, err := h.reconcileMCPCatalogVersions(t.Context(), c, []string{targetDir, compositeDir}, nil, append(targets, composites...))
	require.NoError(t, err)
	objs, errsBySource := h.resolveCompositeSourceRefs(t.Context(), c, "default", "default", objs)
	require.Empty(t, errsBySource)

	var composite *v1.MCPServerCatalogEntry
	for _, parent := range catalogParents(objs) {
		if parent.Spec.Manifest.Runtime == types.RuntimeComposite {
			composite = parent
		}
	}
	require.NotNil(t, composite)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	require.NotNil(t, component.Manifest.NPXConfig)
	assert.Equal(t, "tool-v1", component.Manifest.NPXConfig.Package)
}

func writeVersionManifest(t *testing.T, dir, file, name, entryKey, version, pkg string) {
	t.Helper()
	content := "entryKey: " + entryKey + "\n" +
		"name: " + name + "\n" +
		version +
		"shortDescription: Test\n" +
		"description: Test\n" +
		"icon: icon\n" +
		"runtime: npx\n" +
		"npxConfig:\n" +
		"  package: " + pkg + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600))
}

func catalogParent(t *testing.T, objs []client.Object) *v1.MCPServerCatalogEntry {
	t.Helper()
	for _, obj := range objs {
		if entry, ok := obj.(*v1.MCPServerCatalogEntry); ok {
			return entry
		}
	}
	require.FailNow(t, "catalog parent not found")
	return nil
}

func catalogVersions(objs []client.Object) []*v1.MCPServerCatalogEntryVersion {
	var versions []*v1.MCPServerCatalogEntryVersion
	for _, obj := range objs {
		if version, ok := obj.(*v1.MCPServerCatalogEntryVersion); ok {
			versions = append(versions, version)
		}
	}
	return versions
}

func catalogParents(objs []client.Object) []*v1.MCPServerCatalogEntry {
	var parents []*v1.MCPServerCatalogEntry
	for _, obj := range objs {
		if parent, ok := obj.(*v1.MCPServerCatalogEntry); ok {
			parents = append(parents, parent)
		}
	}
	return parents
}

func testVersionManifest(name, pkg string) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Name:             name,
		ShortDescription: "Test",
		Description:      "Test",
		Icon:             "icon",
		Runtime:          types.RuntimeNPX,
		NPXConfig:        &types.NPXRuntimeConfig{Package: pkg},
		ServerUserType:   types.ServerUserTypeSingleUser,
	}
}
