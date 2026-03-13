package run

import (
	"fmt"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/spf13/pflag"
)

type pullImage string

var (
	pullImageAlways  pullImage = "always"
	pullImageNever   pullImage = "never"
	pullImageMissing pullImage = "missing"
)

func (d pullImage) String() string {
	return string(d)
}

type ImagePolicyOptions struct {
	Policy string
}

func NewImagePolicyOptions() ImagePolicyOptions {
	return ImagePolicyOptions{
		Policy: pullImageMissing.String(),
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
	case pullImageAlways.String():
		return cruntime.PullImagePolicyAlways, nil
	case pullImageMissing.String():
		return cruntime.PullImagePolicyMissing, nil
	case pullImageNever.String():
		return cruntime.PullImagePolicyNever, nil
	default:
		return "", fmt.Errorf("invalid pull policy given: %s", s.opts.Policy)
	}
}
