package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/publishedartifact"
	"github.com/obot-platform/obot/pkg/skillformat"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/blob"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	maxArtifactUploadBytes    = 100 * 1024 * 1024
	maxZIPFiles               = 50
	maxZIPUncompressedBytes   = 100 * 1024 * 1024
	maxSkillMDBytes           = 1024 * 1024
	maxArtifactDescriptionLen = 1024
	maxPublishRetries         = 3
)

// errConcurrentCreate is returned by createNewArtifact when another request
// created the same artifact first, signaling the retry loop to re-GET and update.
var errConcurrentCreate = fmt.Errorf("concurrent create detected")

type publishedArtifactUpdateRequest struct {
	Description *string         `json:"description,omitempty"`
	Version     *int            `json:"version,omitempty"`
	Subjects    []types.Subject `json:"subjects,omitempty"`
}

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

	// Deterministic name based on author + artifact type + workflow name to prevent duplicate creates.
	artifactName := system.PublishedArtifactPrefix + hash.String(authorID + string(manifest.ArtifactType) + manifest.Name)[:12]

	for attempt := range maxPublishRetries {
		// Try to get the existing artifact by its deterministic name.
		var existing v1.PublishedArtifact
		err := req.Get(&existing, artifactName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		if apierrors.IsNotFound(err) {
			// No existing artifact — try to create a new one.
			if err := h.createNewArtifact(req, data, fm, body, manifest, authorID, authorEmail, artifactName); errors.Is(err, errConcurrentCreate) {
				log.Debugf("Concurrent create for artifact %s (attempt %d/%d), retrying", artifactName, attempt+1, maxPublishRetries)
				continue
			} else if err != nil {
				return err
			}
			return nil
		}

		// Update existing artifact with a new version.
		previousVersionSubjects := publishedartifact.VersionSubjects(&existing, existing.Spec.LatestVersion)
		version := existing.Spec.LatestVersion + 1

		// Stamp publish metadata into SKILL.md before uploading the next version.
		fmCopy := withArtifactMetadata(fm, existing.Name, authorEmail, version)
		rewrittenData, err := rewriteSkillFrontmatterInZIP(data, fmCopy, body)
		if err != nil {
			return types.NewErrBadRequest("failed to rewrite SKILL.md in ZIP: %v", err)
		}

		// Upload to a unique provisional key so concurrent publishes never collide.
		nonce, err := randomHex(8)
		if err != nil {
			return fmt.Errorf("failed to generate upload nonce: %w", err)
		}
		blobKey := fmt.Sprintf("published-artifacts/%s/v%d-%s.zip", existing.Name, version, nonce)
		log.Debugf("Updating existing artifact %s: v%d -> v%d, blobKey=%s", existing.Name, existing.Spec.LatestVersion, version, blobKey)

		if err := h.blobStore.Upload(req.Context(), h.bucket, blobKey, bytes.NewReader(rewrittenData)); err != nil {
			return fmt.Errorf("failed to upload artifact: %w", err)
		}

		existing.Spec.LatestVersion = version
		existing.Spec.BlobKey = blobKey
		existing.Spec.Description = manifest.Description
		existing.Spec.AuthorEmail = manifest.AuthorEmail
		existing.Status.Versions = append(existing.Status.Versions, types.PublishedArtifactVersionEntry{
			Version:     version,
			BlobKey:     blobKey,
			Description: manifest.Description,
			CreatedAt:   *types.NewTime(time.Now()),
			Subjects:    previousVersionSubjects,
		})

		if err := req.Update(&existing); apierrors.IsConflict(err) {
			log.Debugf("Conflict updating artifact %s (attempt %d/%d), retrying", existing.Name, attempt+1, maxPublishRetries)
			// Safe to delete — this key is unique to this request.
			if delErr := h.blobStore.Delete(req.Context(), h.bucket, blobKey); delErr != nil {
				log.Errorf("failed to delete provisional blob %s after conflict: %v", blobKey, delErr)
			}
			continue
		} else if err != nil {
			if delErr := h.blobStore.Delete(req.Context(), h.bucket, blobKey); delErr != nil {
				log.Errorf("failed to delete blob %s after update error: %v", blobKey, delErr)
			}
			return err
		}

		log.Infof("Published artifact %s v%d (updated)", existing.Name, version)
		return req.Write(convertPublishedArtifactForRequester(&existing, req.User, req.UserIsAdmin()))
	}

	return types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("failed to publish artifact after %d attempts due to concurrent updates, please retry", maxPublishRetries))
}

func (h *PublishedArtifactHandler) createNewArtifact(req api.Context, data []byte, fm skillformat.Frontmatter, body string, manifest types.PublishedArtifactManifest, authorID, authorEmail, artifactName string) error {
	// Stamp publish metadata into SKILL.md before uploading the first version.
	fm = withArtifactMetadata(fm, artifactName, authorEmail, 1)
	data, err := rewriteSkillFrontmatterInZIP(data, fm, body)
	if err != nil {
		return types.NewErrBadRequest("failed to rewrite SKILL.md in ZIP: %v", err)
	}

	// Upload to a unique blob key so concurrent creates don't overwrite each other.
	nonce, err := randomHex(8)
	if err != nil {
		return fmt.Errorf("failed to generate upload nonce: %w", err)
	}
	blobKey := fmt.Sprintf("published-artifacts/%s/v1-%s.zip", artifactName, nonce)
	log.Debugf("Uploading blob for new artifact %s v1, blobKey=%s", artifactName, blobKey)

	if err := h.blobStore.Upload(req.Context(), h.bucket, blobKey, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to upload artifact: %w", err)
	}

	artifact := v1.PublishedArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      artifactName,
			Namespace: req.Namespace(),
		},
		Spec: v1.PublishedArtifactSpec{
			PublishedArtifactManifest: manifest,
			AuthorID:                  authorID,
			LatestVersion:             1,
			BlobKey:                   blobKey,
		},
		Status: v1.PublishedArtifactStatus{
			Versions: []types.PublishedArtifactVersionEntry{
				{
					Version:     1,
					BlobKey:     blobKey,
					Description: manifest.Description,
					CreatedAt:   *types.NewTime(time.Now()),
				},
			},
		},
	}

	if err := req.Create(&artifact); apierrors.IsAlreadyExists(err) {
		// Another concurrent request created this artifact first — clean up our blob
		// and return errConcurrentCreate so the caller's retry loop re-reads and takes the update path.
		if delErr := h.blobStore.Delete(req.Context(), h.bucket, blobKey); delErr != nil {
			log.Errorf("failed to delete blob %s after AlreadyExists: %v", blobKey, delErr)
		}
		return errConcurrentCreate
	} else if err != nil {
		if delErr := h.blobStore.Delete(req.Context(), h.bucket, blobKey); delErr != nil {
			log.Errorf("failed to delete blob %s after create error: %v", blobKey, delErr)
		}
		return err
	}

	log.Infof("Published artifact %s v1 (new, id=%s)", manifest.Name, artifact.Name)
	return req.WriteCreated(convertPublishedArtifactForRequester(&artifact, req.User, req.UserIsAdmin()))
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

	log.Debugf("Listing artifacts: type=%q query=%q userID=%q isAdmin=%v totalInDB=%d", artifactType, query, req.User.GetUID(), req.UserIsAdmin(), len(artifacts.Items))

	items := make([]types.PublishedArtifact, 0, len(artifacts.Items))
	for i := range artifacts.Items {
		a := &artifacts.Items[i]

		if !publishedartifact.CanAccess(a, req.User, req.UserIsAdmin()) {
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

		items = append(items, convertPublishedArtifactForRequester(a, req.User, req.UserIsAdmin()))
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

	return req.Write(convertPublishedArtifactForRequester(&artifact, req.User, req.UserIsAdmin()))
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

	version := publishedartifact.DefaultDownloadVersion(&artifact, req.User, req.UserIsAdmin())
	if version == 0 {
		return types.NewErrNotFound("artifact %s not found", id)
	}
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

	// Sanitize the artifact name for use in the Content-Disposition header to prevent
	// header injection via quotes or control characters in the name.
	safeName := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' || r < 0x20 {
			return '_'
		}
		return r
	}, artifact.Spec.Name)
	req.ResponseWriter.Header().Set("Content-Type", "application/zip")
	req.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-v%d.zip"`, safeName, version))
	_, err = io.Copy(req.ResponseWriter, reader)
	return err
}

func (h *PublishedArtifactHandler) GetSkillMD(req api.Context) error {
	if err := h.checkConfigured(); err != nil {
		return err
	}

	id := req.PathValue("id")
	var artifact v1.PublishedArtifact
	if err := req.Get(&artifact, id); err != nil {
		return err
	}

	version, err := strconv.Atoi(req.PathValue("version"))
	if err != nil || version < 1 {
		return types.NewErrBadRequest("invalid version: %s", req.PathValue("version"))
	}

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

	reader, err := h.blobStore.Download(req.Context(), h.bucket, blobKey)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxArtifactUploadBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read artifact blob: %w", err)
	}
	if len(data) > maxArtifactUploadBytes {
		return fmt.Errorf("artifact blob exceeds maximum size")
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to read artifact ZIP: %w", err)
	}

	for _, f := range r.File {
		if f.Name != skillformat.SkillMainFile {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", skillformat.SkillMainFile, err)
		}
		defer rc.Close()

		content, err := io.ReadAll(io.LimitReader(rc, maxSkillMDBytes+1))
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", skillformat.SkillMainFile, err)
		}
		if len(content) > maxSkillMDBytes {
			return fmt.Errorf("%s exceeds maximum size of %d bytes", skillformat.SkillMainFile, maxSkillMDBytes)
		}

		req.ResponseWriter.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, err = req.ResponseWriter.Write(content)
		return err
	}

	return types.NewErrNotFound("%s not found in artifact", skillformat.SkillMainFile)
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

	var update publishedArtifactUpdateRequest
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
	if update.Subjects != nil {
		if err := validatePublishedArtifactSubjects(update.Subjects); err != nil {
			return types.NewErrBadRequest("invalid subjects: %v", err)
		}
		version := artifact.Spec.LatestVersion
		if update.Version != nil {
			if *update.Version < 1 {
				return types.NewErrBadRequest("version must be >= 1")
			}
			version = *update.Version
		}
		entry := publishedartifact.VersionEntry(&artifact, version)
		if entry == nil {
			return types.NewErrNotFound("version %d not found", version)
		}
		log.Debugf("Updating artifact %s version %d subjects (count=%d)", id, version, len(update.Subjects))
		entry.Subjects = update.Subjects
	}

	if err := req.Update(&artifact); err != nil {
		return err
	}

	log.Infof("Updated artifact %s (%s)", id, artifact.Spec.Name)
	return req.Write(convertPublishedArtifactForRequester(&artifact, req.User, req.UserIsAdmin()))
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

func validatePublishedArtifactSubjects(subjects []types.Subject) error {
	seen := make(map[types.Subject]struct{}, len(subjects))
	for _, subject := range subjects {
		if err := subject.Validate(); err != nil {
			return err
		}
		if subject.ID == "*" && subject.Type != types.SubjectTypeSelector {
			return fmt.Errorf("wildcard subject (*) must use selector type")
		}
		if subject.ID == "*" && len(subjects) > 1 {
			return fmt.Errorf("wildcard subject (*) must be the only subject")
		}
		if _, ok := seen[subject]; ok {
			return fmt.Errorf("duplicate subject: %s/%s", subject.Type, subject.ID)
		}
		seen[subject] = struct{}{}
	}

	return nil
}

// validateZIP checks the ZIP archive for file count limits, total declared uncompressed size,
// suspicious entry names (path traversal, absolute paths, symlinks), and Windows-style paths.
// Note: declared sizes from headers are attacker-controlled; callers that decompress content
// must also enforce limits on actual bytes read (see rewriteSkillFrontmatterInZIP).
func validateZIP(r *zip.Reader) error {
	if len(r.File) > maxZIPFiles {
		return fmt.Errorf("ZIP contains too many files (%d, max %d)", len(r.File), maxZIPFiles)
	}

	var totalUncompressed uint64
	for _, f := range r.File {
		if err := validateZIPEntryName(f.Name); err != nil {
			return err
		}

		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("ZIP entry %q is a symbolic link", f.Name)
		}
		if mode := f.Mode(); !mode.IsDir() && !mode.IsRegular() && mode&os.ModeType != 0 {
			return fmt.Errorf("ZIP entry %q has unsupported file type", f.Name)
		}

		if f.UncompressedSize64 > uint64(maxZIPUncompressedBytes) {
			return fmt.Errorf("ZIP entry %q uncompressed size exceeds limit (%d bytes)", f.Name, maxZIPUncompressedBytes)
		}
		if totalUncompressed > uint64(maxZIPUncompressedBytes)-f.UncompressedSize64 {
			return fmt.Errorf("ZIP total uncompressed size exceeds limit (%d bytes)", maxZIPUncompressedBytes)
		}
		totalUncompressed += f.UncompressedSize64
	}

	return nil
}

// validateZIPEntryName checks a single ZIP entry name for path traversal, absolute paths,
// and Windows drive-letter paths.
func validateZIPEntryName(name string) error {
	// Normalize backslashes to forward slashes before cleaning.
	normalized := strings.ReplaceAll(filepath.ToSlash(name), "\\", "/")
	cleaned := path.Clean(strings.TrimPrefix(normalized, "./"))

	// Reject entries that are empty or resolve to the current directory.
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf("ZIP entry %q is empty or refers to the current directory", name)
	}
	if strings.HasPrefix(cleaned, "/") {
		return fmt.Errorf("ZIP entry %q is an absolute path", name)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("ZIP entry %q contains path traversal", name)
	}
	// Reject Windows drive-letter paths (e.g. "C:\..." or "C:/...")
	if len(cleaned) >= 2 && cleaned[1] == ':' &&
		((cleaned[0] >= 'a' && cleaned[0] <= 'z') || (cleaned[0] >= 'A' && cleaned[0] <= 'Z')) {
		return fmt.Errorf("ZIP entry %q is an absolute path", name)
	}
	if volume := filepath.VolumeName(filepath.FromSlash(cleaned)); volume != "" {
		return fmt.Errorf("ZIP entry %q is an absolute path", name)
	}
	return nil
}

// readSkillFrontmatterFromZIP finds SKILL.md in the ZIP, parses its frontmatter,
// and returns the frontmatter and body separately.
func readSkillFrontmatterFromZIP(data []byte) (skillformat.Frontmatter, string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return skillformat.Frontmatter{}, "", fmt.Errorf("invalid ZIP archive: %w", err)
	}

	if err := validateZIP(r); err != nil {
		return skillformat.Frontmatter{}, "", err
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

	if err := validateZIP(r); err != nil {
		return nil, err
	}

	newContent, err := skillformat.FormatSkillMD(fm, body)
	if err != nil {
		return nil, fmt.Errorf("failed to format %s: %w", skillformat.SkillMainFile, err)
	}

	var (
		buf          bytes.Buffer
		foundSkillMD bool
		totalWritten uint64
	)
	w := zip.NewWriter(&buf)

	for _, f := range r.File {
		if f.Name == skillformat.SkillMainFile {
			foundSkillMD = true
			fw, err := w.Create(f.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to create %s entry: %w", skillformat.SkillMainFile, err)
			}
			n, err := fw.Write([]byte(newContent))
			if err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", skillformat.SkillMainFile, err)
			}
			totalWritten += uint64(n)
			if totalWritten > uint64(maxZIPUncompressedBytes) {
				return nil, fmt.Errorf("ZIP total uncompressed size exceeds limit (%d bytes)", maxZIPUncompressedBytes)
			}
			continue
		}

		// Copy other files unchanged, enforcing the total uncompressed size limit
		// based on actual bytes read rather than trusting header metadata.
		fw, err := w.CreateHeader(&f.FileHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to create entry %s: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open entry %s: %w", f.Name, err)
		}
		remaining := uint64(maxZIPUncompressedBytes) - totalWritten
		n, err := io.Copy(fw, io.LimitReader(rc, int64(remaining)+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to copy entry %s: %w", f.Name, err)
		}
		totalWritten += uint64(n)
		if totalWritten > uint64(maxZIPUncompressedBytes) {
			return nil, fmt.Errorf("ZIP total uncompressed size exceeds limit (%d bytes)", maxZIPUncompressedBytes)
		}
	}

	if !foundSkillMD {
		return nil, fmt.Errorf("%s not found in ZIP archive", skillformat.SkillMainFile)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close ZIP writer: %w", err)
	}

	return buf.Bytes(), nil
}

func withArtifactMetadata(fm skillformat.Frontmatter, artifactID, authorEmail string, version int) skillformat.Frontmatter {
	fmCopy := fm
	if len(fm.Metadata) > 0 {
		fmCopy.Metadata = make(map[string]string, len(fm.Metadata)+3)
		for k, v := range fm.Metadata {
			fmCopy.Metadata[k] = v
		}
	} else {
		fmCopy.Metadata = make(map[string]string, 3)
	}
	fmCopy.Metadata["id"] = artifactID
	fmCopy.Metadata["author-email"] = authorEmail
	fmCopy.Metadata["version"] = strconv.Itoa(version)
	return fmCopy
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func convertPublishedArtifactForRequester(a *v1.PublishedArtifact, requester user.Info, isAdmin bool) types.PublishedArtifact {
	versions := visibleVersionSummaries(a, requester, isAdmin)
	latestVersion := a.Spec.LatestVersion
	if requester == nil || (a.Spec.AuthorID != requester.GetUID() && !isAdmin) {
		latestVersion = 0
		for _, version := range versions {
			if version.Version > latestVersion {
				latestVersion = version.Version
			}
		}
	}

	return types.PublishedArtifact{
		Metadata:                  MetadataFrom(a),
		PublishedArtifactManifest: a.Spec.PublishedArtifactManifest,
		DisplayName:               skillformat.DisplayName(a.Spec.Name),
		AuthorID:                  a.Spec.AuthorID,
		LatestVersion:             latestVersion,
		Versions:                  versions,
	}
}

func visibleVersionSummaries(a *v1.PublishedArtifact, requester user.Info, isAdmin bool) []types.PublishedArtifactVersionSummary {
	if requester == nil || len(a.Status.Versions) == 0 {
		return nil
	}

	result := make([]types.PublishedArtifactVersionSummary, 0, len(a.Status.Versions))
	for _, version := range a.Status.Versions {
		if a.Spec.AuthorID != requester.GetUID() && !isAdmin && !publishedartifact.SubjectsContainUser(version.Subjects, requester) {
			continue
		}
		result = append(result, types.PublishedArtifactVersionSummary{
			Version:     version.Version,
			Description: version.Description,
			CreatedAt:   version.CreatedAt,
			Subjects:    version.Subjects,
		})
	}
	return result
}
