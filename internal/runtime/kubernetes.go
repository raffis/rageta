package runtime

import (
	"context"
	"fmt"
	"io"
	"time"
	"maps"
	"strings"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/merge"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type kubernetes struct {
	client      clientcorev1.CoreV1Interface
	podTemplate corev1.Pod
	restConfig  *rest.Config
}

func NewKubernetes(client clientcorev1.CoreV1Interface, podTemplate corev1.Pod, restConfig *rest.Config) *kubernetes {
	d := &kubernetes{
		client:      client,
		podTemplate: podTemplate,
		restConfig:  restConfig,
	}

	return d
}

func (d *kubernetes) DeletePod(ctx context.Context, pod *Pod, timeout time.Duration) error {
	seconds := int64(timeout.Seconds())
	/*return d.client.Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{
		GracePeriodSeconds: &seconds,
	})*/
	wg := new(errgroup.Group)
	for _, container := range pod.Status.Containers {
		containerId := container.ContainerID

		wg.Go(func() error {
			return d.client.Pods("default").Delete(ctx, containerId, metav1.DeleteOptions{})
		})
	}

	return wg.Wait()
}

func (d *kubernetes) CreatePod(ctx context.Context, pod *Pod, stdin io.Reader, stdout, stderr io.Writer) (Await, error) {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	meta := metav1.ObjectMeta{
		Name:   pod.Name,
		Labels: pod.Labels,
	}

	var secrets *corev1.Secret
	if len(pod.Spec.Containers[0].Secrets) > 0 {
		secrets = &corev1.Secret{
			ObjectMeta: meta,
			StringData: make(map[string]string),
		}

		maps.Copy(secrets.StringData, pod.Spec.Containers[0].Secrets)
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
	default:
		pullPolicy = corev1.PullIfNotPresent
	}

	kubePod := &corev1.Pod{
		ObjectMeta: meta,
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "step",
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
			RestartPolicy: d.getRestartPolicy(pod.Spec.Containers[0].RestartPolicy),
		},
	}

	if secrets != nil {
		kubePod.Spec.Containers[0].EnvFrom = append(kubePod.Spec.Containers[0].EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: pod.Name,
				},
			},
		})
	}

	for name, value := range pod.Spec.Containers[0].Env {
		kubePod.Spec.Containers[0].Env = append(kubePod.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

VOLUMES:
	for _, volume := range pod.Spec.Containers[0].Volumes {
		//If there is a pod template which already covers the same volume mount we will skip it here
		if len(d.podTemplate.Spec.Containers) > 0 {
			for _, mount := range d.podTemplate.Spec.Containers[0].VolumeMounts {
				if volume.Path == mount.MountPath || strings.HasPrefix(volume.Path, mount.MountPath+"/") {
					continue VOLUMES
				}
			}
		}

		kubePod.Spec.Volumes = append(kubePod.Spec.Volumes, corev1.Volume{
			Name: volume.Name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: volume.HostPath,
				},
			},
		})

		kubePod.Spec.Containers[0].VolumeMounts = append(kubePod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: volume.Path,
		})
	}

	tmpl := d.podTemplate.DeepCopy()
	kubePod, err = merge.Pod(*tmpl, *kubePod)
	if err != nil {
		return nil, fmt.Errorf("failed to merge pod template: %w", err)
	}

	created, err := d.client.Pods("default").Create(ctx, kubePod, metav1.CreateOptions{})
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

	stream := &kubeWait{
		ctx:         ctx,
		logger:      logger,
		watchStream: watchStream,
		client:      d.client,
		restConfig:  d.restConfig,
		podName:     created.Name,
		pod:         pod,
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
		Ready:       make(chan struct{}),
		Done:        make(chan error),
	}

	go func() {
		stream.watch()
	}()

	<-stream.Ready

	return stream, nil
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
	ctx         context.Context
	logger      logr.Logger
	watchStream watch.Interface
	client      clientcorev1.CoreV1Interface
	restConfig  *rest.Config
	podName     string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	execGroup   *errgroup.Group
	pod         *Pod
	Ready       chan struct{}
	Done        chan error
}

func (w *kubeWait) Wait() error {
	return <-w.Done
}

func (w *kubeWait) watch() {
	streamsAttached := false
	ready := false

	for event := range w.watchStream.ResultChan() {
		w.logger.V(5).Info("kube watch stream event", "event", event)

		switch event.Type {
		case watch.Added:
			continue
		case watch.Error:
			w.Done <- fmt.Errorf("watch stream error: %s", event.Object.(*metav1.Status).Message)
			return
		case watch.Deleted:
			w.Done <- fmt.Errorf("pod has been deleted")
			return
		case watch.Modified:
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			if pod.Status.PodIP != "" && !ready {
				ready = true
				w.pod.Status.Containers = append(w.pod.Status.Containers, ContainerStatus{
					ContainerID: pod.Name,
					ContainerIP: pod.Status.PodIP,
					Name:        w.pod.Spec.Containers[0].Name,
				})

				w.Ready <- struct{}{}
			}

			// Attach streams when pod becomes running
			if !streamsAttached && pod.Status.Phase == corev1.PodRunning {
				if err := w.attachStreams(); err != nil {
					w.Done <- fmt.Errorf("failed to attach streams: %w", err)
					return
				}
				streamsAttached = true
				continue
			}

			// Check for container termination
			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Terminated != nil {
					if w.execGroup != nil {
						if err := w.execGroup.Wait(); err != nil {
							w.logger.V(1).Error(err, "remote stream executor failed")
						}
					}

					if status.State.Terminated.ExitCode == 0 {
						w.Done <- nil
						return
					}

					w.Done <- &Result{
						ExitCode: int(status.State.Terminated.ExitCode),
					}

					return
				}
			}
		default:
			w.Done <- fmt.Errorf("unknown event type: %s", event.Type)
			return
		}
	}
}

func (w *kubeWait) attachStreams() error {
	req := w.client.RESTClient().Post().
		Resource("pods").
		Name(w.podName).
		Namespace("default").
		SubResource("attach").
		VersionedParams(&corev1.PodAttachOptions{
			Container: "step",
			Stdin:     w.stdin != nil,
			Stdout:    w.stdout != nil,
			Stderr:    w.stderr != nil,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(w.restConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	w.execGroup = &errgroup.Group{}

	w.execGroup.Go(func() error {
		return exec.StreamWithContext(w.ctx, remotecommand.StreamOptions{
			Stdin:  w.stdin,
			Stdout: w.stdout,
			Stderr: w.stderr,
		})
	})

	return nil
}
