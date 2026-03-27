package run

import (
	"fmt"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/spf13/pflag"
)

type PullImage string

var (
	PullImageAlways  PullImage = "always"
	PullImageNever   PullImage = "never"
	PullImageMissing PullImage = "missing"
)

func (d PullImage) String() string {
	return string(d)
}

type ImagePolicyOptions struct {
	Policy string
}

func NewImagePolicyOptions() ImagePolicyOptions {
	return ImagePolicyOptions{
		Policy: PullImageMissing.String(),
	}
}

func (s ImagePolicyOptions) Build() Step {
	return &ImagePolicy{
		opts: s,
	}
}

func (s *ImagePolicyOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.Policy, "pull", "", s.Policy, "Pull image before running. one of [always, missing, never].")
}

type ImagePolicy struct {
	opts ImagePolicyOptions
}

type ImagePolicyContext struct {
	PullPolicy cruntime.PullImagePolicy
}

func (s *ImagePolicy) Run(rc *RunContext, next Next) error {
	policy, err := s.imagePullPolicy()
	if err != nil {
		return err
	}

	rc.ImagePolicy.PullPolicy = policy
	return next(rc)
}

func (s *ImagePolicy) imagePullPolicy() (cruntime.PullImagePolicy, error) {
	switch s.opts.Policy {
	case PullImageAlways.String():
		return cruntime.PullImagePolicyAlways, nil
	case PullImageMissing.String():
		return cruntime.PullImagePolicyMissing, nil
	case PullImageNever.String():
		return cruntime.PullImagePolicyNever, nil
	default:
		return "", fmt.Errorf("invalid pull policy given: %s", s.opts.Policy)
	}
}
