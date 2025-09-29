package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/provider"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/spf13/cobra"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "tag pipeline",
	Long: `The tag command creates a tarball from the given directory or the single file and stores the artifact in the local database.
The command can read the credentials from '~/.docker/config.json' but they can also be passed with --creds. It can also login to a supported provider with the --provider flag.`,
	Example: `  # tag manifests to local database using the short Git SHA as the artifact tag
  rageta tag ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)"

  # tag and sign artifact with cosign
  digest_url = $(rageta tag artifact \
	oci://ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)" \
	--path="./path/to/local/manifest.yaml" \
	--output json | \
	jq -r '. | .repository + "@" + .digest')
  cosign sign $digest_url

  # tag manifests passed into stdin to local database and set custom annotations
  kustomize build . | rageta tag ghcr.io/org/config/app:$(git rev-parse --short HEAD) -f - \ 
    --source="$(git config --get remote.origin.url)" \
    --revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)" \
    --annotations='org.opencontainers.image.licenses=Apache-2.0' \
    --annotations='org.opencontainers.image.documentation=https://app.org/docs' \
    --annotations='org.opencontainers.image.description=Production config.'

  # tag single manifest file to local database using the short Git SHA as the artifact tag
  rageta tag ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--path="./path/to/local/manifest.yaml" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)"

  # tag manifests to local database using the Git tag as the artifact tag
  rageta tag docker.io/org/my-pipeline:$(git tag --points-at HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git tag --points-at HEAD)@sha1:$(git rev-parse HEAD)"
`,
	RunE: tagCmdRun,
}

type tagFlags struct {
	imgFlags
}

var tagArgs = newTagFlags()

func newTagFlags() tagFlags {
	return tagFlags{}
}

func init() {
	tagCmd.Flags().StringVarP(&tagArgs.path, "path", "f", "", "path to the directory where the Kubernetes manifests are located")
	tagCmd.Flags().StringVar(&tagArgs.source, "source", "", "the source address, e.g. the Git URL")
	tagCmd.Flags().StringVar(&tagArgs.revision, "revision", "", "the source revision in the format '<branch|tag>@sha1:<commit-sha>'")
	tagCmd.Flags().StringArrayVarP(&tagArgs.tags, "tags", "t", nil, "tag additional tags")
	tagCmd.Flags().StringArrayVarP(&tagArgs.annotations, "annotations", "a", nil, "Set custom annotations in the format '<key>=<value>'")

	rootCmd.AddCommand(tagCmd)
}

func tagCmdRun(cmd *cobra.Command, args []string) error {
	imgFlags := imgFlags{
		path:        tagArgs.path,
		source:      tagArgs.source,
		revision:    tagArgs.revision,
		annotations: tagArgs.annotations,
		tags:        tagArgs.tags,
	}

	path, err := prepareOCIFile(imgFlags, args)
	if err != nil {
		return err
	}

	defer func() {
		if path != tagArgs.path && path != "" {
			if err := os.RemoveAll(filepath.Dir(path)); err != nil {
				logger.V(3).Error(err, "error removing temp manifest directory")
			}
		}
	}()

	artifactURL := args[0]

	// Initialize database
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)

	dbFile, err := os.OpenFile(rootArgs.dbPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() {
		_ = dbFile.Close()
	}()

	localDB, err := provider.OpenDatabase(dbFile, decoder, encoder)
	if err != nil {
		return fmt.Errorf("failed to decode database: %w", err)
	}

	// Read manifest from path
	manifestData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse annotations
	annotations := map[string]string{}
	for _, annotation := range tagArgs.annotations {
		kv := strings.Split(annotation, "=")
		if len(kv) != 2 {
			return fmt.Errorf("invalid annotation %s, must be in the format key=value", annotation)
		}
		annotations[kv[0]] = kv[1]
	}

	// Store manifest in database
	err = localDB.Add(artifactURL, manifestData)
	if err != nil {
		return fmt.Errorf("tagging artifact failed: %w", err)
	}

	err = localDB.Persist(dbFile)
	if err != nil {
		return fmt.Errorf("failed to persist database: %w", err)
	}

	// Handle additional tags
	for _, tag := range tagArgs.tags {
		taggedURL := strings.Replace(artifactURL, ":", ":"+tag+":", 1)
		err = localDB.Add(taggedURL, manifestData)
		if err != nil {
			return fmt.Errorf("tagging artifact failed: %w", err)
		}
	}

	fmt.Printf("Successfully tagged %s in local database\n", artifactURL)

	return nil
}
