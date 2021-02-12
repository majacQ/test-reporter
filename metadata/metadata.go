package metadata

import (
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// A Metadata instance provides metadata about a set of test results. It
// identifies the CI provider, the commit SHA, the time at which the tests were
// executed, etc.
type Metadata struct {
	AuthoredAt        time.Time `yaml:":authored_at,omitempty"`
	AuthorEmail       string    `yaml:":author_email,omitempty"`
	AuthorName        string    `yaml:":author_name,omitempty"`
	Branch            string    `yaml:":branch"`
	BuildURL          string    `yaml:":build_url"`
	Check             string    `yaml:":check" env:"BUILDPULSE_CHECK_NAME"` // TODO: Should this env be here or in the providers?
	CIProvider        string    `yaml:":ci_provider"`
	CommitMessage     string    `yaml:":commit_message,omitempty"`
	CommitSHA         string    `yaml:":commit"`
	CommittedAt       time.Time `yaml:":committed_at,omitempty"`
	CommitterEmail    string    `yaml:":committer_email,omitempty"`
	CommitterName     string    `yaml:":committer_name,omitempty"`
	RepoNameWithOwner string    `yaml:":repo_name_with_owner"`
	ReporterOS        string    `yaml:":reporter_os"`
	ReporterVersion   string    `yaml:":reporter_version"`
	Timestamp         time.Time `yaml:":timestamp"`
	TreeSHA           string    `yaml:":tree,omitempty"`

	providerData providerMetadata
}

// NewMetadata creates a new Metadata instance from the given args.
func NewMetadata(version *Version, envs map[string]string, resolver CommitResolver, now func() time.Time, log Logger) (*Metadata, error) {
	m := &Metadata{}

	if err := m.initProviderData(envs, log); err != nil {
		return nil, err
	}

	if err := m.initCommitData(resolver, m.providerData.CommitSHA(), log); err != nil {
		return nil, err
	}

	m.initTimestamp(now)
	m.initVersionData(version)

	return m, nil
}

func (m *Metadata) initProviderData(envs map[string]string, log Logger) error {
	pm, err := newProviderMetadata(envs, log)
	if err != nil {
		return err
	}

	m.providerData = pm
	m.Branch = pm.Branch()
	m.BuildURL = pm.BuildURL()
	m.Check = pm.Check()
	m.CIProvider = pm.Name()
	m.RepoNameWithOwner = pm.RepoNameWithOwner()

	return nil
}

func (m *Metadata) initCommitData(cr CommitResolver, sha string, log Logger) error {
	// Git metadata functionality is experimental. While it's experimental, detect a nil CommitResolver and allow the commit metadata fields to be uploaded with empty values.
	if cr == nil {
		log.Printf("[experimental] no commit resolver available; falling back to commit data from environment\n")

		m.CommitSHA = sha
		return nil
	}

	// Git metadata functionality is experimental. While it's experimental, don't let this error prevent the test-reporter from continuing normal operation. Allow the commit metadata fields to be uploaded with empty values.
	c, err := cr.Lookup(sha)
	if err != nil {
		log.Printf("[experimental] git-based commit lookup unsuccessful; falling back to commit data from environment: %v\n", err)

		m.CommitSHA = sha
		return nil
	}

	m.AuthoredAt = c.AuthoredAt
	m.AuthorEmail = c.AuthorEmail
	m.AuthorName = c.AuthorName
	m.CommitMessage = strings.TrimSpace(c.Message)
	m.CommitSHA = c.SHA
	m.CommittedAt = c.CommittedAt
	m.CommitterEmail = c.CommitterEmail
	m.CommitterName = c.CommitterName
	m.TreeSHA = c.TreeSHA

	return nil
}

func (m *Metadata) initTimestamp(now func() time.Time) {
	m.Timestamp = now()
}

func (m *Metadata) initVersionData(version *Version) {
	m.ReporterOS = version.GoOS
	m.ReporterVersion = version.Number
}

// MarshalYAML TODO Add docs
func (m *Metadata) MarshalYAML() (out []byte, err error) {
	topLevel, err := marshalYAML(m)
	if err != nil {
		return nil, err
	}

	providerLevel, err := marshalYAML(m.providerData)
	if err != nil {
		return nil, err
	}

	return append(topLevel, providerLevel...), nil
}

func marshalYAML(m interface{}) (out []byte, err error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}
