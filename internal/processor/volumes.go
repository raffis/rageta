package processor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithVolumes() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.VolumeMounts == nil || spec.Service != nil {
			return nil
		}
		return &Volumes{
			volumeMounts: spec.VolumeMounts,
		}
	}
}

type Volumes struct {
	volumeMounts []v1beta1.VolumeMount
}

var ErrUnknownVolumeType = errors.New("unknown volume type")

func (s *Volumes) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		mounts := make([]v1beta1.VolumeMount, len(s.volumeMounts))
		subst := []any{}

		for i := range mounts {
			mounts[i] = *s.volumeMounts[i].DeepCopy()
			subst = append(subst, &mounts[i].MountPath)
			if mounts[i].HostPath != nil {
				subst = append(subst, &mounts[i].HostPath.Path)
			}
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for _, mount := range mounts {
			if mount.MountPath == "" {
				return ctx, errors.New("mountPath is required")
			}

			var mountOpts []llb.MountOption
			if mount.ReadOnly {
				mountOpts = append(mountOpts, llb.Readonly)
			}

			switch {
			case mount.HostPath != nil:
				if mount.HostPath.Path == "" {
					return ctx, errors.New("hostPath mount requires a path")
				}

				mountOpts = append(mountOpts, llb.SourcePath(mount.HostPath.Path))
				ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.AddMount(mount.MountPath, llb.Local("context"), mountOpts...))
			case mount.Cache != nil:
				if mount.Cache.Name == "" {
					return ctx, errors.New("cache mount requires a name")
				}

				sharing := llb.CacheMountShared
				switch strings.ToLower(strings.TrimSpace(mount.Cache.Sharing)) {
				case "", "shared":
				case "private":
					sharing = llb.CacheMountPrivate
				case "locked":
					sharing = llb.CacheMountLocked
				default:
					return ctx, fmt.Errorf("cache sharing %q: want shared, private, or locked", mount.Cache.Sharing)
				}

				mountOpts = append(mountOpts, llb.AsPersistentCacheDir(mount.Cache.Name, sharing))
				ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.AddMount(mount.MountPath, llb.Scratch(), mountOpts...))
			case mount.TmpFS != nil:
				mountOpts = append(mountOpts, llb.Tmpfs())
				ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.AddMount(mount.MountPath, llb.Scratch(), mountOpts...))
			default:
				return ctx, ErrUnknownVolumeType
			}

		}

		ctx.Build.Mounts = append(ctx.Build.Mounts, mounts...)

		return next(ctx)
	}, nil
}
