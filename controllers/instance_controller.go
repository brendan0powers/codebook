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
	"reflect"

	codebookv1 "github.com/brendan0powers/codebook/api/v1"
	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// InstanceReconciler reconciles a Instance object
type InstanceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=codebook.dev,resources=instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=codebook.dev,resources=instances/status,verbs=get;update;patch

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *InstanceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("instance", req.NamespacedName)

	instanceState := codebookv1.Stopped
	var instance codebookv1.Instance

	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		log.Error(err, "Unable to fetch Instance")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	if instance.Spec.Running == nil {
		bRunning := true
		instance.Spec.Running = &bRunning
	}

	// Volume Claim
	quantity, err := resource.ParseQuantity("1Gi")
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance.Spec.StorageCapacity != nil {
		quantity = *instance.Spec.StorageCapacity
	}

	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
			StorageClassName: instance.Spec.StorageClass,
		},
	}

	if err := controllerutil.SetControllerReference(&instance, claim, r.Scheme); err != nil {
		log.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	foundClaim := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, req.NamespacedName, foundClaim)

	if err != nil && apierrs.IsNotFound(err) {
		log.Info(fmt.Sprintf("Creating Volume Claim %s/%s\n", req.Namespace, req.Name))
		err = r.Create(ctx, claim)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		//if !reflect.DeepEqual(claim.Spec, foundClaim.Spec) {
		//	foundClaim.Spec = claim.Spec
		//	log.Info(fmt.Sprintf("Updating Volume Claim %s/%s\n", req.Namespace, req.Name))
		//	err = r.Update(ctx, foundClaim)
		//	if err != nil {
		//		return ctrl.Result{}, err
		//	}
		//}
	}

	// Pod

	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"codebookName": req.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "codebook",
					Image: instance.Spec.Image,
					Ports: []corev1.ContainerPort{
						{
							Name:          "web",
							ContainerPort: 8080,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "home",
							MountPath: "/home/coder",
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name:  "codebook-init",
					Image: instance.Spec.Image,
					Command: []string{
						"cp", "-rvT", "/home/coder/", "/volumes/",
					},
					Args: []string{},
					Ports: []corev1.ContainerPort{
						{
							Name:          "web",
							ContainerPort: 80,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "home",
							MountPath: "/volumes",
						},
					},
				},
			},
			RestartPolicy:         "OnFailure",
			ActiveDeadlineSeconds: instance.Spec.MaxRunSeconds,
			Hostname:              req.Name,
			Volumes: []corev1.Volume{
				{
					Name: "home",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: req.Name,
							ReadOnly:  false,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(&instance, pod, r.Scheme); err != nil {
		log.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	found := &corev1.Pod{}
	err = r.Get(ctx, req.NamespacedName, found)

	if *instance.Spec.Running == true {
		if err != nil && apierrs.IsNotFound(err) {
			instanceState = codebookv1.Starting
			log.Info(fmt.Sprintf("Creating Pod %s/%s\n", req.Namespace, req.Name))
			err = r.Create(ctx, pod)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else if err != nil {
			return ctrl.Result{}, err
		} else {
			if !reflect.DeepEqual(pod.Spec, found.Spec) {
				found.Spec.Containers[0].Image = pod.Spec.Containers[0].Image
				found.Spec.ActiveDeadlineSeconds = pod.Spec.ActiveDeadlineSeconds

				switch found.Status.Phase {
				case corev1.PodPending:
					instanceState = codebookv1.Starting
				case corev1.PodRunning:
					instanceState = codebookv1.Running
				case corev1.PodSucceeded:
					instanceState = codebookv1.Stopped
					*instance.Spec.Running = false
				case corev1.PodFailed:
					instanceState = codebookv1.Stopped
					*instance.Spec.Running = false
				case corev1.PodUnknown:
					instanceState = codebookv1.Error
				}

				log.Info(fmt.Sprintf("Updating pod %s/%s\n", req.Namespace, req.Name))
				err = r.Update(ctx, found)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	} else {
		if err != nil && apierrs.IsNotFound(err) {
		} else if err != nil {
			return ctrl.Result{}, err
		} else {
			instanceState = codebookv1.Stopped
			if !reflect.DeepEqual(pod.Spec, found.Spec) {
				err = r.Delete(ctx, found)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	// Service

	service := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "web",
					Port: 8080,
				},
			},
			Selector: map[string]string{
				"codebookName": req.Name,
			},
			ClusterIP: "None",
		},
	}

	if err := controllerutil.SetControllerReference(&instance, service, r.Scheme); err != nil {
		log.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	foundService := &corev1.Service{}
	err = r.Get(ctx, req.NamespacedName, foundService)

	if err != nil && apierrs.IsNotFound(err) {
		log.Info(fmt.Sprintf("Creating Service %s/%s\n", req.Namespace, req.Name))
		err = r.Create(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if !reflect.DeepEqual(service.Spec, foundService.Spec) {
			foundService.Spec = service.Spec
			log.Info(fmt.Sprintf("Updating service %s/%s\n", req.Namespace, req.Name))
			err = r.Update(ctx, foundService)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Ingress

	ingress := &v1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: req.Name + "." + req.Namespace + ".brendanp.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Backend: v1beta1.IngressBackend{
										ServiceName: req.Name,
										ServicePort: intstr.FromString("web"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(&instance, ingress, r.Scheme); err != nil {
		log.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	foundIngress := &v1beta1.Ingress{}
	err = r.Get(ctx, req.NamespacedName, foundIngress)

	if err != nil && apierrs.IsNotFound(err) {
		log.Info(fmt.Sprintf("Creating Ingress %s/%s\n", req.Namespace, req.Name))
		err = r.Create(ctx, ingress)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		if !reflect.DeepEqual(ingress.Spec, foundIngress.Spec) {
			foundIngress.Spec = ingress.Spec
			log.Info(fmt.Sprintf("Updating Ingress %s/%s\n", req.Namespace, req.Name))
			err = r.Update(ctx, foundIngress)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	instance.Status.PodState = instanceState
	err = r.Update(ctx, &instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	handler := &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &codebookv1.Instance{},
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&codebookv1.Instance{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, handler).
		Watches(&source.Kind{Type: &corev1.Service{}}, handler).
		Watches(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, handler).
		Watches(&source.Kind{Type: &v1beta1.Ingress{}}, handler).
		Complete(r)

	return err
}
