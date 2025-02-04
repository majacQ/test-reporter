package submit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/buildpulse/test-reporter/internal/logger"
	"github.com/buildpulse/test-reporter/internal/metadata"
	"github.com/google/uuid"
)

type credentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

// A CommitResolverFactory provides methods for creating a
// metadata.CommitResolver.
type CommitResolverFactory interface {
	NewFromRepository(path string) (metadata.CommitResolver, error)
	NewFromStaticValue(commit *metadata.Commit) metadata.CommitResolver
}

type defaultCommitResolverFactory struct {
	logger logger.Logger
}

// NewCommitResolverFactory returns a CommitResolverFactory that creates
// CommitResolvers with the default production implementations.
func NewCommitResolverFactory(logger logger.Logger) CommitResolverFactory {
	return &defaultCommitResolverFactory{logger: logger}
}

// NewFromRepository returns a CommitResolver for looking up commits in the
// repository located at path.
func (d *defaultCommitResolverFactory) NewFromRepository(path string) (metadata.CommitResolver, error) {
	return metadata.NewRepositoryCommitResolver(path, d.logger)
}

// NewFromStaticValue returns a CommitResolver whose Lookup method always
// produces a Commit with values matching the fields in commit.
func (d *defaultCommitResolverFactory) NewFromStaticValue(commit *metadata.Commit) metadata.CommitResolver {
	return metadata.NewStaticCommitResolver(commit, d.logger)
}

// Submit represents the task of preparing and sending a set of test results to
// BuildPulse.
type Submit struct {
	client  *http.Client
	fs      *flag.FlagSet
	idgen   func() uuid.UUID
	logger  logger.Logger
	version *metadata.Version

	envs           map[string]string
	path           string
	accountID      uint64
	repositoryID   uint64
	repositoryPath string
	tree           string
	credentials    credentials
	commitResolver metadata.CommitResolver
}

// NewSubmit creates a new Submit instance.
func NewSubmit(version *metadata.Version, log logger.Logger) *Submit {
	s := &Submit{
		client:  http.DefaultClient,
		fs:      flag.NewFlagSet("submit", flag.ContinueOnError),
		idgen:   uuid.New,
		logger:  log,
		version: version,
	}

	s.fs.Uint64Var(&s.accountID, "account-id", 0, "BuildPulse account ID (required)")
	s.fs.Uint64Var(&s.repositoryID, "repository-id", 0, "BuildPulse repository ID (required)")
	s.fs.StringVar(&s.repositoryPath, "repository-dir", ".", "Path to local clone of repository")
	s.fs.StringVar(&s.tree, "tree", "", "SHA-1 hash of git tree")
	s.fs.SetOutput(ioutil.Discard) // Disable automatic writing to STDERR

	s.logger.Printf("Current version: %s", s.version.String())
	s.logger.Println("Initiating `submit`")

	return s
}

// Init populates s from args and envs. It returns an error if the required args
// or environment variables are missing or malformed.
func (s *Submit) Init(args []string, envs map[string]string, commitResolverFactory CommitResolverFactory) error {
	s.logger.Printf("Received args: %s", strings.Join(args, " "))

	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	s.logger.Printf("Using working directory: %v", dir)

	s.path = args[0]
	isFlag, err := regexp.MatchString("^-", s.path)
	if err != nil {
		return err
	}
	if isFlag {
		return fmt.Errorf("missing TEST_RESULTS_DIR")
	}
	info, err := os.Stat(s.path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", s.path)
	}

	if err := s.fs.Parse(args[1:]); err != nil {
		return err
	}

	flagset := make(map[string]bool)
	s.fs.Visit(func(f *flag.Flag) { flagset[f.Name] = true })

	if s.accountID == 0 {
		return fmt.Errorf("missing required flag: -account-id")
	}

	if s.repositoryID == 0 {
		return fmt.Errorf("missing required flag: -repository-id")
	}

	id, ok := envs["BUILDPULSE_ACCESS_KEY_ID"]
	if !ok || id == "" {
		return fmt.Errorf("missing required environment variable: BUILDPULSE_ACCESS_KEY_ID")
	}
	s.credentials.AccessKeyID = id

	key, ok := envs["BUILDPULSE_SECRET_ACCESS_KEY"]
	if !ok || key == "" {
		return fmt.Errorf("missing required environment variable: BUILDPULSE_SECRET_ACCESS_KEY")
	}
	s.credentials.SecretAccessKey = key

	if flagset["repository-dir"] && flagset["tree"] {
		return fmt.Errorf("invalid use of flag -repository-dir with flag -tree: use one or the other, but not both")
	}

	re := regexp.MustCompile(`^[0-9a-f]{40}$`)
	if flagset["tree"] && !re.MatchString(s.tree) {
		return fmt.Errorf("invalid value \"%s\" for flag -tree: should be a 40-character SHA-1 hash", s.tree)
	}

	info, err = os.Stat(s.repositoryPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("invalid value for flag -repository-dir: %s is not a directory", s.repositoryPath)
	}

	s.envs = envs

	if flagset["tree"] {
		s.logger.Printf("Using value of -tree flag as the tree SHA for this submission: %s", s.tree)
		s.commitResolver = commitResolverFactory.NewFromStaticValue(&metadata.Commit{TreeSHA: s.tree})
		return nil
	}

	if !flagset["repository-dir"] {
		s.logger.Printf("Using default value for -repository-dir flag: %s", s.repositoryPath)
	}

	s.logger.Printf("Looking for git repository at %s", s.repositoryPath)
	s.commitResolver, err = commitResolverFactory.NewFromRepository(s.repositoryPath)
	if err != nil {
		return fmt.Errorf("invalid value for flag -repository-dir: %v", err)
	}
	s.logger.Printf("Found git repository at %s", s.repositoryPath)

	return nil
}

// Run packages up the test results and sends them to BuildPulse. It returns the
// key that uniquely identifies the uploaded object.
func (s *Submit) Run() (string, error) {
	meta, err := metadata.NewMetadata(s.version, s.envs, s.commitResolver, time.Now, s.logger)
	if err != nil {
		return "", err
	}

	yamlpath := filepath.Join(s.path, "buildpulse.yml")
	s.logger.Printf("Writing metadata to %s", yamlpath)
	yaml, err := meta.MarshalYAML()
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(yamlpath, yaml, 0644)
	if err != nil {
		return "", err
	}

	logpath := filepath.Join(s.path, "buildpulse.log")
	s.logger.Printf("Flushing log to %s", logpath)
	err = ioutil.WriteFile(logpath, []byte(s.logger.Text()), 0644)
	if err != nil {
		return "", err
	}

	s.logger.Printf("Preparing gzipped archive of test results and metadata at %s", s.path)
	path, err := toTarGz(s.path)
	if err != nil {
		return "", err
	}
	s.logger.Printf("Gzipped archive written to %s", path)

	s.logger.Printf("Sending %s to BuildPulse", path)
	key, err := s.upload(path)
	if err != nil {
		return "", err
	}
	s.logger.Printf("Delivered test results to BuildPulse (%s)", key)

	return key, nil
}

// upload transmits the file at the given path to S3
func (s *Submit) upload(path string) (string, error) {
	bucket := fmt.Sprintf("%d.buildpulse-uploads", s.accountID)
	key := fmt.Sprintf("%d/buildpulse-%s.gz", s.repositoryID, s.idgen())

	err := putS3Object(s.client, s.credentials.AccessKeyID, s.credentials.SecretAccessKey, bucket, key, path)
	if err != nil {
		return "", err
	}

	return key, nil
}

// toTarGz creates a gzipped tarball containing the contents of the named
// directory (dir) and returns the path of the resulting file.
func toTarGz(dir string) (dest string, err error) {
	tarPath, err := toTar(dir)
	if err != nil {
		return "", err
	}

	return toGz(tarPath)
}

// toTar creates a tarball containing the submittable contents of the named
// directory (dir) and returns the path of the resulting file.
func toTar(dir string) (dest string, err error) {
	tarfile, err := ioutil.TempFile("", "buildpulse-*.tar")
	if err != nil {
		return "", err
	}
	defer tarfile.Close()

	writer := tar.NewWriter(tarfile)
	defer writer.Close()

	isIncludable := func(info os.FileInfo) bool {
		return info.IsDir() ||
			filepath.Base(info.Name()) == "buildpulse.log" ||
			filepath.Base(info.Name()) == "buildpulse.yml" ||
			bytes.EqualFold([]byte(filepath.Ext(info.Name())), []byte(".xml"))
	}

	return tarfile.Name(), filepath.Walk(dir,
		func(srcpath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !isIncludable(info) {
				return nil
			}

			destpath, err := filepath.Rel(dir, srcpath)
			if err != nil {
				return err
			}

			header, err := tar.FileInfoHeader(info, destpath)
			if err != nil {
				return err
			}

			header.Name = destpath
			if err := writer.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(srcpath)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			return err
		})
}

// toGz gzips the named file (src) and returns the path of the resulting file.
func toGz(src string) (dest string, err error) {
	reader, err := os.Open(src)
	if err != nil {
		return "", err
	}

	zipfile, err := ioutil.TempFile("", "buildpulse-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer zipfile.Close()

	writer := gzip.NewWriter(zipfile)
	defer writer.Close()

	_, err = io.Copy(writer, reader)
	return zipfile.Name(), err
}

// putS3Object puts the named file (src) as an object in the named bucket with the named key.
func putS3Object(client *http.Client, id string, secret string, bucket string, objectKey string, src string) error {
	provider := &awscreds.StaticProvider{
		Value: awscreds.Value{
			AccessKeyID:     id,
			SecretAccessKey: secret,
		},
	}

	sess, err := session.NewSession(
		aws.NewConfig().
			WithCredentials(awscreds.NewCredentials(provider)).
			WithRegion("us-east-2").
			WithHTTPClient(client),
	)
	if err != nil {
		return err
	}

	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
		Body:   file,
	})
	if err != nil {
		return err
	}

	return nil
}
