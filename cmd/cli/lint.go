package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/raffis/rageta/crds"
	"github.com/raffis/rageta/internal/validate"
	"github.com/spf13/cobra"
	yamlv2 "gopkg.in/yaml.v2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/kubectl-validate/pkg/utils"
	"sigs.k8s.io/kubectl-validate/pkg/validator"
)

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Lint",
	Long: `The push artifact command creates a tarball from the given directory or the single file and uploads the artifact to an OCI repository.
The command can read the credentials from '~/.docker/config.json' but they can also be passed with --creds. It can also login to a supported provider with the --provider flag.`,
	Example: `  # Push manifests to GHCR using the short Git SHA as the OCI artifact tag
  echo $GITHUB_PAT | docker login ghcr.io --username flux --password-stdin
  flux push artifact oci://ghcr.io/org/config/app:$(git rev-parse --short HEAD) \
	--path="./path/to/local/manifests" \
	--source="$(git config --get remote.origin.url)" \
	--revision="$(git branch --show-current)@sha1:$(git rev-parse HEAD)"`,
	RunE: lintRun,
}

type lintFlags struct {
	outputFormat OutputFormat
}

var lintArgs = newLintFlags()

func newLintFlags() lintFlags {
	return lintFlags{
		outputFormat: OutputHuman,
	}
}

func init() {
	lintCmd.Flags().VarP(&lintArgs.outputFormat, "output", "o", "Output format. Choice of: \"human\" or \"json\"")
	rootCmd.AddCommand(lintCmd)
}

type OutputFormat string

const (
	OutputHuman OutputFormat = "human"
	OutputJSON  OutputFormat = "json"
)

// String is used both by fmt.Print and by Cobra in help text
func (e *OutputFormat) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *OutputFormat) Set(v string) error {
	switch v {
	case "human", "json":
		*e = OutputFormat(v)
		return nil
	default:
		return fmt.Errorf(`must be one of "human", or "json"`)
	}
}

// Type is only used in help text
func (e *OutputFormat) Type() string {
	return "OutputFormat"
}

func lintRun(cmd *cobra.Command, args []string) error {
	// tool fetches openapi in the following priority order:
	factory, err := validator.New(validate.NewLocalCRDFiles(crds.FS))
	if err != nil {
		return err
	}

	files, err := utils.FindFiles(args...)
	if err != nil {
		return err
	}

	hasError := false
	if lintArgs.outputFormat == OutputHuman {
		for _, path := range files {
			fmt.Fprintf(cmd.OutOrStdout(), "\n\033[1m%v\033[0m...", path)
			var errs []error
			for _, err := range ValidateFile(path, factory) {
				if err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) != 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\033[31mERROR\033[0m")
				for _, err := range errs {
					fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				}
				hasError = true
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\033[32mOK\033[0m")
			}
		}
	} else {
		res := map[string][]metav1.Status{}
		for _, path := range files {
			for _, err := range ValidateFile(path, factory) {
				res[path] = append(res[path], errorToStatus(err))
				hasError = hasError || err != nil
			}
		}
		data, e := json.MarshalIndent(res, "", "    ")
		if e != nil {
			return fmt.Errorf("failed to render results into JSON: %w", e)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}

	return nil
}

func ValidateFile(filePath string, resolver *validator.Validator) []error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return []error{fmt.Errorf("error reading file: %w", err)}
	}

	if utils.IsYaml(filePath) {
		documents, err := utils.SplitYamlDocuments(fileBytes)
		if err != nil {
			return []error{err}
		}
		var errs []error
		for _, document := range documents {
			if utils.IsEmptyYamlDocument(document) {
				errs = append(errs, nil)
			} else {
				errs = append(errs, ValidateDocument(document, resolver))
			}
		}
		return errs
	} else {
		return []error{
			ValidateDocument(fileBytes, resolver),
		}
	}
}

func ValidateDocument(document []byte, resolver *validator.Validator) error {
	_, parsed, err := resolver.Parse(document)
	if err != nil {
		return err
	}
	return resolver.Validate(parsed)
}

type joinedErrors interface {
	Unwrap() []error
}

func errorToStatus(err error) metav1.Status {
	var statusErr *k8serrors.StatusError
	var fieldError *field.Error
	var errorList utilerrors.Aggregate
	var errorList2 joinedErrors
	if errors.As(err, &errorList2) {
		errs := errorList2.Unwrap()
		if len(errs) == 0 {
			return metav1.Status{Status: metav1.StatusSuccess}
		}
		var fieldErrors field.ErrorList
		var otherErrors []error
		var yamlErrors []metav1.StatusCause

		for _, e := range errs {
			var fieldError *field.Error
			var yamlError *yamlv2.TypeError

			if errors.As(e, &fieldError) {
				fieldErrors = append(fieldErrors, fieldError)
			} else if errors.As(e, &yamlError) {
				for _, sub := range yamlError.Errors {
					yamlErrors = append(yamlErrors, metav1.StatusCause{
						Message: sub,
					})
				}
			} else {
				otherErrors = append(otherErrors, e)
			}
		}

		if len(otherErrors) > 0 {
			return k8serrors.NewInternalError(err).ErrStatus
		} else if len(yamlErrors) > 0 && len(fieldErrors) == 0 {
			// YAML type errors are raised during decoding
			return metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusBadRequest,
				Reason: metav1.StatusReasonBadRequest,
				Details: &metav1.StatusDetails{
					Causes: yamlErrors,
				},
				Message: "failed to unmarshal document to YAML",
			}
		}
		return k8serrors.NewInvalid(schema.GroupKind{}, "", fieldErrors).ErrStatus
	} else if errors.As(err, &statusErr) {
		return statusErr.ErrStatus
	} else if errors.As(err, &fieldError) {
		return k8serrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{fieldError}).ErrStatus
	} else if errors.As(err, &errorList) {
		errs := errorList.Errors()
		var fieldErrs []*field.Error
		var otherErrs []error
		for _, e := range errs {
			fieldError = nil
			if errors.As(e, &fieldError) {
				fieldErrs = append(fieldErrs, fieldError)
			} else {
				otherErrs = append(otherErrs, e)
			}
		}
		if len(otherErrs) == 0 {
			return k8serrors.NewInvalid(schema.GroupKind{}, "", fieldErrs).ErrStatus
		} else {
			return k8serrors.NewInternalError(err).ErrStatus
		}
	} else if err != nil {
		return k8serrors.NewInternalError(err).ErrStatus
	}
	return metav1.Status{Status: metav1.StatusSuccess}
}
