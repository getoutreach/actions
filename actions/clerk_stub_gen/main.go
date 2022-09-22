package main

import (
	"io/ioutil"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"path/filepath"
	"os/exec"
	// "net/http"
	// "sort"
	"encoding/json"
    "math/rand"


	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/aws/session"
	// "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/getoutreach/actions/internal/gh"
	"github.com/google/go-github/v47/github"
	"github.com/getoutreach/gobox/pkg/cli/updater/release"
	"github.com/getoutreach/gobox/pkg/cfg"
	actions "github.com/sethvargo/go-githubactions"
	commonConfig "github.com/getoutreach/clerkgen/cmd/config"
)

const (
	format = "2006-Jan-02"
	tagName = "v1.27.4"
)

var (
	// downloadableSchemaProviders: map of schema providers
	downloadableSchemaProviders = map[string]struct{}{"outreach": {}}
)

// interface for s3 client
type S3Client interface {
	ListObjectsV2(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
}

// struct for schema config json output
type Info struct {
	SchemaName string `json:"schema_name"`
	ID         int    `json:"id"`
	Version    int    `json:"version"`
	Provider   string
	Namespace  string
	EventName  string
	Path       string
}

// struct of info needed for the creation of pull requests
type PullRequestArg struct {
	sourceOwner string
	sourceRepo string
	sourceFiles string
	authorName string
	authorEmail string
	commitBranch string
	baseBranch string
	commitMessage string
	prRepo string
	prRepoOwner string
	prBranch string
	prSubject string
	prDescription string
}

func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	client, client_err := gh.NewClient(ctx, false)
	if client_err != nil {
		actions.Errorf("unable to create github client with error %v", client_err)
		return
	}

	ghContext, err := actions.Context()
	if err != nil {
		actions.Errorf("unable to get action context with error %v", err)
		return
	}

	if err := RunAction(ctx, client, ghContext); err != nil {
		actions.Errorf(err.Error())
		return
	}
	exitCode = 0
}


// RunAction is where the actual implementation of the GitHub action goes and is called
// by func main.
func RunAction(ctx context.Context, client *github.Client, actionCtx *actions.GitHubContext) error {

	// githubBranch := strings.TrimSpace(os.Getenv("GITHUB_BRANCH"))
	// slackChannel := strings.TrimSpace(os.Getenv("SLACK_CHANNEL"))
	// awsSecretAccessKey := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	// awsAccessKeyId := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		// Credentials: credentials.NewStaticCredentials(awsAccessKeyId, awsSecretAccessKey, ""),
	})
	if err != nil {
		fmt.Printf("failed to create s3 client with reason: %v\n", err)
		return err
	}
	s3Client := s3.New(sess)
	now := time.Now()

	// Get latest timestamp of schema configs from S3
	maxTime, _, latestObj, oldLatestObj, err := 
		getAllSchemaChangesInLastHour(commonConfig.S3Bucket, commonConfig.ProtobufS3Prefix, s3Client, ctx, now)
	if err != nil {
		fmt.Printf("error while dowloading schema config from S3: %v\n", err)
		return err
	}

	if now.After(maxTime) && now.Add(-1 * time.Hour).Before(maxTime) {

		// Get the names of schemas changed during the past hour
		schemaNames, err := getChangedSchemaNames(commonConfig.S3Bucket, commonConfig.ProtobufS3Prefix, 
			s3Client, oldLatestObj, latestObj)
		if err != nil {
			fmt.Printf("getChangedSchemaNames returned error: %v", err)
			return err
		}

		if len(schemaNames) > 0 {
			// download the clerkgen cli with the given tag
			err = downloadReleaseAssetViaGobox(ctx)
			if err != nil {
				fmt.Printf("downloadReleaseAssetViaGobox returned error: %v", err)
				return err
			}

			// run untar clerkgen
			runCommand("tar", "-xzvf", "clerkgen_" + tagName[1:] + "_linux_amd64.tar.gz") 
		
			// construct parameters passed into clerkgenproto 
			args := []string{}
			args = append(args, "--out-dir")
			args = append(args, "pkg/schema/")
			args = append(args, "-l")
			args = append(args, "go")
			for _, name := range schemaNames {
				args = append(args, "--sn")
				args = append(args, name)
			}

			// run clerkgenproto
			runCommand("./clerkgenproto", args...)

			// intialize parameters used to create pull requests
			pullRequestArg, err := initialize()
			if err != nil {
				fmt.Printf("initialize with %v error\n", err)
				return err
			}

			// create pull request on a branch
			// example is here https://github.com/google/go-github/blob/master/example/commitpr/main.go
			if err := createPR(pullRequestArg, client, ctx); err != nil {
				fmt.Printf("Error while creating the pull request: %v", err)
				return err
			}
		}
	}

	return nil
}

// Get the names of schemas changed during the past hour
func getChangedSchemaNames(s3Bucket string, s3Prefix string, s3Client S3Client, 
	oldLatestObj *s3.Object, latestObj *s3.Object) ([]string, error){
	// get schemas updated
	err := downloadSchemaConfig(s3Bucket, s3Prefix, 
		"schema_before.json", s3Client, *oldLatestObj.Key)
	if err != nil {
		fmt.Printf("downloadSchemaConfig with key: %s returned error: %v", *oldLatestObj.Key, err)
		return nil, err
	}

	err = downloadSchemaConfig(s3Bucket, s3Prefix, 
		"schema_after.json", s3Client, *latestObj.Key)
	if err != nil {
		fmt.Printf("downloadSchemaConfig with key: %s returned error: %v", *latestObj.Key, err)
		return nil, err
	}

	mapBefore, err := readSchemaConfig("schema_before.json")
	if err != nil {
		fmt.Printf("readSchemaConfig of file: schema_before.json returned error: %v", err)
		return nil, err
	}

	mapAfter, err := readSchemaConfig("schema_after.json")
	if err != nil {
		fmt.Printf("readSchemaConfig of file: schema_after.json returned error: %v", err)
		return nil, err
	}

	schemaNames := []string{}
	for key, value := range mapBefore {
		if _, ok:= mapAfter[key]; ok {         
			if value != mapAfter[key] {
				schemaNames = append(schemaNames, key)
			}
		}
	}

	return schemaNames, nil
}

// Set parameters used for creating PRs
func initialize() (*PullRequestArg, error) {
	sourceFileList, err := getSourceFiles("./out")
	if err != nil {
		fmt.Printf("getSourceFiles with %v error\n", err)
		return nil, err
	}
	sourceFilesRaw := strings.Join(sourceFileList, ",")

	return &PullRequestArg{
		sourceOwner: "getoutreach",
		sourceRepo: "clerkcommons",
		sourceFiles: sourceFilesRaw,
		authorName: "Outreach CI", 
		authorEmail: "outreach-ci@users.noreply.github.com",
		commitBranch: "feature-" + randomString(8),
		baseBranch: "main",
		commitMessage: "feat: generate new clerk stubs",
		prRepo: "",
		prRepoOwner: "",
		prBranch: "main",
		prSubject: "feat: generate new clerk stubs",
		prDescription: "Automatically generate new clerk stubs per hour",
	}, nil
}

func randomString(length int) string {
    rand.Seed(time.Now().UnixNano())
    b := make([]byte, length)
    rand.Read(b)
    return fmt.Sprintf("%x", b)[:length]
}

// Download schema config file from s3 bucket to local drive
func downloadSchemaConfig(bucket, prefix, schemaConfigPath string, s3Client S3Client, key string) error {
	file, err := os.Create(schemaConfigPath)
	if err != nil {
		return fmt.Errorf("error cerating file %s with error %v", schemaConfigPath, err)
	}
	defer file.Close()

	sess, _ := session.NewSession(&aws.Config{Region: aws.String(os.Getenv("AWS_REGION"))})
	downloader := s3manager.NewDownloader(sess)
	_, err = downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})

	if err != nil {
		return fmt.Errorf("error downloading object %s with error %v", key, err)
	}

	return nil
}

// ReadSchemaConfig reads schema config file and returns a map of schema_name->version#
func readSchemaConfig(schemaConfigPath string) (map[string]int, error) {
	data, err := os.ReadFile(schemaConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error reading schema config %s with error %v", schemaConfigPath, err)
	}

	var infos []*Info
	err = json.Unmarshal(data, &infos)
	if err != nil {
		return nil, fmt.Errorf("error decoding schema config %s with error %v", schemaConfigPath, err)
	}

	result := make(map[string]int)
	for _, i := range infos {
		if !shouldDownloadSchema(i.SchemaName) {
			continue
		}

		result[i.SchemaName] = i.Version
	}

	return result, nil
}

func shouldDownloadSchema(schemaName string) bool {
	for provider := range downloadableSchemaProviders {
		if strings.HasPrefix(schemaName, provider) {
			return true
		}
	}
	return false
}

// There are the configs of the schemas that are registered in schema registry and uploaded to S3
// It goes over the entire folder in s3 and returns the latest timestamp of the files in that folder
// Get all the schemas that have been changed within the last hour
func getAllSchemaChangesInLastHour(bucket string, prefix string, s3Client S3Client,
	ctx context.Context, now time.Time) (time.Time, time.Time, *s3.Object, *s3.Object, error) {

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	}

	maxTime, _  := time.Parse(format, "2012-May-19") // Just picking a really old date to start comparison with.
	oldMaxTime, _  := time.Parse(format, "2012-May-19")
	
	var latestObj *s3.Object
	var oldLatestObj *s3.Object
	for {
		out, err := s3Client.ListObjectsV2(input)
		if err != nil {
			return maxTime, oldMaxTime, latestObj, oldLatestObj, 
				fmt.Errorf("error listing objects for bucket '%s' and prefix '%s", bucket, prefix)
		}
		for _, obj := range out.Contents {
			if obj.LastModified.After(maxTime) {
				maxTime = *obj.LastModified
				latestObj = obj
			}

			if obj.LastModified.Before(now.Add(-1 * time.Hour)) && obj.LastModified.After(oldMaxTime) {
				oldMaxTime = *obj.LastModified
				oldLatestObj = obj
			}
		}
		if out.NextContinuationToken == nil {
			break
		}
		input.ContinuationToken = out.NextContinuationToken
	}

	if latestObj == nil {
		return maxTime, oldMaxTime, latestObj, oldLatestObj,
			fmt.Errorf("no objects found in bucket '%s' and prefix '%s", bucket, prefix)
	}

	return maxTime, oldMaxTime, latestObj, oldLatestObj, nil
}


// getRef returns the commit branch reference object if it exists or creates it
// from the base branch before returning it.
func getRef(pullRequestArg *PullRequestArg, client *github.Client, ctx context.Context) (ref *github.Reference, err error) {
	if ref, _, err = client.Git.GetRef(ctx, pullRequestArg.sourceOwner, 
		pullRequestArg.sourceRepo, "refs/heads/"+pullRequestArg.commitBranch); err == nil {
		return ref, nil
	}

	// We consider that an error means the branch has not been found and needs to
	// be created.
	if pullRequestArg.commitBranch == pullRequestArg.baseBranch {
		return nil, fmt.Errorf("the commit branch does not exist but `-base-branch` is the same as `-commit-branch`")
	}

	if pullRequestArg.baseBranch == "" {
		return nil, fmt.Errorf("the `-base-branch` should not be set to an empty string when the branch specified by `-commit-branch` does not exists")
	}

	var baseRef *github.Reference
	if baseRef, _, err = client.Git.GetRef(ctx, pullRequestArg.sourceOwner, 
		pullRequestArg.sourceRepo, "refs/heads/"+pullRequestArg.baseBranch); err != nil {
		return nil, err
	}
	newRef := &github.Reference{Ref: github.String("refs/heads/" + pullRequestArg.commitBranch), Object: &github.GitObject{SHA: baseRef.Object.SHA}}
	ref, _, err = client.Git.CreateRef(ctx, pullRequestArg.sourceOwner, pullRequestArg.sourceRepo, newRef)
	return ref, err
}


// getTree generates the tree to commit based on the given files and the commit
// of the ref you got in getRef.
func getTree(pullRequestArg *PullRequestArg, ref *github.Reference, client *github.Client, ctx context.Context) (tree *github.Tree, err error) {
	
	// Create a tree with what to commit.
	entries := []*github.TreeEntry{}

	// Load each file into the tree.
	for _, fileArg := range strings.Split(pullRequestArg.sourceFiles, ",") {
		file, content, err := getFileContent(fileArg)
		if err != nil {
			return nil, err
		}
		entries = append(entries, &github.TreeEntry{Path: github.String(file), Type: github.String("blob"), Content: github.String(string(content)), Mode: github.String("100644")})
	}

	tree, _, err = client.Git.CreateTree(ctx, pullRequestArg.sourceOwner, pullRequestArg.sourceRepo, *ref.Object.SHA, entries)
	return tree, err
}


// getFileContent loads the local content of a file and return the target name
// of the file in the target repository and its contents.
func getFileContent(fileArg string) (targetName string, b []byte, err error) {
	var localFile string
	files := strings.Split(fileArg, ":")
	switch {
	case len(files) < 1:
		return "", nil, fmt.Errorf("empty `-files` parameter")
	case len(files) == 1:
		localFile = files[0]
		targetName = files[0]
	default:
		localFile = files[0]
		targetName = files[1]
	}

	b, err = ioutil.ReadFile(localFile)
	return targetName, b, err
}


// pushCommit creates the commit in the given reference using the given tree.
func pushCommit(pullRequestArg *PullRequestArg, ref *github.Reference, tree *github.Tree, client *github.Client, ctx context.Context) (err error) {
	// Get the parent commit to attach the commit to.
	parent, _, err := client.Repositories.GetCommit(ctx, pullRequestArg.sourceOwner, pullRequestArg.sourceRepo, *ref.Object.SHA, nil)
	if err != nil {
		return err
	}
	// This is not always populated, but is needed.
	parent.Commit.SHA = parent.SHA

	// Create the commit using the tree.
	date := time.Now()
	author := &github.CommitAuthor{Date: &date, Name: &pullRequestArg.authorName, Email: &pullRequestArg.authorEmail}
	commit := &github.Commit{Author: author, Message: &pullRequestArg.commitMessage, Tree: tree, Parents: []*github.Commit{parent.Commit}}
	newCommit, _, err := client.Git.CreateCommit(ctx, pullRequestArg.sourceOwner, pullRequestArg.sourceRepo, commit)
	if err != nil {
		return err
	}

	// Attach the commit to the master branch.
	ref.Object.SHA = newCommit.SHA
	_, _, err = client.Git.UpdateRef(ctx, pullRequestArg.sourceOwner, pullRequestArg.sourceRepo, ref, false)
	return err
}


// createPR creates a pull request. Based on: https://godoc.org/github.com/google/go-github/github#example-PullRequestsService-Create
func createPR(pullRequestArg *PullRequestArg, client *github.Client, ctx context.Context) (err error) {
	if pullRequestArg.prSubject == "" {
		return fmt.Errorf("missing `-pr-title` flag; skipping PR creation")
	}

	if pullRequestArg.prRepoOwner != "" && pullRequestArg.prRepoOwner != pullRequestArg.sourceOwner {
		pullRequestArg.commitBranch = fmt.Sprintf("%s:%s", pullRequestArg.sourceOwner, pullRequestArg.commitBranch)
	} else {
		pullRequestArg.prRepoOwner = pullRequestArg.sourceOwner
	}

	if pullRequestArg.prRepo == "" {
		pullRequestArg.prRepo = pullRequestArg.sourceRepo
	}

	ref, err := getRef(pullRequestArg, client, ctx)
	if err != nil {
		fmt.Printf("Unable to get/create the commit reference: %v\n", err)
		return err
	}
	if ref == nil {
		fmt.Printf("No error where returned but the reference is nil")
		return err
	}

	tree, err := getTree(pullRequestArg, ref, client, ctx)
	if err != nil {
		fmt.Printf("Unable to create the tree based on the provided files: %v\n", err)
		return err
	}

	if err := pushCommit(pullRequestArg, ref, tree, client, ctx); err != nil {
		fmt.Printf("Unable to create the commit: %v\n", err)
		return err
	}

	newPR := &github.NewPullRequest{
		Title:               &pullRequestArg.prSubject,
		Head:                &pullRequestArg.commitBranch,
		Base:                &pullRequestArg.baseBranch,
		Body:                &pullRequestArg.prDescription,
		MaintainerCanModify: github.Bool(true),
	}

	// The Create API doc is https://github.com/google/go-github/blob/master/github/pulls.go#L258
	pr, _, err := client.PullRequests.Create(ctx, pullRequestArg.prRepoOwner, pullRequestArg.prRepo, newPR)
	if err != nil {
		fmt.Printf("Creating pull request failed with %v error", err)
		return err
	}

	fmt.Printf("PR created: %s\n", pr.GetHTMLURL())
	return nil
}

// download the clerkgen cli with the given tag
func downloadReleaseAssetViaGobox(ctx context.Context) error {
	opts := &release.FetchOptions{
		RepoURL:   "https://github.com/getoutreach/clerkgen",
		Tag:       tagName,
		AssetName: "clerkgen_" + tagName[1:] + "_linux_amd64.tar.gz",
	}
	token := (cfg.SecretData)(os.Getenv("GITHUB_TOKEN"))
	rc, _, _, err := release.Fetch(ctx, token, opts)
	if err != nil {
		fmt.Printf("Fetch asset failed with %v error", err)
		return err
	}

	data, err := ioutil.ReadAll(rc)
	if err != nil {
		fmt.Printf("Read asset failed with %v error", err)
		return err
	}
	ioutil.WriteFile("clerkgen_" + tagName[1:] + "_linux_amd64.tar.gz", data, 0755)
	rc.Close()
	return nil
}


// Run command-line tool with variable arguments
func runCommand(program string, args ...string) {
	cmd := exec.Command(program, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil { 
		fmt.Println(err.Error())
        return 
	}
}

// get all files under the given directory dir 
func getSourceFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            fmt.Println(err)
            return err
        }

		if (!info.IsDir()) {
			files = append(files, path)
		}

        fmt.Printf("dir: %v: name: %s\n", info.IsDir(), path)
        return nil
    })

    if err != nil {
        fmt.Println("Walking direction failed with %v error", err)
		return nil, err
    }

	return files, err
}
