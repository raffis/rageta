/*


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

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/utils"
	schedulev1beta1 "github.com/raffis/rageta/pkg/apis/schedule/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=core.rageta.io,resources=runnerpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.rageta.io,resources=runnerpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;update;patch;delete;watch;list
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// RunnerPool reconciles a RunnerPool object
type RunnerPoolReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

type RunnerPoolReconcilerOptions struct {
	MaxConcurrentReconciles int
}

// SetupWithManager adding controllers
func (r *RunnerPoolReconciler) SetupWithManager(mgr ctrl.Manager, opts RunnerPoolReconcilerOptions) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulev1beta1.RunnerPool{}, builder.WithPredicates(
			predicate.GenerationChangedPredicate{},
		)).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &schedulev1beta1.RunnerPool{}, handler.OnlyControllerOwner()),
		).
		Watches(
			&schedulev1beta1.RunnerClaim{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForChangeByRunnerClaim),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *RunnerPoolReconciler) requestsForChangeByRunnerClaim(ctx context.Context, o client.Object) []reconcile.Request {
	var list schedulev1beta1.RunnerPoolList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, pool := range list.Items {
		labelSel, err := metav1.LabelSelectorAsSelector(pool.Spec.Selector)
		if err != nil {
			r.Log.Error(err, "can not select resourceSelector selectors")
			continue
		}

		if labelSel.Matches(labels.Set(o.GetLabels())) {
			r.Log.V(1).Info("runnerclaim change update", "namespace", pool.GetNamespace(), "pool-name", pool.GetName())
			reqs = append(reqs, reconcile.Request{NamespacedName: objectKey(&pool)})
		}
	}

	return reqs
}

// Reconcile RunnerPools
func (r *RunnerPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "name", req.NamespacedName)
	logger.Info("reconciling RunnerPool")

	// Fetch the RunnerPool instance
	pool := schedulev1beta1.RunnerPool{}

	err := r.Get(ctx, req.NamespacedName, &pool)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if pool.Spec.Suspend {
		return ctrl.Result{}, nil
	}

	pool, result, err := r.reconcile(ctx, pool)
	pool.Status.ObservedGeneration = pool.GetGeneration()

	if err != nil {
		logger.Error(err, "reconcile error occurred")
		pool = schedulev1beta1.RunnerPoolReady(pool, metav1.ConditionFalse, "ReconciliationFailed", err.Error())
		r.Recorder.Eventf(&pool, nil, corev1.EventTypeNormal, "Error", "Reconcile", "failed to reconcile: %s", err.Error())
	}

	// Update status after reconciliation.
	if err := r.patchStatus(ctx, &pool); err != nil {
		logger.Error(err, "unable to update status after reconciliation")
		return ctrl.Result{}, err
	}

	return result, err
}

func isOwner(owner, owned metav1.Object) bool {
	runtimeObj, ok := (owner).(runtime.Object)
	if !ok {
		return false
	}

	for _, ownerRef := range owned.GetOwnerReferences() {
		if ownerRef.Name == owner.GetName() && ownerRef.UID == owner.GetUID() && ownerRef.Kind == runtimeObj.GetObjectKind().GroupVersionKind().Kind {
			return true
		}
	}
	return false
}

func (r *RunnerPoolReconciler) reconcile(ctx context.Context, pool schedulev1beta1.RunnerPool) (schedulev1beta1.RunnerPool, ctrl.Result, error) {
	// First we ensure Autoscaling.Min is met
	var list schedulev1beta1.RunnerPoolList
	if err := r.List(ctx, &list, client.InNamespace(pool.Namespace), client.MatchingLabelsSelector{Selector: pool.Spec.Selector}); err != nil {
		return pool, ctrl.Result{}, err
	}

	minRunnerCreate := pool.Spec.AutoScaling.Min - len(list.Items)
	for i := 0; i < minRunnerCreate; i++ {
		runner, result, err := r.createRunner(ctx, pool)
		if err != nil {
			return pool, result, err
		}
		pool = runner
	}

	var runnerClaims schedulev1beta1.RunnerClaimList
	if err := r.List(ctx, &runnerClaims); err != nil {
		return pool, ctrl.Result{}, err
	}

	for _, claim := range runnerClaims.Items {
		labelSel, err := metav1.LabelSelectorAsSelector(pool.Spec.Selector)
		if err != nil {
			r.Log.Error(err, "can not select resourceSelector selectors")
			continue
		}

		if labelSel.Matches(labels.Set(o.GetLabels())) {
			r.Log.V(1).Info("runnerclaim change update", "namespace", pool.GetNamespace(), "pool-name", pool.GetName())
			reqs = append(reqs, reconcile.Request{NamespacedName: objectKey(&pool)})
		}

	}
}

func (r *RunnerPoolReconciler) createRunner(ctx context.Context, pool schedulev1beta1.RunnerPool) (schedulev1beta1.RunnerPool, ctrl.Result, error) {
	var (
		gid          int64 = 0
		uid          int64 = 0
		runAsNonRoot       = false
	)

	controllerOwner := true
	podTemplate := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", pool.Name, rand.String(5)),
			Namespace: pool.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       pool.Name,
					APIVersion: pool.APIVersion,
					Kind:       pool.Kind,
					UID:        pool.UID,
					Controller: &controllerOwner,
				},
			},
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:    &uid,
				RunAsGroup:   &gid,
				RunAsNonRoot: &runAsNonRoot,
			},
		},
	}

	if pool.Spec.PodTemplate != nil {
		podTemplate.Labels = pool.Spec.PodTemplate.Labels
		podTemplate.Annotations = pool.Spec.PodTemplate.Annotations
	}

	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}

	podTemplate.Labels["app.kubernetes.io/instance"] = "buildkit"
	podTemplate.Labels["app.kubernetes.io/name"] = "buildkit"
	podTemplate.Labels["app.kubernetes.io/managed-by"] = "rageta-controller"

	containers := []corev1.Container{
		{
			Name:  "buildkit",
			Image: "rageta/buidkit:latest",
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Port: intstr.IntOrString{StrVal: "http", Type: intstr.String},
						Path: "/",
					},
				},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Port: intstr.IntOrString{StrVal: "http", Type: intstr.String},
						Path: "/",
					},
				},
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "http",
					ContainerPort: 8080,
				},
			},
		},
	}

	containers, err := utils.MergePatchContainers(containers, podTemplate.Spec.Containers)
	if err != nil {
		return pool, ctrl.Result{}, err
	}

	podTemplate.Spec.Containers = containers
	r.Log.Info("create runner", "pod-name", podTemplate.Name)

	/*if pool.Spec.Wait {
		var app appsv1.Deployment
		if err := r.Get(ctx, client.ObjectKey{
			Name:      fmt.Sprintf("swagger-ui-%s", pool.Name),
			Namespace: pool.Namespace,
		}, &app); err != nil {
			return pool, ctrl.Result{}, err
		}

		if app.Status.ReadyReplicas == 0 {
			pool = infrav1beta1.RunnerPoolHealthy(pool, metav1.ConditionFalse, "NoEndpointReady", "health check failed; no endpoint is ready")
			pool = infrav1beta1.RunnerPoolReady(pool, metav1.ConditionFalse, "ReconciliationFailed", "health check failed; no endpoint is ready")
			r.Recorder.Eventf(&pool, nil, corev1.EventTypeWarning, "Error", "HealthCheck", "health check failed; no endpoint is ready")
			return pool, ctrl.Result{}, nil
		}

		pool = infrav1beta1.RunnerPoolHealthy(pool, metav1.ConditionTrue, "EndpointReady", "health check passed; at least one endpoint is ready")
	} else {
		conditions.Delete(&pool, infrav1beta1.ConditionHealthy)
	}*/

	//conditions.Delete(&pool, schedulev1beta1.ConditionReconciling)
	pool = schedulev1beta1.RunnerPoolReady(pool, metav1.ConditionTrue, "ReconciliationSuccessful", fmt.Sprintf("pod/%s created", podTemplate.Name))
	return pool, ctrl.Result{}, nil
}

func (r *RunnerPoolReconciler) createOrUpdateWithOwnershipValidation(ctx context.Context, owner client.Object, obj client.Object) error {
	existing := obj.DeepCopyObject().(client.Object)
	err := r.Get(ctx, client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, existing)

	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, obj); err != nil {
			return err
		}
	} else {
		if !isOwner(owner, existing) {
			return fmt.Errorf("can not take ownership of existing resource: %s", obj.GetName())
		}

		obj.GetObjectKind().SetGroupVersionKind(existing.GetObjectKind().GroupVersionKind())

		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return fmt.Errorf("can not convert resource to unstructured: %w", err)
		}

		err = r.Apply(
			ctx,
			client.ApplyConfigurationFromUnstructured(&unstructured.Unstructured{Object: content}),
			client.FieldOwner("swagger-pool-controller"),
			client.ForceOwnership,
		)

		if err != nil {
			return fmt.Errorf("can not apply resource: %w", err)
		}
	}

	return nil
}

func (r *RunnerPoolReconciler) patchStatus(ctx context.Context, pool *schedulev1beta1.RunnerPool) error {
	key := client.ObjectKeyFromObject(pool)
	latest := &schedulev1beta1.RunnerPool{}
	if err := r.Get(ctx, key, latest); err != nil {
		return err
	}

	return r.Status().Patch(ctx, pool, client.MergeFrom(latest))
}

// objectKey returns client.ObjectKey for the object.
func objectKey(object metav1.Object) client.ObjectKey {
	return client.ObjectKey{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}
