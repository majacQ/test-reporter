package metadata

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/buildpulse/test-reporter/internal/logger"
	"github.com/caarlos0/env/v6"
)

// A providerMetadata instance supplies the metadata for a build taking place on
// a specific CI provider (e.g., CircleCI, GitHub Actions, etc.).
type providerMetadata interface {
	Init(envs map[string]string, log logger.Logger) error
	Branch() string
	BuildURL() string
	CommitSHA() string
	Name() string
	RepoNameWithOwner() string
}

func newProviderMetadata(envs map[string]string, log logger.Logger) (providerMetadata, error) {
	var pm providerMetadata

	switch {
	case envs["BUILDKITE"] == "true":
		pm = &buildkiteMetadata{}
	case envs["CIRCLECI"] == "true":
		pm = &circleMetadata{}
	case envs["GITHUB_ACTIONS"] == "true":
		pm = &githubMetadata{}
	case envs["JENKINS_HOME"] != "":
		pm = &jenkinsMetadata{}
	case envs["SEMAPHORE"] == "true":
		pm = &semaphoreMetadata{}
	case envs["TRAVIS"] == "true":
		pm = &travisMetadata{}
	default:
		return nil, fmt.Errorf("unrecognized environment: system does not appear to be a supported CI provider (Buildkite, CircleCI, GitHub Actions, Jenkins, Semaphore, or Travis CI)")
	}
	log.Printf("Detected build environment: %s", pm.Name())

	if err := pm.Init(envs, log); err != nil {
		return nil, err
	}

	return pm, nil
}

var _ providerMetadata = (*buildkiteMetadata)(nil)

type buildkiteMetadata struct {
	// Fields derived from Buildkite-specific environment variables
	BuildkiteBranch                 string `env:"BUILDKITE_BRANCH" yaml:"-"`
	BuildkiteBuildID                string `env:"BUILDKITE_BUILD_ID" yaml:":buildkite_build_id"`
	BuildkiteBuildNumber            uint   `env:"BUILDKITE_BUILD_NUMBER" yaml:":buildkite_build_number"`
	BuildkiteBuildURL               string `env:"BUILDKITE_BUILD_URL" yaml:"-"`
	BuildkiteCommit                 string `env:"BUILDKITE_COMMIT" yaml:"-"`
	BuildkiteJobID                  string `env:"BUILDKITE_JOB_ID" yaml:":buildkite_job_id"`
	BuildkiteLabel                  string `env:"BUILDKITE_LABEL" yaml:":buildkite_label"`
	BuildkiteOrganizationSlug       string `env:"BUILDKITE_ORGANIZATION_SLUG" yaml:":buildkite_organization_slug"`
	BuildkitePipelineID             string `env:"BUILDKITE_PIPELINE_ID" yaml:":buildkite_pipeline_id"`
	BuildkitePipelineSlug           string `env:"BUILDKITE_PIPELINE_SLUG" yaml:":buildkite_pipeline_slug"`
	BuildkiteProjectSlug            string `env:"BUILDKITE_PROJECT_SLUG" yaml:":buildkite_project_slug"`
	BuildkitePullRequest            string `env:"BUILDKITE_PULL_REQUEST" yaml:"-"`
	BuildkitePullRequestBaseBranch  string `env:"BUILDKITE_PULL_REQUEST_BASE_BRANCH" yaml:":buildkite_pull_request_base_branch,omitempty"`
	BuildkitePullRequestNumber      uint   `yaml:":buildkite_pull_request_number,omitempty"`
	BuildkitePullRequestRepo        string `env:"BUILDKITE_PULL_REQUEST_REPO" yaml:":buildkite_pull_request_repo,omitempty"`
	BuildkiteRebuiltFromBuildID     string `env:"BUILDKITE_REBUILT_FROM_BUILD_ID" yaml:":buildkite_rebuilt_from_build_id,omitempty"`
	BuildkiteRebuiltFromBuildNumber uint   `env:"BUILDKITE_REBUILT_FROM_BUILD_NUMBER" yaml:":buildkite_rebuilt_from_build_number,omitempty"`
	BuildkiteRepoURL                string `env:"BUILDKITE_REPO" yaml:"-"`
	BuildkiteRetryCount             uint   `env:"BUILDKITE_RETRY_COUNT" yaml:":buildkite_retry_count"`
	BuildkiteTag                    string `env:"BUILDKITE_TAG" yaml:":buildkite_tag,omitempty"`

	nwo string
}

func (b *buildkiteMetadata) Init(envs map[string]string, log logger.Logger) error {
	if err := env.Parse(b, env.Options{Environment: envs}); err != nil {
		return err
	}

	log.Printf("Using $BUILDKITE_COMMIT environment variable as commit SHA: %s", b.BuildkiteCommit)

	nwo, err := nameWithOwnerFromGitURL(b.BuildkiteRepoURL)
	if err != nil {
		return err
	}
	b.nwo = nwo

	prNum, err := strconv.ParseUint(b.BuildkitePullRequest, 0, 0)
	if err == nil {
		b.BuildkitePullRequestNumber = uint(prNum)
	}

	return nil
}

func (b *buildkiteMetadata) Branch() string {
	return b.BuildkiteBranch
}

func (b *buildkiteMetadata) BuildURL() string {
	return b.BuildkiteBuildURL
}

func (b *buildkiteMetadata) CommitSHA() string {
	return b.BuildkiteCommit
}

func (b *buildkiteMetadata) Name() string {
	return "buildkite"
}

func (b *buildkiteMetadata) RepoNameWithOwner() string {
	return b.nwo
}

type circleMetadata struct {
	// Fields derived from Circle-specific environment variables
	CircleBranch              string `env:"CIRCLE_BRANCH" yaml:"-"`
	CircleBuildNumber         uint   `env:"CIRCLE_BUILD_NUM" yaml:":circle_build_num"`
	CircleBuildURL            string `env:"CIRCLE_BUILD_URL" yaml:"-"`
	CircleJob                 string `env:"CIRCLE_JOB" yaml:":circle_job"`
	CircleProjectReponame     string `env:"CIRCLE_PROJECT_REPONAME" yaml:"-"`
	CircleProjectUsername     string `env:"CIRCLE_PROJECT_USERNAME" yaml:"-"`
	CirclePullRequestNumber   uint   `env:"CIRCLE_PR_NUMBER" yaml:":circle_pr_number,omitempty"`
	CirclePullRequestReponame string `env:"CIRCLE_PR_REPONAME" yaml:":circle_pr_reponame,omitempty"`
	CirclePullRequestURL      string `env:"CIRCLE_PULL_REQUEST" yaml:":circle_pull_request,omitempty"`
	CirclePullRequestUsername string `env:"CIRCLE_PR_USERNAME" yaml:":circle_pr_username,omitempty"`
	CircleRepoURL             string `env:"CIRCLE_REPOSITORY_URL" yaml:":circle_repository_url"`
	CircleSHA1                string `env:"CIRCLE_SHA1" yaml:"-"`
	CircleTag                 string `env:"CIRCLE_TAG" yaml:":circle_tag,omitempty"`
	CircleUsername            string `env:"CIRCLE_USERNAME" yaml:":circle_username"`
	CircleWorkflowID          string `env:"CIRCLE_WORKFLOW_ID" yaml:":circle_workflow_id"`
}

func (c *circleMetadata) Init(envs map[string]string, log logger.Logger) error {
	if err := env.Parse(c, env.Options{Environment: envs}); err != nil {
		return err
	}

	log.Printf("Using $CIRCLE_SHA1 environment variable as commit SHA: %s", c.CircleSHA1)

	return nil
}

func (c *circleMetadata) Branch() string {
	return c.CircleBranch
}

func (c *circleMetadata) BuildURL() string {
	return c.CircleBuildURL
}

func (c *circleMetadata) CommitSHA() string {
	return c.CircleSHA1
}

func (c *circleMetadata) Name() string {
	return "circleci"
}

func (c *circleMetadata) RepoNameWithOwner() string {
	return fmt.Sprintf("%s/%s", c.CircleProjectUsername, c.CircleProjectReponame)
}

var _ providerMetadata = (*githubMetadata)(nil)

type githubMetadata struct {
	// Fields derived from GitHub-specific environment variables
	GithubActor     string `env:"GITHUB_ACTOR" yaml:":github_actor"`
	GithubBaseRef   string `env:"GITHUB_BASE_REF" yaml:":github_base_ref"`
	GithubEventName string `env:"GITHUB_EVENT_NAME" yaml:":github_event_name"`
	GithubHeadRef   string `env:"GITHUB_HEAD_REF" yaml:":github_head_ref"`
	GithubRef       string `env:"GITHUB_REF" yaml:":github_ref"`
	GithubRepoNWO   string `env:"GITHUB_REPOSITORY" yaml:"-"`
	GithubRepoURL   string `yaml:":github_repo_url"`
	GithubRunID     uint   `env:"GITHUB_RUN_ID" yaml:":github_run_id"`
	GithubRunNumber uint   `env:"GITHUB_RUN_NUMBER" yaml:":github_run_number"`
	GithubServerURL string `env:"GITHUB_SERVER_URL" yaml:"-"`
	GithubSHA       string `env:"GITHUB_SHA" yaml:"-"`
	GithubWorkflow  string `env:"GITHUB_WORKFLOW" yaml:":github_workflow"`

	branch   string
	buildURL string
}

func (g *githubMetadata) Init(envs map[string]string, log logger.Logger) error {
	if err := env.Parse(g, env.Options{Environment: envs}); err != nil {
		return err
	}

	log.Printf("Using $GITHUB_SHA environment variable as commit SHA: %s", g.GithubSHA)

	g.GithubRepoURL = fmt.Sprintf("%s/%s", g.GithubServerURL, g.GithubRepoNWO)

	g.buildURL = fmt.Sprintf("%s/actions/runs/%d", g.GithubRepoURL, g.GithubRunID)

	isBranch, err := regexp.MatchString("^refs/heads/", g.GithubRef)
	if err != nil {
		return err
	}
	if isBranch {
		g.branch = strings.TrimPrefix(g.GithubRef, "refs/heads/")
	}

	return nil
}

func (g *githubMetadata) Branch() string {
	return g.branch
}

func (g *githubMetadata) BuildURL() string {
	return g.buildURL
}

func (g *githubMetadata) CommitSHA() string {
	return g.GithubSHA
}

func (g *githubMetadata) Name() string {
	return "github-actions"
}

func (g *githubMetadata) RepoNameWithOwner() string {
	return g.GithubRepoNWO
}

var _ providerMetadata = (*jenkinsMetadata)(nil)

type jenkinsMetadata struct {
	// Fields derived from Jenkins-specific environment variables
	GitBranch             string `env:"GIT_BRANCH" yaml:"-"`
	GitCommit             string `env:"GIT_COMMIT" yaml:"-"`
	GitURL                string `env:"GIT_URL" yaml:"-"`
	JenkinsExecutorNumber uint   `env:"EXECUTOR_NUMBER" yaml:":jenkins_executor_number"`
	JenkinsJobName        string `env:"JOB_NAME" yaml:":jenkins_job_name"`
	JenkinsJobURL         string `env:"JOB_URL" yaml:":jenkins_job_url"`
	JenkinsNodeName       string `env:"NODE_NAME" yaml:":jenkins_node_name"`
	JenkinsWorkspace      string `env:"WORKSPACE" yaml:":jenkins_workspace"`

	buildURL string
	nwo      string
}

func (j *jenkinsMetadata) Init(envs map[string]string, log logger.Logger) error {
	if err := env.Parse(j, env.Options{Environment: envs}); err != nil {
		return err
	}

	log.Printf("Using $GIT_COMMIT environment variable as commit SHA: %s", j.GitCommit)

	url, ok := envs["BUILD_URL"]
	if !ok || url == "" {
		return fmt.Errorf("missing required environment variable: BUILD_URL")
	}
	j.buildURL = url

	nwo, err := nameWithOwnerFromGitURL(j.GitURL)
	if err != nil {
		return err
	}
	j.nwo = nwo

	return nil
}

func (j *jenkinsMetadata) Branch() string {
	return j.GitBranch
}

func (j *jenkinsMetadata) BuildURL() string {
	return j.buildURL
}

func (j *jenkinsMetadata) CommitSHA() string {
	return j.GitCommit
}

func (j *jenkinsMetadata) Name() string {
	return "jenkins"
}

func (j *jenkinsMetadata) RepoNameWithOwner() string {
	return j.nwo
}

var _ providerMetadata = (*semaphoreMetadata)(nil)

type semaphoreMetadata struct {
	// Fields derived from Semaphore-specific environment variables
	SemaphoreAgentMachineEnvironmentType string `env:"SEMAPHORE_AGENT_MACHINE_ENVIRONMENT_TYPE" yaml:":semaphore_agent_machine_environment_type"`
	SemaphoreAgentMachineOsImage         string `env:"SEMAPHORE_AGENT_MACHINE_OS_IMAGE" yaml:":semaphore_agent_machine_os_image"`
	SemaphoreAgentMachineType            string `env:"SEMAPHORE_AGENT_MACHINE_TYPE" yaml:":semaphore_agent_machine_type"`
	SemaphoreGitBranch                   string `env:"SEMAPHORE_GIT_BRANCH" yaml:"-"`
	SemaphoreGitCommitRange              string `env:"SEMAPHORE_GIT_COMMIT_RANGE" yaml:":semaphore_git_commit_range"`
	SemaphoreGitDir                      string `env:"SEMAPHORE_GIT_DIR" yaml:":semaphore_git_dir"`
	SemaphoreGitRef                      string `env:"SEMAPHORE_GIT_REF" yaml:":semaphore_git_ref"`
	SemaphoreGitRefType                  string `env:"SEMAPHORE_GIT_REF_TYPE" yaml:":semaphore_git_ref_type"`
	SemaphoreGitRepoSlug                 string `env:"SEMAPHORE_GIT_REPO_SLUG" yaml:"-"`
	SemaphoreGitSHA                      string `env:"SEMAPHORE_GIT_SHA" yaml:"-"`
	SemaphoreGitURL                      string `env:"SEMAPHORE_GIT_URL" yaml:":semaphore_git_url"`
	SemaphoreJobID                       string `env:"SEMAPHORE_JOB_ID" yaml:":semaphore_job_id"`
	SemaphoreJobName                     string `env:"SEMAPHORE_JOB_NAME" yaml:":semaphore_job_name"`
	SemaphoreJobResult                   string `env:"SEMAPHORE_JOB_RESULT" yaml:":semaphore_job_result"`
	SemaphoreOrganizationURL             string `env:"SEMAPHORE_ORGANIZATION_URL" yaml:":semaphore_organization_url"`
	SemaphoreProjectID                   string `env:"SEMAPHORE_PROJECT_ID" yaml:":semaphore_project_id"`
	SemaphoreProjectName                 string `env:"SEMAPHORE_PROJECT_NAME" yaml:":semaphore_project_name"`
	SemaphoreWorkflowID                  string `env:"SEMAPHORE_WORKFLOW_ID" yaml:":semaphore_workflow_id"`
	SemaphoreWorkflowNumber              uint   `env:"SEMAPHORE_WORKFLOW_NUMBER" yaml:":semaphore_workflow_number"`
}

func (s *semaphoreMetadata) Init(envs map[string]string, log logger.Logger) error {
	if err := env.Parse(s, env.Options{Environment: envs}); err != nil {
		return err
	}

	log.Printf("Using $SEMAPHORE_GIT_SHA environment variable as commit SHA: %s", s.SemaphoreGitSHA)

	return nil
}

func (s *semaphoreMetadata) Branch() string {
	return s.SemaphoreGitBranch
}

func (s *semaphoreMetadata) BuildURL() string {
	return fmt.Sprintf("%s/workflows/%s", s.SemaphoreOrganizationURL, s.SemaphoreWorkflowID)
}

func (s *semaphoreMetadata) CommitSHA() string {
	return s.SemaphoreGitSHA
}

func (s *semaphoreMetadata) Name() string {
	return "semaphore"
}

func (s *semaphoreMetadata) RepoNameWithOwner() string {
	return s.SemaphoreGitRepoSlug
}

var _ providerMetadata = (*travisMetadata)(nil)

type travisMetadata struct {
	// Fields derived from Travis-specific environment variables
	TravisBranch            string `env:"TRAVIS_BRANCH" yaml:"-"`
	TravisBuildDir          string `env:"TRAVIS_BUILD_DIR" yaml:":travis_build_dir"`
	TravisBuildID           uint   `env:"TRAVIS_BUILD_ID" yaml:":travis_build_id"`
	TravisBuildNumber       uint   `env:"TRAVIS_BUILD_NUMBER" yaml:":travis_build_number"`
	TravisBuildWebURL       string `env:"TRAVIS_BUILD_WEB_URL" yaml:":travis_build_web_url"`
	TravisCommit            string `env:"TRAVIS_COMMIT" yaml:"-"`
	TravisCommitRange       string `env:"TRAVIS_COMMIT_RANGE" yaml:":travis_commit_range"`
	TravisCPUArch           string `env:"TRAVIS_CPU_ARCH" yaml:":travis_cpu_arch"`
	TravisDist              string `env:"TRAVIS_DIST" yaml:":travis_dist"`
	TravisEventType         string `env:"TRAVIS_EVENT_TYPE" yaml:":travis_event_type"`
	TravisJobID             uint   `env:"TRAVIS_JOB_ID" yaml:":travis_job_id"`
	TravisJobName           string `env:"TRAVIS_JOB_NAME" yaml:":travis_job_name"`
	TravisJobNumber         string `env:"TRAVIS_JOB_NUMBER" yaml:":travis_job_number"`
	TravisJobWebURL         string `env:"TRAVIS_JOB_WEB_URL" yaml:"-"`
	TravisOsName            string `env:"TRAVIS_OS_NAME" yaml:":travis_os_name"`
	TravisPullRequest       string `env:"TRAVIS_PULL_REQUEST" yaml:"-"`
	TravisPullRequestBranch string `env:"TRAVIS_PULL_REQUEST_BRANCH" yaml:":travis_pull_request_branch,omitempty"`
	TravisPullRequestNumber uint   `yaml:":travis_pull_request_number,omitempty"`
	TravisPullRequestSha    string `env:"TRAVIS_PULL_REQUEST_SHA" yaml:":travis_pull_request_sha,omitempty"`
	TravisPullRequestSlug   string `env:"TRAVIS_PULL_REQUEST_SLUG" yaml:":travis_pull_request_slug,omitempty"`
	TravisRepoSlug          string `env:"TRAVIS_REPO_SLUG" yaml:"-"`
	TravisSudo              bool   `env:"TRAVIS_SUDO" yaml:":travis_sudo"`
	TravisTag               string `env:"TRAVIS_TAG" yaml:":travis_tag"`
	TravisTestResult        uint   `env:"TRAVIS_TEST_RESULT" yaml:":travis_test_result"`
}

func (t *travisMetadata) Init(envs map[string]string, log logger.Logger) error {
	if err := env.Parse(t, env.Options{Environment: envs}); err != nil {
		return err
	}

	log.Printf("Using $TRAVIS_COMMIT environment variable as commit SHA: %s", t.TravisCommit)

	prNum, err := strconv.ParseUint(t.TravisPullRequest, 0, 0)
	if err == nil {
		t.TravisPullRequestNumber = uint(prNum)
	}

	return nil
}

func (t *travisMetadata) Branch() string {
	return t.TravisBranch
}

func (t *travisMetadata) BuildURL() string {
	return t.TravisJobWebURL
}

func (t *travisMetadata) CommitSHA() string {
	return t.TravisCommit
}

func (t *travisMetadata) Name() string {
	return "travis-ci"
}

func (t *travisMetadata) RepoNameWithOwner() string {
	return t.TravisRepoSlug
}

func nameWithOwnerFromGitURL(url string) (string, error) {
	re := regexp.MustCompile(`github.com[:/](.*)`)

	matches := re.FindStringSubmatch(url)
	if len(matches) != 2 {
		return "", fmt.Errorf("unable to extract repository name-with-owner from URL: %s", url)
	}

	return strings.TrimSuffix(matches[1], ".git"), nil
}
