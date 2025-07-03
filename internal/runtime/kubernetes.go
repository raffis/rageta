package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type kubernetesOption func(*kubernetes)

type kubernetes struct {
	client clientcorev1.CoreV1Interface
}

func NewKubernetes(client clientcorev1.CoreV1Interface, opts ...kubernetesOption) *kubernetes {
	d := &kubernetes{
		client: client,
	}

	return d
}

func (d *kubernetes) DeletePod(ctx context.Context, pod *Pod) error {
	return d.client.Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
}

func (d *kubernetes) CreatePod(ctx context.Context, pod *Pod, stdin io.Reader, stdout, stderr io.Writer) (Await, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	var securityContext *corev1.SecurityContext

	if pod.Spec.Containers[0].Uid != nil {
		uid := int64(*pod.Spec.Containers[0].Uid)
		noRoot := uid != 0
		securityContext = &corev1.SecurityContext{
			RunAsUser:    &uid,
			RunAsNonRoot: &noRoot,
		}
	}

	if pod.Spec.Containers[0].Guid != nil {
		guid := int64(*pod.Spec.Containers[0].Guid)
		securityContext.RunAsGroup = &guid
	}

	var pullPolicy corev1.PullPolicy
	switch pod.Spec.Containers[0].ImagePullPolicy {
	case PullImagePolicyAlways:
		pullPolicy = corev1.PullAlways
	case PullImagePolicyMissing:
		pullPolicy = corev1.PullIfNotPresent
	case PullImagePolicyNever:
		pullPolicy = corev1.PullNever
	}

	restartPolicy := d.getRestartPolicy(pod.Spec.Containers[0].RestartPolicy)
	spec := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: pod.Name,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: restartPolicy,
			Containers: []corev1.Container{
				{
					Name:            pod.Spec.Containers[0].Name,
					Image:           pod.Spec.Containers[0].Image,
					ImagePullPolicy: pullPolicy,
					Command:         pod.Spec.Containers[0].Command,
					Args:            pod.Spec.Containers[0].Args,
					StdinOnce:       pod.Spec.Containers[0].Stdin,
					Stdin:           pod.Spec.Containers[0].Stdin,
					WorkingDir:      pod.Spec.Containers[0].PWD,
					SecurityContext: securityContext,
				},
			},
		},
	}

	for name, value := range pod.Spec.Containers[0].Env {
		spec.Spec.Containers[0].Env = append(spec.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	for _, volume := range pod.Spec.Containers[0].Volumes {
		spec.Spec.Volumes = append(spec.Spec.Volumes, corev1.Volume{
			Name: volume.Name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: volume.HostPath,
				},
			},
		})

		spec.Spec.Containers[0].VolumeMounts = append(spec.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: volume.Path,
		})
	}

	created, err := d.client.Pods("default").Create(ctx, &spec, metav1.CreateOptions{})

	logger.V(3).Info("create pod", "pod", created, "error", err)

	if err != nil {
		return nil, err
	}

	watchStream, err := d.client.Pods("default").Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(metav1.ObjectNameField, created.Name).String(),
	})

	if err != nil {
		return nil, err
	}

	return &kubeWait{
		watchStream: watchStream,
	}, nil
}

func (d *kubernetes) getRestartPolicy(policy RestartPolicy) corev1.RestartPolicy {
	switch policy {
	case RestartPolicyAlways:
		return corev1.RestartPolicyAlways
	case RestartPolicyOnFailure:
		return corev1.RestartPolicyOnFailure
	default:
		return corev1.RestartPolicyNever
	}
}

type kubeWait struct {
	watchStream watch.Interface
}

func (w *kubeWait) Wait() error {
	for event := range w.watchStream.ResultChan() {
		fmt.Printf("event %#v\n", event)
		switch event.Type {
		case watch.Error:
			return &Result{
				ExitCode: int(1),
			}

		case watch.Deleted:
			return &Result{
				ExitCode: int(1),
			}
		case watch.Modified:
			return &Result{
				ExitCode: int(1),
			}
		}
	}

	return nil
}
