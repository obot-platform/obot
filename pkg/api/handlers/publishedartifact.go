package handlers

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/skillformat"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/blob"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	maxArtifactUploadBytes    = 100 * 1024 * 1024
	maxZIPFiles               = 50
	maxZIPUncompressedBytes   = 100 * 1024 * 1024
	maxSkillMDBytes           = 1024 * 1024
	maxArtifactDescriptionLen = 1024
)

type PublishedArtifactHandler struct {
	blobStore blob.BlobStore
	bucket    string
}

func NewPublishedArtifactHandler(blobStore blob.BlobStore, bucket string) *PublishedArtifactHandler {
	return &PublishedArtifactHandler{
		blobStore: blobStore,
		bucket:    bucket,
	}
}

func (h *PublishedArtifactHandler) checkConfigured() error {
	if h.blobStore == nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "published artifact storage is not configured")
	}
	return nil
}

func (h *PublishedArtifactHandler) Create(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	data, err := req.Body(api.BodyOptions{MaxBytes: maxArtifactUploadBytes})
	if err != nil {
		return err
	}

	log.Debugf("Received artifact upload (%d bytes)", len(data))

	fm, body, err := readSkillFrontmatterFromZIP(data)
	if err != nil {
		return types.NewErrBadRequest("invalid artifact ZIP: %v", err)
	}

	log.Debugf("Parsed SKILL.md from ZIP: name=%q description=%q", fm.Name, fm.Description)

	if fm.Description == "" {
		return types.NewErrBadRequest("description is required — add a description to the SKILL.md frontmatter before publishing")
	}

	authorID := req.User.GetUID()
	authorEmail := auth.FirstExtraValue(req.User.GetExtra(), "email")

	log.Debugf("Artifact author: id=%q email=%q", authorID, authorEmail)

	// Build the manifest for DB storage from SKILL.md frontmatter.
	manifest := types.PublishedArtifactManifest{
		Name:         fm.Name,
		Description:  fm.Description,
		ArtifactType: types.PublishedArtifactTypeWorkflow,
		AuthorEmail:  authorEmail,
	}

	// Auto-version: look for existing artifact with same name + type + author
	var existing *v1.PublishedArtifact
	var artifacts v1.PublishedArtifactList
	if err := req.List(&artifacts, kclient.MatchingFields{
		"spec.authorID":     authorID,
		"spec.artifactType": string(manifest.ArtifactType),
	}); err != nil {
		return err
	}
	for i := range artifacts.Items {
		if artifacts.Items[i].Spec.Name == manifest.Name {
			existing = &artifacts.Items[i]
			break
		}
	}

	var version int
	if existing != nil {
		version = existing.Spec.LatestVersion + 1
	} else {
		version = 1
	}

	// Inject author email and version into SKILL.md frontmatter metadata and rewrite the ZIP.
	if fm.Metadata == nil {
		fm.Metadata = make(map[string]string)
	}
	fm.Metadata["author-email"] = authorEmail
	fm.Metadata["version"] = fmt.Sprintf("%d", version)
	data, err = rewriteSkillFrontmatterInZIP(data, fm, body)
	if err != nil {
		return types.NewErrBadRequest("failed to rewrite SKILL.md in ZIP: %v", err)
	}

	if existing != nil {
		// Increment version on existing artifact
		blobKey := fmt.Sprintf("published-artifacts/%s/v%d.zip", existing.Name, version)

		log.Debugf("Updating existing artifact %s: v%d -> v%d, blobKey=%s", existing.Name, existing.Spec.LatestVersion, version, blobKey)

		if err := h.blobStore.Upload(req.Context(), h.bucket, blobKey, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("failed to upload artifact: %w", err)
		}

		existing.Spec.LatestVersion = version
		existing.Spec.BlobKey = blobKey
		existing.Spec.Description = manifest.Description
		existing.Status.Versions = append(existing.Status.Versions, types.PublishedArtifactVersionEntry{
			Version:     version,
			BlobKey:     blobKey,
			Description: manifest.Description,
			CreatedAt:   *types.NewTime(time.Now()),
		})

		if err := req.Update(existing); err != nil {
			return err
		}

		log.Infof("Published artifact %s v%d (updated)", existing.Name, version)
		return req.Write(convertPublishedArtifact(existing))
	}

	// Create new artifact
	artifact := v1.PublishedArtifact{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.PublishedArtifactPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.PublishedArtifactSpec{
			PublishedArtifactManifest: manifest,
			AuthorID:                  authorID,
			LatestVersion:             1,
			Visibility:                types.PublishedArtifactVisibilityPrivate,
		},
	}

	if err := req.Create(&artifact); err != nil {
		return err
	}

	blobKey := fmt.Sprintf("published-artifacts/%s/v1.zip", artifact.Name)
	log.Debugf("Creating new artifact %s v1, blobKey=%s", artifact.Name, blobKey)

	if err := h.blobStore.Upload(req.Context(), h.bucket, blobKey, bytes.NewReader(data)); err != nil {
		// Clean up the created resource on upload failure
		log.Errorf("Blob upload failed for %s, cleaning up resource: %v", artifact.Name, err)
		_ = req.Delete(&artifact)
		return fmt.Errorf("failed to upload artifact: %w", err)
	}

	artifact.Spec.BlobKey = blobKey
	artifact.Status.Versions = []types.PublishedArtifactVersionEntry{
		{
			Version:     1,
			BlobKey:     blobKey,
			Description: manifest.Description,
			CreatedAt:   *types.NewTime(time.Now()),
		},
	}

	if err := req.Update(&artifact); err != nil {
		return err
	}

	log.Infof("Published artifact %s v1 (new, id=%s)", manifest.Name, artifact.Name)
	return req.WriteCreated(convertPublishedArtifact(&artifact))
}

func (h *PublishedArtifactHandler) List(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	var artifacts v1.PublishedArtifactList
	var listOpts []kclient.ListOption

	artifactType := req.URL.Query().Get("type")
	if artifactType != "" {
		listOpts = append(listOpts, kclient.MatchingFields{
			"spec.artifactType": artifactType,
		})
	}

	if err := req.List(&artifacts, listOpts...); err != nil {
		return err
	}

	query := strings.ToLower(req.URL.Query().Get("q"))
	userID := req.User.GetUID()
	isAdmin := req.UserIsAdmin()

	log.Debugf("Listing artifacts: type=%q query=%q userID=%q isAdmin=%v totalInDB=%d", artifactType, query, userID, isAdmin, len(artifacts.Items))

	items := make([]types.PublishedArtifact, 0, len(artifacts.Items))
	for i := range artifacts.Items {
		a := &artifacts.Items[i]

		// Visibility filter: public, or owned by requester, or admin
		if a.Spec.Visibility != types.PublishedArtifactVisibilityPublic &&
			a.Spec.AuthorID != userID && !isAdmin {
			continue
		}

		// Text search filter
		if query != "" {
			nameMatch := strings.Contains(strings.ToLower(a.Spec.Name), query)
			displayNameMatch := strings.Contains(strings.ToLower(skillformat.DisplayName(a.Spec.Name)), query)
			descMatch := strings.Contains(strings.ToLower(a.Spec.Description), query)
			if !nameMatch && !displayNameMatch && !descMatch {
				continue
			}
		}

		items = append(items, convertPublishedArtifact(a))
	}

	log.Debugf("Returning %d artifacts (filtered from %d)", len(items), len(artifacts.Items))
	return req.Write(types.PublishedArtifactList{Items: items})
}

func (h *PublishedArtifactHandler) Get(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	var artifact v1.PublishedArtifact
	if err := req.Get(&artifact, req.PathValue("id")); err != nil {
		return err
	}

	if err := h.checkVisibility(&artifact, req); err != nil {
		return err
	}

	return req.Write(convertPublishedArtifact(&artifact))
}

func (h *PublishedArtifactHandler) Download(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	id := req.PathValue("id")
	var artifact v1.PublishedArtifact
	if err := req.Get(&artifact, id); err != nil {
		return err
	}

	if err := h.checkVisibility(&artifact, req); err != nil {
		return err
	}

	version := artifact.Spec.LatestVersion
	if v := req.URL.Query().Get("version"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			return types.NewErrBadRequest("invalid version: %s", v)
		}
		version = parsed
	}

	log.Debugf("Download requested: artifact=%s name=%q version=%d", id, artifact.Spec.Name, version)

	// Find the blob key for the requested version
	blobKey := ""
	for _, entry := range artifact.Status.Versions {
		if entry.Version == version {
			blobKey = entry.BlobKey
			break
		}
	}
	if blobKey == "" {
		return types.NewErrNotFound("version %d not found", version)
	}

	log.Debugf("Downloading blob: bucket=%s key=%s", h.bucket, blobKey)

	reader, err := h.blobStore.Download(req.Context(), h.bucket, blobKey)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	defer reader.Close()

	req.ResponseWriter.Header().Set("Content-Type", "application/zip")
	req.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-v%d.zip"`, artifact.Spec.Name, version))
	_, err = io.Copy(req.ResponseWriter, reader)
	return err
}

func (h *PublishedArtifactHandler) Update(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	id := req.PathValue("id")
	var artifact v1.PublishedArtifact
	if err := req.Get(&artifact, id); err != nil {
		return err
	}

	if err := h.checkOwnership(&artifact, req); err != nil {
		return err
	}

	var update struct {
		Description *string                            `json:"description,omitempty"`
		Visibility  *types.PublishedArtifactVisibility `json:"visibility,omitempty"`
	}
	if err := req.Read(&update); err != nil {
		return err
	}

	if update.Description != nil {
		if *update.Description == "" {
			return types.NewErrBadRequest("description must not be empty")
		}
		if len(*update.Description) > maxArtifactDescriptionLen {
			return types.NewErrBadRequest("description must be %d characters or fewer", maxArtifactDescriptionLen)
		}
		log.Debugf("Updating artifact %s description: %q -> %q", id, artifact.Spec.Description, *update.Description)
		artifact.Spec.Description = *update.Description
	}
	if update.Visibility != nil {
		if *update.Visibility != types.PublishedArtifactVisibilityPrivate &&
			*update.Visibility != types.PublishedArtifactVisibilityPublic {
			return types.NewErrBadRequest("invalid visibility: %s", *update.Visibility)
		}
		log.Debugf("Updating artifact %s visibility: %q -> %q", id, artifact.Spec.Visibility, *update.Visibility)
		artifact.Spec.Visibility = *update.Visibility
	}

	if err := req.Update(&artifact); err != nil {
		return err
	}

	log.Infof("Updated artifact %s (%s)", id, artifact.Spec.Name)
	return req.Write(convertPublishedArtifact(&artifact))
}

func (h *PublishedArtifactHandler) Delete(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	id := req.PathValue("id")
	var artifact v1.PublishedArtifact
	if err := req.Get(&artifact, id); err != nil {
		return err
	}

	if err := h.checkOwnership(&artifact, req); err != nil {
		return err
	}

	log.Debugf("Deleting artifact %s (%s), removing %d version blobs", id, artifact.Spec.Name, len(artifact.Status.Versions))

	// Delete all version blobs
	for _, entry := range artifact.Status.Versions {
		if err := h.blobStore.Delete(req.Context(), h.bucket, entry.BlobKey); err != nil {
			log.Errorf("Failed to delete blob %s for artifact %s: %v", entry.BlobKey, id, err)
		}
	}

	if err := req.Delete(&artifact); err != nil {
		return err
	}

	log.Infof("Deleted artifact %s (%s)", id, artifact.Spec.Name)
	return nil
}

func (h *PublishedArtifactHandler) checkVisibility(artifact *v1.PublishedArtifact, req api.Context) error {
	if artifact.Spec.Visibility == types.PublishedArtifactVisibilityPublic {
		return nil
	}
	if artifact.Spec.AuthorID == req.User.GetUID() || req.UserIsAdmin() {
		return nil
	}
	return types.NewErrNotFound("artifact %s not found", req.PathValue("id"))
}

func (h *PublishedArtifactHandler) checkOwnership(artifact *v1.PublishedArtifact, req api.Context) error {
	if artifact.Spec.AuthorID == req.User.GetUID() || req.UserIsAdmin() {
		return nil
	}
	return types.NewErrHTTP(http.StatusForbidden, "you do not have permission to modify this artifact")
}

// readSkillFrontmatterFromZIP finds SKILL.md in the ZIP, parses its frontmatter,
// and returns the frontmatter and body separately.
func readSkillFrontmatterFromZIP(data []byte) (skillformat.Frontmatter, string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return skillformat.Frontmatter{}, "", fmt.Errorf("invalid ZIP archive: %w", err)
	}

	for _, f := range r.File {
		if f.Name == skillformat.SkillMainFile {
			rc, err := f.Open()
			if err != nil {
				return skillformat.Frontmatter{}, "", fmt.Errorf("failed to open %s: %w", skillformat.SkillMainFile, err)
			}
			defer rc.Close()

			content, err := io.ReadAll(io.LimitReader(rc, maxSkillMDBytes+1))
			if err != nil {
				return skillformat.Frontmatter{}, "", fmt.Errorf("failed to read %s: %w", skillformat.SkillMainFile, err)
			}
			if len(content) > maxSkillMDBytes {
				return skillformat.Frontmatter{}, "", fmt.Errorf("%s exceeds maximum size of %d bytes", skillformat.SkillMainFile, maxSkillMDBytes)
			}

			fm, body, err := skillformat.ParseAndValidateFrontmatter(string(content))
			if err != nil {
				return skillformat.Frontmatter{}, "", fmt.Errorf("invalid %s: %w", skillformat.SkillMainFile, err)
			}
			return fm, body, nil
		}
	}

	return skillformat.Frontmatter{}, "", fmt.Errorf("%s not found in ZIP", skillformat.SkillMainFile)
}

// rewriteSkillFrontmatterInZIP replaces the SKILL.md in the ZIP with updated frontmatter,
// preserving the body content and all other files.
func rewriteSkillFrontmatterInZIP(data []byte, fm skillformat.Frontmatter, body string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP archive: %w", err)
	}

	newContent, err := skillformat.FormatSkillMD(fm, body)
	if err != nil {
		return nil, fmt.Errorf("failed to format %s: %w", skillformat.SkillMainFile, err)
	}

	if len(r.File) > maxZIPFiles {
		return nil, fmt.Errorf("ZIP contains too many files (%d, max %d)", len(r.File), maxZIPFiles)
	}

	var totalUncompressed uint64
	for _, f := range r.File {
		totalUncompressed += f.UncompressedSize64
		if totalUncompressed > maxZIPUncompressedBytes {
			return nil, fmt.Errorf("ZIP uncompressed size exceeds limit (%d bytes)", maxZIPUncompressedBytes)
		}
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	for _, f := range r.File {
		if f.Name == skillformat.SkillMainFile {
			fw, err := w.Create(f.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to create %s entry: %w", skillformat.SkillMainFile, err)
			}
			if _, err := fw.Write([]byte(newContent)); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", skillformat.SkillMainFile, err)
			}
			continue
		}

		// Copy other files unchanged
		fw, err := w.CreateHeader(&f.FileHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to create entry %s: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open entry %s: %w", f.Name, err)
		}
		lr := io.LimitReader(rc, int64(f.UncompressedSize64)+1)
		n, err := io.Copy(fw, lr)
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("failed to copy entry %s: %w", f.Name, err)
		}
		rc.Close()
		if n > int64(f.UncompressedSize64) {
			return nil, fmt.Errorf("entry %s exceeds declared uncompressed size", f.Name)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close ZIP writer: %w", err)
	}

	return buf.Bytes(), nil
}

func convertPublishedArtifact(a *v1.PublishedArtifact) types.PublishedArtifact {
	return types.PublishedArtifact{
		Metadata:                  MetadataFrom(a),
		PublishedArtifactManifest: a.Spec.PublishedArtifactManifest,
		DisplayName:               skillformat.DisplayName(a.Spec.Name),
		AuthorID:                  a.Spec.AuthorID,
		LatestVersion:             a.Spec.LatestVersion,
		Visibility:                a.Spec.Visibility,
	}
}
