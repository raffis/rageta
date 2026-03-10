package run

import (
	"fmt"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/spf13/pflag"
)

const (
	pullAlways  = "always"
	pullMissing = "missing"
	pullNever   = "never"
)

type ImagePolicyOptions struct {
	Policy string
}

func (s ImagePolicyOptions) Build() Step {
	return &ImagePolicy{
		opts: s,
	}
}

func (s ImagePolicyOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.Policy, "pull", "", s.Policy, "Pull image before running. one of [always, missing, never].")
}

type ImagePolicy struct {
	opts ImagePolicyOptions
}

func (s *ImagePolicy) Run(rc *RunContext, next Next) error {
	policy, err := s.imagePullPolicy()
	if err != nil {
		return err
	}
	rc.ImagePullPolicy = policy
	return next(rc)
}

func (s *ImagePolicy) imagePullPolicy() (cruntime.PullImagePolicy, error) {
	switch s.opts.Policy {
	case pullAlways:
		return cruntime.PullImagePolicyAlways, nil
	case pullMissing:
		return cruntime.PullImagePolicyMissing, nil
	case pullNever:
		return cruntime.PullImagePolicyNever, nil
	default:
		return "", fmt.Errorf("invalid pull policy given: %s", s.opts.Policy)
	}
}
