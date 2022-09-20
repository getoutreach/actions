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
	"log"
	"net/http"
	"sort"

	// "github.com/pkg/errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/aws/session"
	// "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/getoutreach/actions/internal/gh"
	"github.com/google/go-github/v47/github"
	actions "github.com/sethvargo/go-githubactions"
	"golang.org/x/oauth2"
	"github.com/bradfitz/slice"

	commonConfig "github.com/getoutreach/clerkgen/cmd/config"
)

const (
	format = "2006-Jan-02"
)

// S3Client: interface for s3 client
type S3Client interface {
	ListObjectsV2(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
}

var client *github.Client
var sourceOwner *string
var sourceRepo *string
var sourceFiles *string
var authorName *string
var authorEmail *string
var commitBranch *string
var baseBranch *string
var commitMessage *string
var prRepo *string
var prRepoOwner *string
var prSubject *string
var prBranch *string
var prDescription *string
var ctx context.Context


func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	var client_err error
	client, client_err = gh.NewClient(ctx, false)
	if client_err != nil {
		actions.Errorf("create github client: %v", client_err)
		return
	}

	ghContext, err := actions.Context()
	if err != nil {
		actions.Errorf("unable to get action context: %v", err)
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
	// Get latest timestamp of schema configs from S3
	maxTime, err := GetSchemaConfigLatestTimestamp(commonConfig.S3Bucket, commonConfig.ProtobufS3Prefix, s3Client, ctx)
	if err != nil {
		fmt.Printf("error while dowloading schema config from S3: %v\n", err)
		return err
	}

	now := time.Now()
	if now.After(maxTime) && now.Add(-1 * time.Hour).Before(maxTime) {
		// get release asset
		tagName, err2 := DownloadReleaseAsset(ctx, client)
		if err2 != nil {
			fmt.Printf("DownloadReleaseAsset returned error: %v", err2)
			return err2
		}

		// run clerkgen
		RunCommand("tar", "-xzvf", "clerkgen_" + tagName + "_linux_arm64.tar.gz") 
		RunCommand("./clerkgenproto", "--out-dir", "out", "--all-schemas", "-l", "go")

		sourceFiles, err = strings.Join(GetSourceFiles("./out"), ",")

		// create pull request on a branch
		// example is here: https://github.com/google/go-github/blob/master/example/commitpr/main.go
		Initialize()
		ref, err := getRef()
		if err != nil {
			log.Fatalf("Unable to get/create the commit reference: %s\n", err)
			return err
		}
		if ref == nil {
			log.Fatalf("No error where returned but the reference is nil")
			return err
		}

		tree, err := getTree(ref)
		if err != nil {
			log.Fatalf("Unable to create the tree based on the provided files: %s\n", err)
			return err
		}

		if err := pushCommit(ref, tree); err != nil {
			log.Fatalf("Unable to create the commit: %s\n", err)
			return err
		}

		if err := createPR(); err != nil {
			log.Fatalf("Error while creating the pull request: %s", err)
			return err
		}

		// run cleanups
		RunCommand("rm", "-rf", "out") 
	}

	return nil
}


// Set variables used for submitting PRs
func Initialize()
{
	*sourceOwner = "getoutreach"
	*sourceRepo = "clerkcommons"
	*authorName = "Outreach CI"
	*authorEmail = "outreach-ci@users.noreply.github.com"
	*commitBranch = "feature"
	*baseBranch = "main"
	*commitMessage = "Generate new clerk stubs"
	*prRepo = ""
	*prRepoOwner = ""
	*prBranch = "main"
	*prSubject = "feat: generate new clerk stubs"
	*prDescription = "Automatically generate new clerk stubs per hour"
}

// There are the configs of the schemas that are registered in schema registry and uploaded to S3
// It goes over the entire folder in s3 and returns the latest timestamp of the files in that folder
func GetSchemaConfigLatestTimestamp(bucket string, prefix string, s3Client S3Client,
	ctx context.Context) (time.Time, error) {

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	}

	maxTime, _  := time.Parse(format, "2012-May-19") // Just picking a really old date to start comparison with.
	
	var latestObj *s3.Object
	for {
		out, err := s3Client.ListObjectsV2(input)
		if err != nil {
			return maxTime, fmt.Errorf("error listing objects for bucket '%s' and prefix '%s", bucket, prefix)
		}
		for _, obj := range out.Contents {
			if obj.LastModified.After(maxTime) {
				maxTime = *obj.LastModified
				latestObj = obj
			}
		}
		if out.NextContinuationToken == nil {
			break
		}
		input.ContinuationToken = out.NextContinuationToken
	}

	if latestObj == nil {
		return maxTime, fmt.Errorf("no objects found in bucket '%s' and prefix '%s", bucket, prefix)
	}

	return maxTime, nil
}


// getRef returns the commit branch reference object if it exists or creates it
// from the base branch before returning it.
func getRef() (ref *github.Reference, err error) {
	if ref, _, err = client.Git.GetRef(ctx, *sourceOwner, *sourceRepo, "refs/heads/"+*commitBranch); err == nil {
		return ref, nil
	}

	// We consider that an error means the branch has not been found and needs to
	// be created.
	if *commitBranch == *baseBranch {
		return nil, fmt.Errorf("the commit branch does not exist but `-base-branch` is the same as `-commit-branch`")
	}

	if *baseBranch == "" {
		return nil, fmt.Errorf("the `-base-branch` should not be set to an empty string when the branch specified by `-commit-branch` does not exists")
	}

	var baseRef *github.Reference
	if baseRef, _, err = client.Git.GetRef(ctx, *sourceOwner, *sourceRepo, "refs/heads/"+*baseBranch); err != nil {
		return nil, err
	}
	newRef := &github.Reference{Ref: github.String("refs/heads/" + *commitBranch), Object: &github.GitObject{SHA: baseRef.Object.SHA}}
	ref, _, err = client.Git.CreateRef(ctx, *sourceOwner, *sourceRepo, newRef)
	return ref, err
}


// getTree generates the tree to commit based on the given files and the commit
// of the ref you got in getRef.
func getTree(ref *github.Reference) (tree *github.Tree, err error) {
	// Create a tree with what to commit.
	entries := []*github.TreeEntry{}

	// Load each file into the tree.
	for _, fileArg := range strings.Split(*sourceFiles, ",") {
		file, content, err := getFileContent(fileArg)
		if err != nil {
			return nil, err
		}
		entries = append(entries, &github.TreeEntry{Path: github.String(file), Type: github.String("blob"), Content: github.String(string(content)), Mode: github.String("100644")})
	}

	tree, _, err = client.Git.CreateTree(ctx, *sourceOwner, *sourceRepo, *ref.Object.SHA, entries)
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
func pushCommit(ref *github.Reference, tree *github.Tree) (err error) {
	// Get the parent commit to attach the commit to.
	parent, _, err := client.Repositories.GetCommit(ctx, *sourceOwner, *sourceRepo, *ref.Object.SHA, nil)
	if err != nil {
		return err
	}
	// This is not always populated, but is needed.
	parent.Commit.SHA = parent.SHA

	// Create the commit using the tree.
	date := time.Now()
	author := &github.CommitAuthor{Date: &date, Name: authorName, Email: authorEmail}
	commit := &github.Commit{Author: author, Message: commitMessage, Tree: tree, Parents: []*github.Commit{parent.Commit}}
	newCommit, _, err := client.Git.CreateCommit(ctx, *sourceOwner, *sourceRepo, commit)
	if err != nil {
		return err
	}

	// Attach the commit to the master branch.
	ref.Object.SHA = newCommit.SHA
	_, _, err = client.Git.UpdateRef(ctx, *sourceOwner, *sourceRepo, ref, false)
	return err
}


// createPR creates a pull request. Based on: https://godoc.org/github.com/google/go-github/github#example-PullRequestsService-Create
func createPR() (err error) {
	if *prSubject == "" {
		return fmt.Errorf("missing `-pr-title` flag; skipping PR creation")
	}

	if *prRepoOwner != "" && *prRepoOwner != *sourceOwner {
		*commitBranch = fmt.Sprintf("%s:%s", *sourceOwner, *commitBranch)
	} else {
		prRepoOwner = sourceOwner
	}

	if *prRepo == "" {
		prRepo = sourceRepo
	}

	newPR := &github.NewPullRequest{
		Title:               prSubject,
		Head:                commitBranch,
		Base:                prBranch,
		Body:                prDescription,
		MaintainerCanModify: github.Bool(true),
	}

	pr, _, err := client.PullRequests.Create(ctx, *prRepoOwner, *prRepo, newPR)
	if err != nil {
		fmt.Printf("Creating pull request failed with %v error", err)
		return err
	}

	fmt.Printf("PR created: %s\n", pr.GetHTMLURL())
	return nil
}


func DownloadReleaseAsset(ctx context.Context, client *github.Client) (string, error) {
	releases, _, err :=  client.Repositories.ListReleases(ctx, "getoutreach", "clerkgen", nil)
	if err != nil {
		fmt.Printf("Repositories.ListReleases with owner %s and repo %s returned error: %v", 
				   "getoutreach", "clerkgen", err)
		return "", err
	}

	// sort descendingly in terms of PublishedAt
	sort.Slice(releases, func(i, j int) bool {
		return (*releases[i].PublishedAt).Time.After((*releases[j].PublishedAt).Time)
	})

	// clerkgen_1.27.4_linux_amd64.tar.gz
	assetName := "clerkgen_" + (*releases[0].TagName)[1:] + "_linux_arm64.tar.gz"
	assets, _, err2 := client.Repositories.ListReleaseAssets(ctx, "getoutreach", "clerkgen", *releases[0].ID, nil)
	if err2 != nil {
		fmt.Printf("Repositories.ListReleaseAssets with owner %s and repo %s returned error: %v", 
				   "getoutreach", "clerkgen", err2)
		return "", err2
	}

	var assetId int64

	for _, item := range assets {
		if *item.Name == assetName {

			assetId = *item.ID
			break
		} 
	}

	reader, _, err3 := client.Repositories.DownloadReleaseAsset(ctx, "getoutreach", "clerkgen",
																assetId, http.DefaultClient)
	if err3 != nil {
		fmt.Printf("Repositories.DownloadReleaseAsset with owner %s and repo %s returned error: %v", 
				   "getoutreach", "clerkgen", err3)
		return "", err3
	}
	data, err3 = ioutil.ReadAll(reader)
	if err3 != nil {
		fmt.Printf("Repositories.DownloadReleaseAsset with owner %s and repo %s returned bad reader: %v", 
				   "getoutreach", "clerkgen", err3)
		return "", err3
	}
	ioutil.WriteFile(assetName, data, 0755)
	reader.Close()
	return (*releases[0].TagName)[1:], nil
}


// first command untar: tar -xf archive.tar.gz
// second command: /Users/yinfangzhuang/Downloads/clerkgen_1.27.3_darwin_amd64/clerkgenproto --out-dir yz --all-schemas -l go
func RunCommand(program string, args ...string) {
    cmd := exec.Command(program, args...)
    stdout, err := cmd.Output()

    if err != nil {
        fmt.Println(err.Error())
        return
    }

    // Print the output
    fmt.Println(string(stdout))
}

func GetSourceFiles(dir string) ([]string, error) {
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
