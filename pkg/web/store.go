package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOStore manages test plan storage in MinIO
type MinIOStore struct {
	client *minio.Client
	bucket string
}

// StoredVersion represents a stored test plan version
type StoredVersion struct {
	ID          string         `json:"id"`
	Tag         string         `json:"tag"`
	CreatedAt   time.Time      `json:"createdAt"`
	Description string         `json:"description"`
	TotalTests  int            `json:"totalTests"`
	Features    int            `json:"features"`
	Categories  map[string]int `json:"categories"`

	// Generation context — identifies WHY this version differs from others
	GenerationContext *GenerationContext `json:"generationContext,omitempty"`
}

// GenerationContext captures the inputs and tool state that produced this version
type GenerationContext struct {
	GeneratedAt        string            `json:"generatedAt"`        // from YAML generatedAt field
	ToolVersion        string            `json:"toolVersion"`        // testgen suite version (e.g. "1.0")
	ToolCommit         string            `json:"toolCommit"`         // git commit hash of the tool
	SourceYangDir      string            `json:"sourceYangDir"`      // YANG model directory path
	SourceRESTAPI      string            `json:"sourceRestApi"`      // REST API spec path
	SourceNOSAPI       string            `json:"sourceNosApi"`       // NOSAPI spec path
	SourceFingerprints map[string]string `json:"sourceFingerprints"` // file path -> SHA256 hash
}

// NewMinIOStore creates a new MinIO storage client
func NewMinIOStore(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIOStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		log.Printf("Created MinIO bucket: %s", bucket)
	}

	return &MinIOStore{client: client, bucket: bucket}, nil
}

// StoreVersion uploads all test plan files for a version
func (s *MinIOStore) StoreVersion(version *StoredVersion, files map[string][]byte) error {
	ctx := context.Background()

	// Store version metadata
	metaJSON, err := json.Marshal(version)
	if err != nil {
		return fmt.Errorf("failed to marshal version metadata: %w", err)
	}
	metaKey := fmt.Sprintf("versions/%s/metadata.json", version.ID)
	_, err = s.client.PutObject(ctx, s.bucket, metaKey, bytes.NewReader(metaJSON), int64(len(metaJSON)),
		minio.PutObjectOptions{ContentType: "application/json"})
	if err != nil {
		return fmt.Errorf("failed to store version metadata: %w", err)
	}

	// Store each file
	for name, data := range files {
		key := fmt.Sprintf("versions/%s/plans/%s", version.ID, name)
		contentType := "application/x-yaml"
		if strings.HasSuffix(name, ".html") {
			contentType = "text/html"
		} else if strings.HasSuffix(name, ".json") {
			contentType = "application/json"
		}
		_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
			minio.PutObjectOptions{ContentType: contentType})
		if err != nil {
			return fmt.Errorf("failed to store file %s: %w", name, err)
		}
	}

	return nil
}

// ListVersions returns all stored versions sorted by creation time
func (s *MinIOStore) ListVersions() ([]StoredVersion, error) {
	ctx := context.Background()
	var versions []StoredVersion

	// List all version metadata files
	prefix := "versions/"
	objectCh := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range objectCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("error listing objects: %w", obj.Err)
		}
		if !strings.HasSuffix(obj.Key, "/metadata.json") {
			continue
		}
		version, err := s.getVersionMetadata(ctx, obj.Key)
		if err != nil {
			log.Printf("Warning: failed to read version %s: %v", obj.Key, err)
			continue
		}
		versions = append(versions, *version)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].CreatedAt.After(versions[j].CreatedAt)
	})

	return versions, nil
}

// GetVersion returns metadata for a specific version
func (s *MinIOStore) GetVersion(id string) (*StoredVersion, error) {
	key := fmt.Sprintf("versions/%s/metadata.json", id)
	return s.getVersionMetadata(context.Background(), key)
}

// GetVersionFiles returns all test plan files for a version
func (s *MinIOStore) GetVersionFiles(id string) (map[string][]byte, error) {
	ctx := context.Background()
	files := make(map[string][]byte)
	prefix := fmt.Sprintf("versions/%s/plans/", id)

	objectCh := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range objectCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("error listing objects: %w", obj.Err)
		}
		data, err := s.getObject(ctx, obj.Key)
		if err != nil {
			return nil, err
		}
		// Strip prefix to get relative path
		relPath := strings.TrimPrefix(obj.Key, prefix)
		files[relPath] = data
	}

	return files, nil
}

// GetFile returns a specific file from a version
func (s *MinIOStore) GetFile(versionID, filePath string) ([]byte, error) {
	key := fmt.Sprintf("versions/%s/plans/%s", versionID, filePath)
	return s.getObject(context.Background(), key)
}

// GetLatestVersionID returns the ID of the latest version
func (s *MinIOStore) GetLatestVersionID() (string, error) {
	versions, err := s.ListVersions()
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found")
	}
	return versions[0].ID, nil
}

func (s *MinIOStore) getVersionMetadata(ctx context.Context, key string) (*StoredVersion, error) {
	data, err := s.getObject(ctx, key)
	if err != nil {
		return nil, err
	}
	var version StoredVersion
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, fmt.Errorf("failed to unmarshal version: %w", err)
	}
	return &version, nil
}

func (s *MinIOStore) getObject(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %s: %w", key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read object %s: %w", key, err)
	}
	return data, nil
}

// ListTestPlanFiles lists test plan files for the latest version or a given version
func (s *MinIOStore) ListTestPlanFiles(versionID string) ([]TestPlanFile, error) {
	files, err := s.GetVersionFiles(versionID)
	if err != nil {
		return nil, err
	}

	var result []TestPlanFile
	for name, data := range files {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		category := ""
		feature := name
		if len(parts) == 2 {
			category = parts[0]
			feature = strings.TrimSuffix(parts[1], ".yaml")
		}
		result = append(result, TestPlanFile{
			Category: category,
			Feature:  feature,
			FileName: filepath.Base(name),
			Path:     name,
			Size:     len(data),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Feature < result[j].Feature
	})

	return result, nil
}

// TestPlanFile represents a test plan file listing entry
type TestPlanFile struct {
	Category string `json:"category"`
	Feature  string `json:"feature"`
	FileName string `json:"fileName"`
	Path     string `json:"path"`
	Size     int    `json:"size"`
}
