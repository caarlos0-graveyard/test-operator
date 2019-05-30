/*
Copyright 2019 The Kubernetes Authors.

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

package foo

import (
	"context"

	shipsv1beta1 "github.com/caarlos0/test-operator/pkg/apis/ships/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Foo Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileFoo{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("foo-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Foo
	err = c.Watch(&source.Kind{Type: &shipsv1beta1.Foo{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch all types we create...
	for _, t := range []runtime.Object{
		&batchv1.Job{},
		&corev1.Pod{},
	} {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &shipsv1beta1.Foo{},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileFoo{}

// ReconcileFoo reconciles a Foo object
type ReconcileFoo struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;watch
// +kubebuilder:rbac:groups=ships.k8s.io,resources=foos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ships.k8s.io,resources=foos/status,verbs=get;update;patch
func (r *ReconcileFoo) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Foo instance
	instance := &shipsv1beta1.Foo{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	type reconcilerFn func(instance *shipsv1beta1.Foo) (reconcile.Result, error)

	for _, fn := range []reconcilerFn{
		r.reconcileJob,
		r.reconcileStatus,
	} {
		if result, err := fn(instance); err != nil {
			return result, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileFoo) reconcileJob(instance *shipsv1beta1.Foo) (reconcile.Result, error) {
	var backoff int32
	var job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    instance.Labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: instance.Labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  instance.Name,
							Image: instance.Spec.Image,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(instance, job, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	var found = &batchv1.Job{}
	var err = r.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Job", "namespace", job.Namespace, "name", job.Name)
		err = r.Create(context.TODO(), job)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileFoo) reconcileStatus(instance *shipsv1beta1.Foo) (reconcile.Result, error) {
	log.Info(
		"Will try to update Foo status based on job and pod",
		"namespace", instance.Namespace,
		"name", instance.Name,
	)

	var found = &batchv1.Job{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, found); err != nil {
		return reconcile.Result{}, err
	}

	for _, cond := range found.Status.Conditions {
		if cond.Status == corev1.ConditionFalse {
			continue
		}
		switch cond.Type {
		case batchv1.JobComplete:
			instance.Status.Status = string(batchv1.JobComplete)
		case batchv1.JobFailed:
			instance.Status.Status = string(batchv1.JobFailed)
		}
		break
	}

	if instance.Status.Status != "" {
		// means its either done or failed
		return reconcile.Result{}, nil
	}
	// otherwise its still running or trying to run, try to get pod status:

	// set to unknown first
	instance.Status.Status = string(corev1.PodUnknown)

	var pods = &corev1.PodList{}
	var opts = &client.ListOptions{
		Namespace: instance.GetNamespace(),
	}
	opts = opts.MatchingLabels(instance.Labels)
	if err := r.List(context.TODO(), opts, pods); err != nil {
		return reconcile.Result{}, nil
	}

	if len(pods.Items) == 0 {
		return reconcile.Result{}, nil
	}

	// we only create one pod so only one should be found
	var pod = pods.Items[0]
	instance.Status.Status = string(pod.Status.Phase)
	for _, cond := range pod.Status.ContainerStatuses {
		if cond.State.Waiting != nil {
			instance.Status.Status = cond.State.Waiting.Reason
			break
		}
	}

	log.Info(
		"Updating Job Status",
		"namespace", instance.Namespace,
		"name", instance.Name,
		"status", instance.Status,
	)
	return reconcile.Result{}, r.Status().Update(context.Background(), instance)
}
