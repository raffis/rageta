package runner

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type TemplateStep struct {
	volumes []string
	user    string
}

func WithTemplate(volumes []string, userSpec string) *TemplateStep {
	return &TemplateStep{volumes: volumes, user: userSpec}
}

func (s *TemplateStep) Run(rc *RunContext, next Next) error {
	tmpl, err := s.buildTemplate(rc.ContextDir)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}
	rc.Template = tmpl
	return next(rc)
}

func (s *TemplateStep) buildTemplate(contextDir string) (v1beta1.Template, error) {
	tmpl := v1beta1.Template{}
	volumes := append([]string(nil), s.volumes...)
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

	if s.user != "" {
		parts := strings.SplitN(s.user, ":", 2)
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

func (s *TemplateStep) getUID(name string) (int, error) {
	if uid, err := strconv.Atoi(name); err == nil {
		return uid, nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

func (s *TemplateStep) getGID(name string) (int, error) {
	if gid, err := strconv.Atoi(name); err == nil {
		return gid, nil
	}
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(g.Gid)
}
