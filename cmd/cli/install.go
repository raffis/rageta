package main

/*
Copyright 2022 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"github.com/spf13/cobra"

	fluxoci "github.com/fluxcd/pkg/oci"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install",
	Long: `The push artifact command creates a tarball from the given directory or the single file and uploads the artifact to an OCI repository.
The command can read the credentials from '~/.docker/config.json' but they can also be passed with --creds. It can also login to a supported provider with the --provider flag.`,
	Example: `  # Push manifests to GHCR using the short Git SHA as the OCI artifact tag
  echo $GITHUB_PAT | docker login ghcr.io --username flux --password-stdin
  flux push artifact oci://ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)"`,
	RunE: installRun,
}

type installFlags struct {
	repository string
	debug      bool
}

var installArgs = newInstallFlags()

func newInstallFlags() installFlags {
	return installFlags{
		//provider: sourcev1.GenericOCIProvider,
	}
}

func init() {
	installCmd.Flags().StringVarP(&installArgs.repository, "repository", "r", "", "Name of the repository to install from")
	installCmd.Flags().BoolVarP(&pushArtifactArgs.debug, "debug", "", false, "display logs from underlying library")
	rootCmd.AddCommand(installCmd)

	fluxoci.CanonicalConfigMediaType = "application/rageta"
}

func installRun(cmd *cobra.Command, args []string) error {

	return nil
}
