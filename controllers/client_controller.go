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
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/builder"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/status"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/metrics"
)

// Event reasons
const (
	EventReasonClientConnected    = "ClientConnected"
	EventReasonConfigReloaded     = "ConfigReloaded"
	EventReasonConfigReloadFailed = "ConfigReloadFailed"
	EventReasonPodImageUpdated    = "PodImageUpdated"
	EventReasonInvalidConfig      = "InvalidConfig"
)

// frpcImage is the frpc container image managed by the operator.
const frpcImage = "fatedier/frpc:v0.71.0"

// ClientReconciler reconciles a Client object
type ClientReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Config    *rest.Config
	Clientset kubernetes.Interface
	Recorder  record.EventRecorder
}

// execInPod runs a command in a pod container and returns its stdout.
func (r *ClientReconciler) execInPod(namespace, podName, containerName string, command []string) (string, error) {
	req := r.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(r.Config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return stdout.String(), fmt.Errorf("exec failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *ClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Start Client Reconciler")

	log.Info("find client configuration")
	client := &frpv1alpha1.Client{}
	err := r.Client.Get(ctx, req.NamespacedName, client)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("client not found, removing its metrics")
			metrics.DeleteClient(req.Namespace, req.Name)
		}
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
		// The last good ConfigMap keeps running, but the current spec cannot be applied: say so
		// on the Client (and in frp_client_config_synced) instead of only in the operator log.
		msg := fmt.Sprintf("invalid configuration: %v", err)
		log.Error(err, "invalid client configuration")
		r.Recorder.Event(client, corev1.EventTypeWarning, EventReasonInvalidConfig, msg)
		r.setCondition(client, status.ConditionTypeConfigSync, metav1.ConditionFalse, status.ReasonInvalidConfig, msg)
		phase := client.Status.Phase
		if phase == "" {
			phase = status.ClientPhasePending
		}
		if statusErr := r.updateClientStatus(ctx, client, phase, msg, len(filteredUpstreams), len(filteredVisitors)); statusErr != nil {
			log.Error(statusErr, "failed to update client status")
		}
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

	for _, port := range models.VisitorServicePorts(filteredVisitors) {
		serviceBuilder.AddVisitorPort(port)
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
	podBuilder := builder.NewPodBuilder().
		SetName(client.Name).
		SetNamespace(client.Namespace).
		SetImage(frpcImage).
		SetPodTemplate(client.Spec.PodTemplate)

	// Wire TLS secret if configured
	if client.Spec.Server.TLS != nil {
		if client.Spec.Server.TLS.CertFile != nil {
			podBuilder.SetTLSSecret(client.Spec.Server.TLS.CertFile.Secret.Name)
		} else if client.Spec.Server.TLS.KeyFile != nil {
			podBuilder.SetTLSSecret(client.Spec.Server.TLS.KeyFile.Secret.Name)
		}
		if client.Spec.Server.TLS.TrustedCAFile != nil {
			if client.Spec.Server.TLS.TrustedCAFile.ConfigMap != nil {
				podBuilder.SetTLSCAConfigMap(client.Spec.Server.TLS.TrustedCAFile.ConfigMap.Name)
			} else if client.Spec.Server.TLS.TrustedCAFile.Secret != nil && podBuilder.TLSSecret == "" {
				podBuilder.SetTLSSecret(client.Spec.Server.TLS.TrustedCAFile.Secret.Name)
			}
		}
	}

	pod, err := podBuilder.Build()
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
			r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodFailed, err.Error())
			if statusErr := r.updateClientStatus(ctx, client, status.ClientPhaseFailed, err.Error(), len(filteredUpstreams), len(filteredVisitors)); statusErr != nil {
				log.Error(statusErr, "failed to update client status")
			}
			metrics.RecordProxies(client.Namespace, client.Name, nil, fmt.Errorf("pod failed"))
			return ctrl.Result{}, err
		}
		// Emit event and set status
		r.Recorder.Event(client, corev1.EventTypeNormal, EventReasonClientConnected,
			fmt.Sprintf("FRP client pod created for server %s:%d", client.Spec.Server.Host, client.Spec.Server.Port))
		r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodCreated, "Pod created, waiting for it to start")
		if err := r.updateClientStatus(ctx, client, status.ClientPhasePending, "Pod created, waiting for it to start", len(filteredUpstreams), len(filteredVisitors)); err != nil {
			log.Error(err, "failed to update client status")
		}
		metrics.RecordProxies(client.Namespace, client.Name, nil, fmt.Errorf("pod not running"))
		// Requeue to wait for pod to be created and running
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Roll the pod when the image has changed (e.g. operator upgrade bumped the frpc version).
	// The config written by this operator may not be loadable by an older frpc binary.
	if len(createdPod.Spec.Containers) > 0 && createdPod.Spec.Containers[0].Image != pod.Spec.Containers[0].Image {
		if createdPod.DeletionTimestamp == nil {
			log.Info("frpc image changed, deleting pod to recreate it",
				"current", createdPod.Spec.Containers[0].Image, "desired", pod.Spec.Containers[0].Image)
			if err := r.Client.Delete(context.TODO(), createdPod); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			r.Recorder.Event(client, corev1.EventTypeNormal, EventReasonPodImageUpdated,
				fmt.Sprintf("Recreating FRP client pod with image %s", pod.Spec.Containers[0].Image))
		}
		r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodCreated, "Recreating pod with updated image")
		if err := r.updateClientStatus(ctx, client, status.ClientPhasePending, "Recreating pod with updated image", len(filteredUpstreams), len(filteredVisitors)); err != nil {
			log.Error(err, "failed to update client status")
		}
		metrics.RecordProxies(client.Namespace, client.Name, nil, fmt.Errorf("pod being recreated"))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	log.Info("check pod running")
	if createdPod.Status.Phase != corev1.PodRunning {
		r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodCreated, "Pod not yet running")
		if err := r.updateClientStatus(ctx, client, status.ClientPhasePending, "Pod not yet running", len(filteredUpstreams), len(filteredVisitors)); err != nil {
			log.Error(err, "failed to update client status")
		}
		metrics.RecordProxies(client.Namespace, client.Name, nil, fmt.Errorf("pod not running"))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Pod is running, update status
	r.setCondition(client, status.ConditionTypeReady, metav1.ConditionTrue, status.ReasonPodRunning, "FRP client pod is running")
	if err := r.updateClientStatus(ctx, client, status.ClientPhaseRunning,
		fmt.Sprintf("Connected to %s:%d", client.Spec.Server.Host, client.Spec.Server.Port),
		len(filteredUpstreams), len(filteredVisitors)); err != nil {
		log.Error(err, "failed to update client status")
	}

	// Read proxy status from the frpc admin API for metrics. Failure is not fatal.
	config.Common.AdminAddress = service.Name + "." + service.Namespace + ".svc"
	proxies, statusErr := handler.Status(config)
	if statusErr != nil {
		log.Info("failed to read frpc status", "error", statusErr.Error())
	}
	metrics.RecordProxies(client.Namespace, client.Name, proxies, statusErr)

	log.Info("compare configmap")
	reloadPending := createdConfigMap.Annotations != nil && createdConfigMap.Annotations["frp.zufardhiyaulhaq.com/reload-pending"] == "true"

	if !reflect.DeepEqual(createdConfigMap.Data, configmap.Data) {
		log.Info("found config diff, update configmap")

		createdConfigMap.Data = configmap.Data
		if createdConfigMap.Annotations == nil {
			createdConfigMap.Annotations = make(map[string]string)
		}
		createdConfigMap.Annotations["frp.zufardhiyaulhaq.com/reload-pending"] = "true"

		err := r.Client.Update(ctx, createdConfigMap, &ctrlclient.UpdateOptions{})
		if err != nil {
			return ctrl.Result{}, err
		}

		// Requeue to allow ConfigMap to sync to the pod
		log.Info("configmap updated, requeuing to verify sync")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Only verify and reload if there's a pending reload
	if reloadPending {
		// Read the config file from the pod to verify it matches expected config
		log.Info("verifying configmap is synced to pod")
		podConfigContent, err := r.execInPod(createdPod.Namespace, createdPod.Name, "frpc", []string{"cat", "/frp/config.toml"})
		if err != nil {
			log.Error(err, "failed to read config from pod, requeuing")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Compare pod's config with expected config
		expectedConfig := configmap.Data["config.toml"]
		if podConfigContent != expectedConfig {
			log.Info("configmap not yet synced to pod, requeuing")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		log.Info("verifying frpc config with frpc verify")
		if out, err := r.execInPod(createdPod.Namespace, createdPod.Name, "frpc", []string{"frpc", "verify", "-c", "/frp/config.toml"}); err != nil {
			detail := strings.TrimSpace(out)
			if detail == "" {
				detail = err.Error()
			}
			msg := fmt.Sprintf("frpc verify failed: %s", detail)
			log.Error(err, "frpc verify failed, not reloading")
			r.Recorder.Event(client, corev1.EventTypeWarning, EventReasonConfigReloadFailed, msg)
			r.setCondition(client, status.ConditionTypeConfigSync, metav1.ConditionFalse, status.ReasonConfigReloadFailed, msg)
			if statusErr := r.updateClientStatus(ctx, client, status.ClientPhaseRunning, msg, len(filteredUpstreams), len(filteredVisitors)); statusErr != nil {
				log.Error(statusErr, "failed to update client status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// Config is synced, reload frpc
		log.Info("configmap synced to pod, reloading frpc config")
		err = handler.Reload(config)
		if err != nil {
			log.Error(err, "failed to reload config")
			r.Recorder.Event(client, corev1.EventTypeWarning, EventReasonConfigReloadFailed,
				fmt.Sprintf("Failed to reload config: %v", err))
			r.setCondition(client, status.ConditionTypeConfigSync, metav1.ConditionFalse, status.ReasonConfigReloadFailed, err.Error())
			if statusErr := r.updateClientStatus(ctx, client, status.ClientPhaseRunning, fmt.Sprintf("Config reload failed: %v", err), len(filteredUpstreams), len(filteredVisitors)); statusErr != nil {
				log.Error(statusErr, "failed to update client status")
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Clear the reload-pending annotation
		delete(createdConfigMap.Annotations, "frp.zufardhiyaulhaq.com/reload-pending")
		err = r.Client.Update(ctx, createdConfigMap, &ctrlclient.UpdateOptions{})
		if err != nil {
			log.Error(err, "failed to clear reload-pending annotation")
			return ctrl.Result{}, err
		}
		log.Info("config reloaded successfully")
		r.Recorder.Event(client, corev1.EventTypeNormal, EventReasonConfigReloaded, "Configuration reloaded successfully")
		r.setCondition(client, status.ConditionTypeConfigSync, metav1.ConditionTrue, status.ReasonConfigReloaded, "Configuration synchronized")
		if statusErr := r.updateClientStatus(ctx, client, status.ClientPhaseRunning,
			fmt.Sprintf("Connected to %s:%d", client.Spec.Server.Host, client.Spec.Server.Port),
			len(filteredUpstreams), len(filteredVisitors)); statusErr != nil {
			log.Error(statusErr, "failed to update client status")
		}
	} else {
		log.Info("no configmap diff found")
		r.setCondition(client, status.ConditionTypeConfigSync, metav1.ConditionTrue, status.ReasonConfigReloaded, "Configuration synchronized")
		if statusErr := r.updateClientStatus(ctx, client, status.ClientPhaseRunning,
			fmt.Sprintf("Connected to %s:%d", client.Spec.Server.Host, client.Spec.Server.Port),
			len(filteredUpstreams), len(filteredVisitors)); statusErr != nil {
			log.Error(statusErr, "failed to update client status")
		}
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
	r.Config = mgr.GetConfig()
	r.Recorder = mgr.GetEventRecorderFor("client-controller")

	clientset, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}
	r.Clientset = clientset

	return ctrl.NewControllerManagedBy(mgr).
		For(&frpv1alpha1.Client{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// updateClientStatus updates the status of a Client resource and refreshes its metrics.
func (r *ClientReconciler) updateClientStatus(ctx context.Context, client *frpv1alpha1.Client,
	phase, message string, upstreamCount, visitorCount int) error {

	client.Status.Phase = phase
	client.Status.Message = message
	client.Status.UpstreamCount = upstreamCount
	client.Status.VisitorCount = visitorCount

	clientID := client.Namespace + "/" + client.Name
	if client.Spec.ClientID != nil {
		clientID = *client.Spec.ClientID
	}
	metrics.RecordClient(client, clientID, frpcImage)
	metrics.SetLastReconcile(client.Namespace, client.Name, time.Now())

	return r.Status().Update(ctx, client)
}

// setCondition sets or updates a condition on the Client status. LastTransitionTime only moves
// when the condition's status changes, so repeated reconciles do not rewrite an unchanged status.
func (r *ClientReconciler) setCondition(client *frpv1alpha1.Client,
	conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {

	meta.SetStatusCondition(&client.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})
}
