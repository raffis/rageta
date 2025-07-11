package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/logs"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/fluxcd/pkg/oci"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push pipeline",
	Long: `The push command creates a tarball from the given directory or the single file and uploads the artifact to an OCI repository.
The command can read the credentials from '~/.docker/config.json' but they can also be passed with --creds. It can also login to a supported provider with the --provider flag.`,
	Example: `  # Push manifests to GHCR using the short Git SHA as the OCI artifact tag
  echo $GITHUB_PAT | docker login ghcr.io --username rageta --password-stdin
  rageta push ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)"

  # Push and sign artifact with cosign
  digest_url = $(rageta push artifact \
	oci://ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)" \
	--path="./path/to/local/manifest.yaml" \
	--output json | \
	jq -r '. | .repository + "@" + .digest')
  cosign sign $digest_url

  # Push manifests passed into stdin to GHCR and set custom OCI annotations
  kustomize build . | rageta push ghcr.io/org/config/app:$(git rev-parse --short HEAD) -f - \ 
    --source="$(git config --get remote.origin.url)" \
    --revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)" \
    --annotations='org.opencontainers.image.licenses=Apache-2.0' \
    --annotations='org.opencontainers.image.documentation=https://app.org/docs' \
    --annotations='org.opencontainers.image.description=Production config.'

  # Push single manifest file to GHCR using the short Git SHA as the OCI artifact tag
  echo $GITHUB_PAT | docker login ghcr.io --username rageta --password-stdin
  rageta push ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--path="./path/to/local/manifest.yaml" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)"

  # Push manifests to Docker Hub using the Git tag as the OCI artifact tag
  echo $DOCKER_PAT | docker login --username rageta --password-stdin
  rageta push docker.io/org/my-pipeline:$(git tag --points-at HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git tag --points-at HEAD)@sha1:$(git rev-parse HEAD)"

  # Login directly to the registry provider
  # You might need to export the following variable if you use local config files for AWS:
  # export AWS_SDK_LOAD_CONFIG=1
  rageta push <account>.dkr.ecr.<region>.amazonaws.com/my-pipeline:$(git tag --points-at HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git tag --points-at HEAD)@sha1:$(git rev-parse HEAD)" \
	--provider aws

  # Login by passing credentials directly
  rageta push docker.io/org/my-pipeline:$(git tag --points-at HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git tag --points-at HEAD)@sha1:$(git rev-parse HEAD)" \
	--creds rageta:$DOCKER_PAT
`,
	RunE: pushCmdRun,
}

type pushFlags struct {
	path        string
	source      string
	revision    string
	annotations []string
	tags        []string
	output      string
	debug       bool
	ociOptions  *ocisetup.Options
}

var pushArgs = newpushFlags()

func newpushFlags() pushFlags {
	opts := ocisetup.DefaultOptions()
	opts.Timeout = rootArgs.timeout
	return pushFlags{
		ociOptions: opts,
	}
}

func init() {
	pushCmd.Flags().StringVarP(&pushArgs.path, "path", "f", "", "path to the directory where the Kubernetes manifests are located")
	pushCmd.Flags().StringVar(&pushArgs.source, "source", "", "the source address, e.g. the Git URL")
	pushCmd.Flags().StringVar(&pushArgs.revision, "revision", "", "the source revision in the format '<branch|tag>@sha1:<commit-sha>'")
	pushCmd.Flags().StringArrayVarP(&pushArgs.tags, "tags", "t", nil, "Push additional tags")
	pushCmd.Flags().StringArrayVarP(&pushArgs.annotations, "annotations", "a", nil, "Set custom OCI annotations in the format '<key>=<value>'")
	pushCmd.Flags().StringVarP(&pushArgs.output, "output", "o", "",
		"the format in which the artifact digest should be printed, can be 'json' or 'yaml'")
	pushCmd.Flags().BoolVarP(&pushArgs.debug, "debug", "", false, "display logs from underlying library")

	pushArgs.ociOptions.BindFlags(pushCmd.Flags())

	rootCmd.AddCommand(pushCmd)
	oci.CanonicalConfigMediaType = "application/rageta"
}

func pushCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("artifact URL is required")
	}
	ociURL := args[0]

	if pushArgs.source == "" {
		return fmt.Errorf("--source is required")
	}

	if pushArgs.revision == "" {
		return fmt.Errorf("--revision is required")
	}

	if pushArgs.path == "" {
		return fmt.Errorf("invalid path %q", pushArgs.path)
	}

	ctx := cmd.Context()
	if rootArgs.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rootArgs.timeout)
		defer cancel()
	}

	pushArgs.ociOptions.URL = ociURL
	ociClient, err := pushArgs.ociOptions.Build(ctx)
	if err != nil {
		return err
	}

	path := pushArgs.path
	if pushArgs.path == "-" || pushArgs.path == "/dev/stdin" {
		path, err = saveReaderToFile(os.Stdin)
		if err != nil {
			return err
		}

		defer os.Remove(path)
	}

	if fstat, err := os.Stat(path); err != nil {
		return fmt.Errorf("invalid path '%s', must point to an existing directory or file: %w", path, err)
	} else if !fstat.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return err
		}

		defer f.Close()

		path, err = saveReaderToFile(f)
		if err != nil {
			return err
		}

		defer os.Remove(path)
	}

	annotations := map[string]string{}
	for _, annotation := range pushArgs.annotations {
		kv := strings.Split(annotation, "=")
		if len(kv) != 2 {
			return fmt.Errorf("invalid annotation %s, must be in the format key=value", annotation)
		}
		annotations[kv[0]] = kv[1]
	}

	if pushArgs.debug {
		// direct logs from crane library to stderr
		// this can be useful to figure out things happening underneath e.g when the library is retrying a request
		logs.Warn.SetOutput(os.Stderr)
	}

	meta := oci.Metadata{
		Source:      pushArgs.source,
		Revision:    pushArgs.revision,
		Annotations: annotations,
	}

	if pushArgs.output == "" {
		fmt.Printf("pushing artifact to %s\n", ociURL)
	}

	digestURL, err := ociClient.Push(ctx, ociURL, path,
		oci.WithPushMetadata(meta),
	)
	if err != nil {
		return fmt.Errorf("pushing artifact failed: %w", err)
	}

	for _, tag := range pushArgs.tags {
		_, err = ociClient.Tag(ctx, ociURL, tag)
		if err != nil {
			return fmt.Errorf("remote tag failed: %w", err)
		}
	}

	digest, err := name.NewDigest(digestURL)
	if err != nil {
		return fmt.Errorf("artifact digest parsing failed: %w", err)
	}

	tag, err := name.NewTag(ociURL)
	if err != nil {
		return fmt.Errorf("artifact tag parsing failed: %w", err)
	}

	info := struct {
		URL        string `json:"url"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Digest     string `json:"digest"`
	}{
		URL:        fmt.Sprintf("oci://%s", digestURL),
		Repository: digest.Repository.Name(),
		Tag:        tag.TagStr(),
		Digest:     digest.DigestStr(),
	}

	switch pushArgs.output {
	case "json":
		marshalled, err := json.MarshalIndent(&info, "", "  ")
		if err != nil {
			return fmt.Errorf("artifact digest JSON conversion failed: %w", err)
		}
		marshalled = append(marshalled, "\n"...)
		rootCmd.Print(string(marshalled))
	case "yaml":
		marshalled, err := yaml.Marshal(&info)
		if err != nil {
			return fmt.Errorf("artifact digest YAML conversion failed: %w", err)
		}
		rootCmd.Print(string(marshalled))
	default:
		fmt.Printf("Successfully pushed to %s\n", digestURL)
	}

	return nil
}

func saveReaderToFile(reader io.Reader) (string, error) {
	b, err := io.ReadAll(bufio.NewReader(reader))
	if err != nil {
		return "", err
	}
	b = bytes.TrimRight(b, "\r\n")

	tmpDir, err := os.MkdirTemp("", "push")
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(filepath.Join(tmpDir, "main.yaml"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return "", err
	}

	defer f.Close()

	if _, err := f.Write(b); err != nil {
		return "", fmt.Errorf("error writing stdin to file: %w", err)
	}

	return f.Name(), nil
}
