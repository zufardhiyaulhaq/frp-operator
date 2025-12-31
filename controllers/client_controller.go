/*
Copyright 2022.

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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/builder"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
)

// ClientReconciler reconciles a Client object
type ClientReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *ClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Start Client Reconciler")

	log.Info("find client configuration")
	client := &frpv1alpha1.Client{}
	err := r.Client.Get(ctx, req.NamespacedName, client)
	if err != nil {
		return ctrl.Result{}, nil
	}

	log.Info("list upstream configuration")
	upstreams := &frpv1alpha1.UpstreamList{}
	err = r.Client.List(ctx, upstreams)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("list visitor configuration")
	visitors := &frpv1alpha1.VisitorList{}
	err = r.Client.List(ctx, visitors)
	if err != nil {
		return ctrl.Result{}, err
	}

	var filteredUpstreams []frpv1alpha1.Upstream
	for _, upstream := range upstreams.Items {
		if upstream.Spec.Client == client.Name {
			filteredUpstreams = append(filteredUpstreams, upstream)
		}
	}
	log.Info(fmt.Sprintf("find %d upstream for %s", len(filteredUpstreams), client.Name))

	var filteredVisitors []frpv1alpha1.Visitor
	for _, visitor := range visitors.Items {
		if visitor.Spec.Client == client.Name {
			filteredVisitors = append(filteredVisitors, visitor)
		}
	}
	log.Info(fmt.Sprintf("find %d visitor for %s", len(filteredVisitors), client.Name))

	config, err := models.NewConfig(r.Client, client, filteredUpstreams, filteredVisitors)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Build configuration")
	configuration, err := builder.NewConfigurationBuilder().
		SetConfig(config).
		Build()
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Build config map")
	configmap, err := builder.NewConfigMapBuilder().
		SetConfig(configuration).
		SetName(client.Name).
		SetNamespace(client.Namespace).
		Build()
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("set reference config map")
	if err := controllerutil.SetControllerReference(client, configmap, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("get config map")
	createdConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: configmap.Name, Namespace: configmap.Namespace}, createdConfigMap)
	if err != nil && errors.IsNotFound(err) {
		log.Info("create config map")
		err = r.Client.Create(context.TODO(), configmap)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Build service")
	serviceBuilder := builder.NewServiceBuilder().
		SetName(client.Name).
		SetNamespace(client.Namespace).
		SetAdminPort(config.Common.AdminPort)

	for _, visitor := range filteredVisitors {
		if visitor.Spec.STCP != nil {
			serviceBuilder.AddVisitorPort(visitor.Spec.STCP.Port)
		}

		if visitor.Spec.XTCP != nil {
			serviceBuilder.AddVisitorPort(visitor.Spec.XTCP.Port)
		}
	}
	service, err := serviceBuilder.Build()
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("set reference service")
	if err = controllerutil.SetControllerReference(client, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("get service")
	createdService := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, createdService)
	if err != nil && errors.IsNotFound(err) {
		log.Info("create service")
		err = r.Client.Create(context.TODO(), service)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Build pod")
	pod, err := builder.NewPodBuilder().
		SetName(client.Name).
		SetNamespace(client.Namespace).
		SetImage("fatedier/frpc:v0.65.0").
		Build()
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("set reference pod")
	if err := controllerutil.SetControllerReference(client, pod, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("get pod")
	createdPod := &corev1.Pod{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, createdPod)
	if err != nil && errors.IsNotFound(err) {
		log.Info("create pod")
		err = r.Client.Create(context.TODO(), pod)
		if err != nil {
			return ctrl.Result{}, err
		}
		time.Sleep(10 * time.Second)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("check pod running")
	if createdPod.Status.Phase != corev1.PodRunning {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	log.Info("compare configmap")
	if !reflect.DeepEqual(createdConfigMap.Data, configmap.Data) {
		log.Info("found config diff, update configmap")

		createdConfigMap.Data = configmap.Data
		err := r.Client.Update(ctx, createdConfigMap, &ctrlclient.UpdateOptions{})
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO: need to figure out how to make sure configmap is sync in the pod rather than implementing sleep
		log.Info("wait for configmap sync in the pod")
		time.Sleep(10 * time.Second)

		log.Info("reload config")
		config.Common.AdminAddress = service.Name + "." + service.Namespace + ".svc"
		err = handler.Reload(config)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.Info("no configmap diff found")
	}

	log.Info("compare service")
	if !reflect.DeepEqual(createdService.Spec.Ports, service.Spec.Ports) {
		log.Info("found service diff, update service")

		createdService.Spec.Ports = service.Spec.Ports
		err := r.Client.Update(ctx, createdService, &ctrlclient.UpdateOptions{})
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.Info("no service diff found")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&frpv1alpha1.Client{}).
		Complete(r)
}
