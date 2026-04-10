package run

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type TemplateOptions struct {
	Volumes []string
	User    string
}

func (s *TemplateOptions) BindFlags(flags flagset.Interface) {
	flags.StringSliceVarP(&s.Volumes, "bind", "b", s.Volumes, "Bind directory as volume to the pipeline.")
	flags.StringVarP(&s.User, "user", "u", s.User, "Username or UID (format: <name|uid>[:<group|gid>])")
}

func (s TemplateOptions) Build() Step {
	return &Template{opts: s}
}

type Template struct {
	opts TemplateOptions
}

type TemplateContext struct {
	Container v1beta1.Template
}

func (s *Template) Run(rc *RunContext, next Next) error {
	tmpl, err := s.buildTemplate(rc.ContextDir.Path)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}
	rc.Template.Container = tmpl
	return next(rc)
}

func (s *Template) buildTemplate(contextDir string) (v1beta1.Template, error) {
	tmpl := v1beta1.Template{}
	volumes := append([]string(nil), s.opts.Volumes...)
	volumes = append(volumes, fmt.Sprintf("%s:%s", contextDir, contextDir))

	for i, volume := range volumes {
		v := strings.Split(volume, ":")
		if len(v) != 2 {
			return tmpl, errors.New("invalid volume mount provided")
		}
		tmpl.VolumeMounts = append(tmpl.VolumeMounts, v1beta1.VolumeMount{
			Name:      fmt.Sprintf("volume-%d", i),
			MountPath: v[1],
			HostPath:  v[0],
		})
	}

	if s.opts.User != "" {
		parts := strings.SplitN(s.opts.User, ":", 2)
		uid, err := s.getUID(parts[0])
		if err != nil {
			return tmpl, err
		}
		intOrStr := intstr.FromInt(uid)
		tmpl.Uid = &intOrStr
		if len(parts) == 2 {
			gid, err := s.getGID(parts[1])
			if err != nil {
				return tmpl, err
			}
			gidStr := intstr.FromInt(gid)
			tmpl.Guid = &gidStr
		}
	}
	return tmpl, nil
}

func (s *Template) getUID(name string) (int, error) {
	if uid, err := strconv.Atoi(name); err == nil {
		return uid, nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

func (s *Template) getGID(name string) (int, error) {
	if gid, err := strconv.Atoi(name); err == nil {
		return gid, nil
	}
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(g.Gid)
}
