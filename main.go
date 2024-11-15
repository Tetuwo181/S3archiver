package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Uploader manages S3 operations
type S3Uploader struct {
	Client *s3.Client
}

// ArchivedFiles represents the structure of archived files in JSON
type ArchivedFiles struct {
	Files []string `json:"files"`
}

// LoadArchivedFiles loads archived files from the JSON file
func LoadArchivedFiles(filename string) (ArchivedFiles, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return ArchivedFiles{Files: []string{}}, nil // Return empty list if file doesn't exist
		}
		return ArchivedFiles{}, err
	}

	var archived ArchivedFiles
	err = json.Unmarshal(data, &archived)
	return archived, err
}

// SaveArchivedFiles saves the updated archived files to the JSON file
func SaveArchivedFiles(filename string, archived ArchivedFiles) error {
	data, err := json.MarshalIndent(archived, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, 0644)
}

// ListS3Files retrieves the list of files from the S3 bucket
func (u *S3Uploader) ListS3Files(bucket string) ([]string, error) {
	var files []string
	paginator := s3.NewListObjectsV2Paginator(u.Client, &s3.ListObjectsV2Input{
		Bucket: &bucket,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			files = append(files, *obj.Key)
		}
	}
	return files, nil
}

// UploadFile uploads a file to S3
func (u *S3Uploader) UploadFile(localPath, s3Key string, bucket string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", localPath, err)
	}
	defer file.Close()

	_, err = u.Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &s3Key,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s to S3: %w", s3Key, err)
	}
	log.Printf("Uploaded %s to S3 as %s\n", localPath, s3Key)
	return nil
}

// EnsureArchiveDirectory ensures the archive directory exists
func EnsureArchiveDirectory(directory string) error {
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return os.MkdirAll(directory, 0755)
	}
	return nil
}

// GenerateArchiveFilePath generates a file path for the archive JSON
func GenerateArchiveFilePath(archiveDir, localDir string) string {
	// Replace path separators with "_" and ":" with "-"
	baseName := strings.ReplaceAll(localDir, string(os.PathSeparator), "_")
	baseName = strings.ReplaceAll(baseName, ":", "-")
	return filepath.Join(archiveDir, baseName+".json")
}

// Main logic
func main() {
	// Parse command-line arguments
	credFile := flag.String("cred", "", "Path to AWS credentials file (optional, defaults to ~/.aws/credentials)")
	bucketName := flag.String("bucket", "", "Name of the S3 bucket (required)")
	region := flag.String("region", "ap-northeast-1", "AWS region (default: ap-northeast-1)")
	localDirectory := flag.String("local", "", "Local directory to archive (required)")
	archiveFile := flag.String("archive", "", "Path to archive JSON file (optional)")
	flag.Parse()

	// Validate required arguments
	if *bucketName == "" || *localDirectory == "" {
		flag.Usage()
		log.Fatalf("Both -bucket and -local flags are required")
	}

	// Determine archive file path
	var archiveJSON string
	if *archiveFile != "" {
		archiveJSON = *archiveFile
	} else {
		archiveDir := "archives"
		if err := EnsureArchiveDirectory(archiveDir); err != nil {
			log.Fatalf("Failed to ensure archive directory: %v", err)
		}
		archiveJSON = GenerateArchiveFilePath(archiveDir, *localDirectory)
	}

	// Load AWS configuration
	var cfg aws.Config
	var err error

	if *credFile == "" {
		// Use default credentials file (~/.aws/credentials)
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(*region),
		)
		log.Println("Using default AWS credentials file (~/.aws/credentials)")
	} else {
		// Use specified credentials file
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithSharedCredentialsFiles([]string{*credFile}),
			config.WithRegion(*region),
		)
		log.Printf("Using specified AWS credentials file (%s)\n", *credFile)
	}

	if err != nil {
		log.Fatalf("Unable to load AWS config: %v", err)
	}
	client := s3.NewFromConfig(cfg)
	uploader := S3Uploader{Client: client}

	// Load archived files
	archived, err := LoadArchivedFiles(archiveJSON)
	if err != nil {
		log.Fatalf("Failed to load archived files: %v", err)
	}

	// Fetch file list from S3
	s3Files, err := uploader.ListS3Files(*bucketName)
	if err != nil {
		log.Fatalf("Failed to list files in S3 bucket: %v", err)
	}

	// Convert S3 files to a map for quick lookup
	s3FileMap := make(map[string]bool)
	for _, s3File := range s3Files {
		s3FileMap[s3File] = true
	}

	// Scan local files and compare with S3 and archived files
	err = filepath.Walk(*localDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Get relative path for S3 key
		relativePath := strings.TrimPrefix(path, *localDirectory)
		relativePath = strings.TrimPrefix(relativePath, string(os.PathSeparator))
		s3Key := strings.ReplaceAll(relativePath, string(os.PathSeparator), "/")

		// Check if file is already in S3 or archived
		if s3FileMap[s3Key] {
			log.Printf("Skipping %s: already exists in S3", s3Key)
			return nil
		}
		for _, archivedFile := range archived.Files {
			if archivedFile == s3Key {
				log.Printf("Skipping %s: already archived", s3Key)
				return nil
			}
		}

		// Upload the file
		if err := uploader.UploadFile(path, s3Key, *bucketName); err != nil {
			return err
		}

		// Add the file to the archived list
		archived.Files = append(archived.Files, s3Key)
		return nil
	})
	if err != nil {
		log.Fatalf("Error scanning local files: %v", err)
	}

	// Save updated archived files
	err = SaveArchivedFiles(archiveJSON, archived)
	if err != nil {
		log.Fatalf("Failed to save archived files: %v", err)
	}

	log.Println("Process completed successfully!")
}
