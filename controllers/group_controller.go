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
	"github.com/prometheus/common/log"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckarep/golang-set"
	"github.com/go-logr/logr"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codebookv1 "github.com/brendan0powers/codebook/api/v1"
)

// GroupReconciler reconciles a Group object
type GroupReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=codebook.dev,resources=groups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=codebook.dev,resources=groups/status,verbs=get;update;patch

func (r *GroupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("group", req.NamespacedName)

	var group codebookv1.Group
	var numInstances int64
	var numRunningInstances int64
	var instanceStatus []codebookv1.GroupInstanceStatus
	instanceLabels := map[string]string{
		"groupName": req.Name,
	}

	if err := r.Get(ctx, req.NamespacedName, &group); err != nil {
		log.Error(err, "Unable to fetch Group")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	var instances codebookv1.InstanceList

	if err := r.List(ctx, &instances, client.InNamespace(req.Namespace), client.MatchingLabels(instanceLabels)); err != nil {
		log.Error(err, "Unable to list Instances")
		return ctrl.Result{}, err
	}

	updateInstances, createInstances, deleteInstances := r.CalculateInstanceActions(&group, &instances)

	log.Info(updateInstances, createInstances, deleteInstances)

	instanceMap := map[string]codebookv1.Instance{}

	for _, i := range instances.Items {
		instanceMap[i.Name] = i
	}

	if err := r.CreateInstances(ctx, createInstances, &group); err != nil {
		log.Error(err, "Failed to delete instance")
		return ctrl.Result{}, err
	}

	if err := r.UpdateInstances(ctx, &group, updateInstances, instanceMap); err != nil {
		log.Error(err, "Failed to update instance")
		return ctrl.Result{}, err
	}

	if err := r.DeleteInstances(ctx, deleteInstances, instanceMap); err != nil {
		log.Error(err, "Failed to delete instance")
		return ctrl.Result{}, err
	}

	numInstances = int64(len(createInstances) + len(updateInstances))

	for _, instance := range instances.Items {
		if instance.Status.PodState == codebookv1.Running {
			numRunningInstances += 1
		}
		instanceStatus = append(instanceStatus, codebookv1.GroupInstanceStatus{
			Name:   instance.Name,
			Status: instance.Status.PodState,
		})
	}

	group.Status.NumInstances = &numInstances
	group.Status.NumRunningInstances = &numRunningInstances
	group.Status.Instances = instanceStatus

	if err := r.Update(ctx, &group); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GroupReconciler) CalculateInstanceActions(group *codebookv1.Group, instances *codebookv1.InstanceList) ([]string, []string, []string) {
	specInstances := mapset.NewSet()
	foundInstances := mapset.NewSet()

	for _, name := range group.Spec.Instances {
		specInstances.Add(name)
	}

	for _, obj := range instances.Items {
		foundInstances.Add(obj.ObjectMeta.Name)
	}

	var updateInstances, createInstances, deleteInstances []string

	specInstances.Intersect(foundInstances).Each(func(i interface{}) bool { updateInstances = append(updateInstances, i.(string)); return false })
	foundInstances.Difference(specInstances).Each(func(i interface{}) bool { deleteInstances = append(deleteInstances, i.(string)); return false })
	specInstances.Difference(foundInstances).Each(func(i interface{}) bool { createInstances = append(createInstances, i.(string)); return false })

	return updateInstances, createInstances, deleteInstances
}

func (r *GroupReconciler) CreateInstances(ctx context.Context, instances []string, group *codebookv1.Group) error {

	for _, name := range instances {
		instance := &codebookv1.Instance{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: group.Namespace,
				Labels: map[string]string{
					"groupName": group.Name,
				},
			},
			Spec: codebookv1.InstanceSpec{
				Image:            group.Spec.Image,
				MaxRunSeconds:    group.Spec.MaxRunSeconds,
				StorageClass:     group.Spec.StorageClass,
				StorageCapacity:  group.Spec.StorageCapacity,
			},
		}

		if err := controllerutil.SetControllerReference(group, instance, r.Scheme); err != nil {
			log.Error(err, "Failed to set instance reference")
			return err
		}

		if err := r.Create(ctx, instance); err != nil {
			return err
		}
	}

	return nil
}

func (r *GroupReconciler) DeleteInstances(ctx context.Context, instances []string, instanceMap map[string]codebookv1.Instance) error {

	for _, name := range instances {
		instance, ok := instanceMap[name]
		if !ok {
			log.Info("Instance not found:", name)
			continue
		}

		err := r.Delete(ctx, &instance)

		if err != nil && !apierrs.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *GroupReconciler) UpdateInstances(ctx context.Context, group *codebookv1.Group, instances []string, instanceMap map[string]codebookv1.Instance) error {

	for _, name := range instances {
		instance, ok := instanceMap[name]
		if !ok {
			log.Info("Instance not found:", name)
			continue
		}

		instance.Spec.Image = group.Spec.Image
		instance.Spec.MaxRunSeconds = group.Spec.MaxRunSeconds
		instance.Spec.StorageClass = group.Spec.StorageClass
		instance.Spec.StorageCapacity = group.Spec.StorageCapacity

		err := r.Update(ctx, &instance)

		if err != nil && !apierrs.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	handler := &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &codebookv1.Group{},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&codebookv1.Group{}).
		Watches(&source.Kind{Type: &codebookv1.Instance{}}, handler).
		Complete(r)
}
